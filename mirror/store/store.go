// Package store is the persistence layer for the Douban mirror: a frontier of
// URLs to crawl, the normalized records harvested from them, crawl cursors, and
// the raw artifact files captured verbatim.
//
// It uses a pure-Go SQLite (modernc.org/sqlite) so the binary keeps building
// with CGO_ENABLED=0. The database lives at <dir>/douban.db; raw artifacts go
// under <dir>/raw/ and JSONL exports under <dir>/export/.
package store

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Frontier status values.
const (
	StatusPending = "pending"
	StatusDone    = "done"
	StatusFailed  = "failed"
	StatusBlocked = "blocked"
	StatusSkipped = "skipped"
)

// Source values record which surface produced an artifact or record.
const (
	SourceFrodo  = "frodo"
	SourceHTML   = "html"
	SourceMobile = "mobile"
)

// Frontier is one row of the crawl queue: a URL plus what is known about it.
type Frontier struct {
	URL          string
	EntityType   string
	EntityID     string
	Source       string
	Status       string
	Attempts     int
	Priority     int
	HTTPStatus   int
	Error        string
	RawPath      string
	ContentHash  string
	DiscoveredAt time.Time
	FetchedAt    time.Time
}

// Record is a normalized structured entity, stored as JSON and exported to JSONL.
type Record struct {
	EntityType string
	EntityID   string
	Source     string
	JSON       json.RawMessage
	FetchedAt  time.Time
}

// Store owns the mirror directory and its SQLite database.
type Store struct {
	dir string
	db  *sql.DB
	mu  sync.Mutex // serializes writes; SQLite is single-writer
	now func() time.Time
}

const schema = `
CREATE TABLE IF NOT EXISTS frontier (
	url           TEXT PRIMARY KEY,
	entity_type   TEXT NOT NULL,
	entity_id     TEXT NOT NULL,
	source        TEXT NOT NULL,
	status        TEXT NOT NULL DEFAULT 'pending',
	attempts      INTEGER NOT NULL DEFAULT 0,
	priority      INTEGER NOT NULL DEFAULT 0,
	http_status   INTEGER NOT NULL DEFAULT 0,
	error         TEXT NOT NULL DEFAULT '',
	raw_path      TEXT NOT NULL DEFAULT '',
	content_hash  TEXT NOT NULL DEFAULT '',
	discovered_at TEXT NOT NULL DEFAULT '',
	fetched_at    TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_frontier_status   ON frontier(status, priority DESC);
CREATE INDEX IF NOT EXISTS idx_frontier_type     ON frontier(entity_type);

CREATE TABLE IF NOT EXISTS records (
	entity_type TEXT NOT NULL,
	entity_id   TEXT NOT NULL,
	source      TEXT NOT NULL,
	json        TEXT NOT NULL,
	fetched_at  TEXT NOT NULL DEFAULT '',
	PRIMARY KEY (entity_type, entity_id)
);
CREATE INDEX IF NOT EXISTS idx_records_type ON records(entity_type);

CREATE TABLE IF NOT EXISTS meta (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);
`

// Open opens (creating if needed) the mirror at dir and applies the schema.
func Open(dir string) (*Store, error) {
	if dir == "" {
		return nil, fmt.Errorf("store: empty dir")
	}
	for _, sub := range []string{"", "raw", "export"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			return nil, fmt.Errorf("store: mkdir %s: %w", sub, err)
		}
	}
	dsn := "file:" + filepath.Join(dir, "douban.db") +
		"?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: open db: %w", err)
	}
	db.SetMaxOpenConns(1) // one writer; reads are fast and serialized
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("store: schema: %w", err)
	}
	return &Store{dir: dir, db: db, now: time.Now}, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// Dir, RawDir, ExportDir, DBPath expose the on-disk layout.
func (s *Store) Dir() string       { return s.dir }
func (s *Store) RawDir() string    { return filepath.Join(s.dir, "raw") }
func (s *Store) ExportDir() string { return filepath.Join(s.dir, "export") }
func (s *Store) DBPath() string    { return filepath.Join(s.dir, "douban.db") }

// Enqueue inserts a frontier URL if it is not already present. It returns true
// when a new row was added. Re-seeding an existing URL is a no-op (its status
// and history are preserved).
func (s *Store) Enqueue(f Frontier) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if f.Status == "" {
		f.Status = StatusPending
	}
	res, err := s.db.Exec(
		`INSERT OR IGNORE INTO frontier
			(url, entity_type, entity_id, source, status, priority, discovered_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
		f.URL, f.EntityType, f.EntityID, f.Source, f.Status, f.Priority,
		s.now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return false, fmt.Errorf("store: enqueue: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// NextPending returns up to limit pending frontier rows, highest priority first.
// Callers coordinate dequeueing (a single feeder goroutine) so the same row is
// not handed to two workers; rows stay pending while in flight so an interrupted
// run resumes cleanly.
func (s *Store) NextPending(limit int) ([]Frontier, error) {
	rows, err := s.db.Query(
		`SELECT url, entity_type, entity_id, source, status, attempts, priority
			FROM frontier WHERE status = ?
			ORDER BY priority DESC, rowid ASC LIMIT ?`,
		StatusPending, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("store: next pending: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Frontier
	for rows.Next() {
		var f Frontier
		if err := rows.Scan(&f.URL, &f.EntityType, &f.EntityID, &f.Source,
			&f.Status, &f.Attempts, &f.Priority); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// Complete marks a URL with a terminal (or retryable) status, recording the HTTP
// status, any error, the raw artifact path and hash, and bumping the attempt
// count. The fetched_at timestamp is set for done/blocked/skipped.
func (s *Store) Complete(url, status string, httpStatus int, errMsg, rawPath, hash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	fetched := ""
	if status == StatusDone || status == StatusBlocked || status == StatusSkipped {
		fetched = s.now().UTC().Format(time.RFC3339)
	}
	_, err := s.db.Exec(
		`UPDATE frontier
			SET status = ?, attempts = attempts + 1, http_status = ?, error = ?,
			    raw_path = ?, content_hash = ?, fetched_at = ?
			WHERE url = ?`,
		status, httpStatus, errMsg, rawPath, hash, fetched, url,
	)
	if err != nil {
		return fmt.Errorf("store: complete %s: %w", url, err)
	}
	return nil
}

// ResetFailed moves failed rows back to pending so a later crawl retries them.
// An empty entityType resets every type. It returns the number requeued.
func (s *Store) ResetFailed(entityType string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var (
		res sql.Result
		err error
	)
	if entityType == "" {
		res, err = s.db.Exec(`UPDATE frontier SET status = ? WHERE status = ?`,
			StatusPending, StatusFailed)
	} else {
		res, err = s.db.Exec(
			`UPDATE frontier SET status = ? WHERE status = ? AND entity_type = ?`,
			StatusPending, StatusFailed, entityType)
	}
	if err != nil {
		return 0, fmt.Errorf("store: reset failed: %w", err)
	}
	return res.RowsAffected()
}

// PutRecord upserts a normalized record.
func (s *Store) PutRecord(r Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(
		`INSERT INTO records (entity_type, entity_id, source, json, fetched_at)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(entity_type, entity_id)
			DO UPDATE SET source = excluded.source, json = excluded.json,
			              fetched_at = excluded.fetched_at`,
		r.EntityType, r.EntityID, r.Source, string(r.JSON),
		s.now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("store: put record %s/%s: %w", r.EntityType, r.EntityID, err)
	}
	return nil
}

// WriteRaw gzips data to raw/<source>/<type>/<shard>/<id>.<ext>.gz and returns
// the path relative to the mirror dir plus the sha256 of the uncompressed bytes.
// The shard is the last two digits of the id, capping directory fan-out.
func (s *Store) WriteRaw(source, entityType, id, ext string, data []byte) (relPath, hash string, err error) {
	sum := sha256.Sum256(data)
	hash = hex.EncodeToString(sum[:])
	rel := filepath.Join("raw", source, entityType, shardOf(id), id+"."+ext+".gz")
	abs := filepath.Join(s.dir, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", "", fmt.Errorf("store: raw mkdir: %w", err)
	}
	f, err := os.Create(abs)
	if err != nil {
		return "", "", fmt.Errorf("store: raw create: %w", err)
	}
	gz := gzip.NewWriter(f)
	if _, err := gz.Write(data); err != nil {
		_ = gz.Close()
		_ = f.Close()
		return "", "", fmt.Errorf("store: raw write: %w", err)
	}
	if err := gz.Close(); err != nil {
		_ = f.Close()
		return "", "", fmt.Errorf("store: raw gzip close: %w", err)
	}
	if err := f.Close(); err != nil {
		return "", "", fmt.Errorf("store: raw close: %w", err)
	}
	return rel, hash, nil
}

// ReadRaw returns the decompressed bytes of a raw artifact given its relative path.
func (s *Store) ReadRaw(relPath string) ([]byte, error) {
	f, err := os.Open(filepath.Join(s.dir, relPath))
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer func() { _ = gz.Close() }()
	return io.ReadAll(gz)
}

// SetMeta and GetMeta store and read crawl cursors and counters.
func (s *Store) SetMeta(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(
		`INSERT INTO meta (key, value) VALUES (?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value,
	)
	if err != nil {
		return fmt.Errorf("store: set meta %s: %w", key, err)
	}
	return nil
}

// GetMeta returns the value for key, or ok=false if unset.
func (s *Store) GetMeta(key string) (value string, ok bool, err error) {
	row := s.db.QueryRow(`SELECT value FROM meta WHERE key = ?`, key)
	switch err := row.Scan(&value); err {
	case nil:
		return value, true, nil
	case sql.ErrNoRows:
		return "", false, nil
	default:
		return "", false, fmt.Errorf("store: get meta %s: %w", key, err)
	}
}

// FrontierCounts returns frontier row counts grouped by (entity_type, status).
func (s *Store) FrontierCounts() (map[string]map[string]int, error) {
	rows, err := s.db.Query(
		`SELECT entity_type, status, COUNT(*) FROM frontier GROUP BY entity_type, status`)
	if err != nil {
		return nil, fmt.Errorf("store: frontier counts: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]map[string]int{}
	for rows.Next() {
		var t, st string
		var n int
		if err := rows.Scan(&t, &st, &n); err != nil {
			return nil, err
		}
		if out[t] == nil {
			out[t] = map[string]int{}
		}
		out[t][st] = n
	}
	return out, rows.Err()
}

// RecordCounts returns record counts grouped by entity_type.
func (s *Store) RecordCounts() (map[string]int, error) {
	rows, err := s.db.Query(`SELECT entity_type, COUNT(*) FROM records GROUP BY entity_type`)
	if err != nil {
		return nil, fmt.Errorf("store: record counts: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]int{}
	for rows.Next() {
		var t string
		var n int
		if err := rows.Scan(&t, &n); err != nil {
			return nil, err
		}
		out[t] = n
	}
	return out, rows.Err()
}

// QueueRows returns frontier rows filtered by status and/or type, for inspection.
// Empty status or entityType means "any".
func (s *Store) QueueRows(status, entityType string, limit int) ([]Frontier, error) {
	q := `SELECT url, entity_type, entity_id, source, status, attempts, priority,
		http_status, error, raw_path FROM frontier WHERE 1=1`
	var args []any
	if status != "" {
		q += " AND status = ?"
		args = append(args, status)
	}
	if entityType != "" {
		q += " AND entity_type = ?"
		args = append(args, entityType)
	}
	q += " ORDER BY rowid ASC"
	if limit > 0 {
		q += " LIMIT ?"
		args = append(args, limit)
	}
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("store: queue rows: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []Frontier
	for rows.Next() {
		var f Frontier
		if err := rows.Scan(&f.URL, &f.EntityType, &f.EntityID, &f.Source,
			&f.Status, &f.Attempts, &f.Priority, &f.HTTPStatus, &f.Error, &f.RawPath); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// ExportJSONL streams records of entityType (empty = all) to w, one JSON object
// per line. It returns the number of records written.
func (s *Store) ExportJSONL(ctx context.Context, entityType string, w io.Writer) (int, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if entityType == "" {
		rows, err = s.db.QueryContext(ctx, `SELECT json FROM records ORDER BY entity_type, entity_id`)
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT json FROM records WHERE entity_type = ? ORDER BY entity_id`, entityType)
	}
	if err != nil {
		return 0, fmt.Errorf("store: export: %w", err)
	}
	defer func() { _ = rows.Close() }()

	n := 0
	for rows.Next() {
		var js string
		if err := rows.Scan(&js); err != nil {
			return n, err
		}
		if _, err := io.WriteString(w, js); err != nil {
			return n, err
		}
		if _, err := io.WriteString(w, "\n"); err != nil {
			return n, err
		}
		n++
	}
	return n, rows.Err()
}

// shardOf returns the two-digit shard key for an id (last two characters,
// left-padded). Non-id inputs fall back to "00".
func shardOf(id string) string {
	switch {
	case len(id) >= 2:
		return id[len(id)-2:]
	case len(id) == 1:
		return "0" + id
	default:
		return "00"
	}
}
