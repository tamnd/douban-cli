// Package crawl runs the Douban mirror: it drains the frontier, fetches each URL
// through the source that serves it, stores the raw bytes and a record, expands
// the links it discovers, and marks every row's outcome. The engine is
// resumable (in-flight rows stay pending) and polite (per-host rate limiting),
// and it never silently caps; it logs what it blocks or skips.
package crawl

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/tamnd/douban-cli/douban"
	"github.com/tamnd/douban-cli/mirror"
	"github.com/tamnd/douban-cli/mirror/store"
)

// HTMLClient fetches an absolute URL. *douban.Client satisfies it.
type HTMLClient interface {
	Get(ctx context.Context, url string) ([]byte, error)
}

// FrodoClient fetches a signed API path. *douban.FrodoClient satisfies it.
type FrodoClient interface {
	Get(ctx context.Context, path string, params url.Values) ([]byte, error)
}

// Config tunes a crawl run.
type Config struct {
	Concurrency int           // worker pool size (default 4)
	Limit       int           // max URLs to process this run (0 = drain the frontier)
	HTMLDelay   time.Duration // per-host spacing for HTML hosts (default 5s)
	Types       []string      // restrict to these entity types (empty = all)
	Source      string        // restrict to this source (empty = all)
	Logf        func(format string, args ...any)
}

// Stats summarizes a run.
type Stats struct {
	Processed  int
	Done       int
	Failed     int
	Blocked    int
	Skipped    int
	Discovered int
}

// Engine ties the store and fetchers together.
type Engine struct {
	store *store.Store
	html  HTMLClient
	frodo FrodoClient
	cfg   Config
	limit *hostLimiter
}

// New builds an Engine. Either fetcher may be nil if a run touches only the
// other source.
func New(st *store.Store, html HTMLClient, frodo FrodoClient, cfg Config) *Engine {
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 4
	}
	if cfg.HTMLDelay <= 0 {
		cfg.HTMLDelay = 5 * time.Second
	}
	if cfg.Logf == nil {
		cfg.Logf = func(string, ...any) {}
	}
	return &Engine{
		store: st,
		html:  html,
		frodo: frodo,
		cfg:   cfg,
		limit: newHostLimiter(cfg.HTMLDelay),
	}
}

// Run drains pending frontier rows (subject to the configured filters and limit)
// until none remain or the context is cancelled. Newly discovered links are
// enqueued as it goes, so a single Run expands the mirror outward from its seeds.
func (e *Engine) Run(ctx context.Context) (Stats, error) {
	var st Stats
	batch := e.cfg.Concurrency * 4
	if batch < 16 {
		batch = 16
	}
	for {
		if err := ctx.Err(); err != nil {
			return st, err
		}
		n := batch
		if e.cfg.Limit > 0 {
			if rem := e.cfg.Limit - st.Processed; rem < n {
				n = rem
			}
		}
		if n <= 0 {
			break
		}
		rows, err := e.store.Pending(e.cfg.Types, e.cfg.Source, n)
		if err != nil {
			return st, err
		}
		if len(rows) == 0 {
			break
		}
		e.runBatch(ctx, rows, &st)
	}
	return st, nil
}

// runBatch processes rows concurrently with a bounded worker pool, folding each
// outcome into st under a mutex.
func (e *Engine) runBatch(ctx context.Context, rows []store.Frontier, st *Stats) {
	sem := make(chan struct{}, e.cfg.Concurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, f := range rows {
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(f store.Frontier) {
			defer wg.Done()
			defer func() { <-sem }()
			out := e.process(ctx, f)
			mu.Lock()
			st.Processed++
			st.Done += out.done
			st.Failed += out.failed
			st.Blocked += out.blocked
			st.Skipped += out.skipped
			st.Discovered += out.discovered
			mu.Unlock()
		}(f)
	}
	wg.Wait()
}

type outcome struct {
	done, failed, blocked, skipped, discovered int
}

// process fetches and stores one frontier row, routing by source.
func (e *Engine) process(ctx context.Context, f store.Frontier) outcome {
	switch f.Source {
	case store.SourceFrodo:
		return e.fetchFrodo(ctx, f)
	case store.SourceHTML, store.SourceMobile:
		return e.fetchHTML(ctx, f)
	default:
		_ = e.store.Complete(f.URL, store.StatusSkipped, 0, "unknown source", "", "")
		return outcome{skipped: 1}
	}
}

// fetchFrodo pulls a signed JSON document, archives it, and records it. The raw
// Frodo JSON is the record for now; the normalizer milestone maps it to a
// uniform schema.
func (e *Engine) fetchFrodo(ctx context.Context, f store.Frontier) outcome {
	path, ok := frodoPath(f.EntityType, f.EntityID)
	if !ok || e.frodo == nil {
		e.cfg.Logf("blocked %s/%s: no frodo route", f.EntityType, f.EntityID)
		_ = e.store.Complete(f.URL, store.StatusBlocked, 0, "no frodo route", "", "")
		return outcome{blocked: 1}
	}
	body, err := e.frodo.Get(ctx, path, nil)
	// The movie.douban.com/subject/ space does not distinguish films from TV
	// series, so a TV id requested on /movie/ comes back as code 996 with the
	// correct endpoint named in the error. Retry the sibling /tv/ path so TV
	// series are captured instead of recorded as blocked.
	if alt, ok := frodoMediaAlt(f.EntityType, f.EntityID, err); ok {
		if b2, err2 := e.frodo.Get(ctx, alt, nil); err2 == nil {
			body, err = b2, nil
		}
	}
	switch {
	case errors.Is(err, douban.ErrNotFound):
		_ = e.store.Complete(f.URL, store.StatusSkipped, 404, "not found", "", "")
		return outcome{skipped: 1}
	case err != nil:
		var fe *douban.FrodoError
		if errors.As(err, &fe) {
			e.cfg.Logf("blocked %s/%s: %s", f.EntityType, f.EntityID, fe.Error())
			_ = e.store.Complete(f.URL, store.StatusBlocked, fe.Status, fe.Error(), "", "")
			return outcome{blocked: 1}
		}
		_ = e.store.Complete(f.URL, store.StatusFailed, 0, err.Error(), "", "")
		return outcome{failed: 1}
	}

	rel, hash, err := e.store.WriteRaw(store.SourceFrodo, f.EntityType, f.EntityID, "json", body)
	if err != nil {
		_ = e.store.Complete(f.URL, store.StatusFailed, 200, err.Error(), "", "")
		return outcome{failed: 1}
	}
	rec, nerr := mirror.NormalizeFrodo(f.EntityType, f.EntityID, f.URL, body)
	if nerr != nil && json.Valid(body) {
		rec = json.RawMessage(body) // fall back to raw if it is not an object
	}
	if rec != nil {
		_ = e.store.PutRecord(store.Record{
			EntityType: f.EntityType, EntityID: f.EntityID,
			Source: store.SourceFrodo, JSON: rec,
		})
	}
	_ = e.store.Complete(f.URL, store.StatusDone, 200, "", rel, hash)
	return outcome{done: 1}
}

// fetchHTML pulls a page over a per-host rate limit, archives it, records a
// minimal entity stub, and enqueues the entity links it finds. Rich per-type
// parsing arrives with the normalizer milestone.
func (e *Engine) fetchHTML(ctx context.Context, f store.Frontier) outcome {
	if e.html == nil {
		_ = e.store.Complete(f.URL, store.StatusSkipped, 0, "no html client", "", "")
		return outcome{skipped: 1}
	}
	e.limit.wait(ctx, hostOf(f.URL))

	body, err := e.html.Get(ctx, f.URL)
	switch {
	case errors.Is(err, douban.ErrNotFound):
		_ = e.store.Complete(f.URL, store.StatusSkipped, 404, "not found", "", "")
		return outcome{skipped: 1}
	case errors.Is(err, douban.ErrForbidden):
		// A 403 is a wall (login-gated or anti-bot), not a transient error;
		// record it as blocked so reset-failed does not retry it forever.
		e.cfg.Logf("blocked %s/%s: http 403", f.EntityType, f.EntityID)
		_ = e.store.Complete(f.URL, store.StatusBlocked, 403, "http 403", "", "")
		return outcome{blocked: 1}
	case err != nil:
		_ = e.store.Complete(f.URL, store.StatusFailed, 0, err.Error(), "", "")
		return outcome{failed: 1}
	}

	rel, hash, err := e.store.WriteRaw(store.SourceHTML, f.EntityType, f.EntityID, "html", body)
	if err != nil {
		_ = e.store.Complete(f.URL, store.StatusFailed, 200, err.Error(), "", "")
		return outcome{failed: 1}
	}
	_ = e.store.PutRecord(store.Record{
		EntityType: f.EntityType, EntityID: f.EntityID,
		Source: store.SourceHTML,
		JSON:   mirror.NormalizeHTML(f.EntityType, f.EntityID, f.URL, body),
	})

	discovered := e.expand(f.URL, body)
	_ = e.store.Complete(f.URL, store.StatusDone, 200, "", rel, hash)
	return outcome{done: 1, discovered: discovered}
}

var hrefRE = regexp.MustCompile(`href="([^"]+)"`)

// expand finds entity links on a page and enqueues the ones not already known.
// Links are resolved against the page URL before classification.
func (e *Engine) expand(pageURL string, body []byte) int {
	base, err := url.Parse(pageURL)
	if err != nil {
		return 0
	}
	seen := map[string]bool{}
	added := 0
	for _, m := range hrefRE.FindAllSubmatch(body, -1) {
		href := string(m[1])
		ref, err := url.Parse(href)
		if err != nil {
			continue
		}
		abs := base.ResolveReference(ref).String()
		abs = strings.SplitN(abs, "#", 2)[0]
		if seen[abs] {
			continue
		}
		seen[abs] = true
		c, ok := mirror.Classify(abs)
		if !ok {
			continue
		}
		// Enqueue the entity's canonical URL, not the discovered href, so a
		// subject's sub-pages (/comments, /reviews, /new_review) collapse onto
		// one row that is fetched once, instead of each becoming a separate,
		// often login-gated, request.
		enqURL := c.Canonical
		if enqURL == "" {
			enqURL = abs
		}
		ins, err := e.store.Enqueue(store.Frontier{
			URL: enqURL, EntityType: c.EntityType, EntityID: c.EntityID, Source: c.Source,
		})
		if err == nil && ins {
			added++
		}
	}
	return added
}

// frodoPath maps an entity to its signed API path. Movie, tv, book, music,
// game, celebrity and personage are verified against the live API; a tv id is
// requested on /movie/ first and falls back to /tv/ via frodoMediaAlt. Note and
// review are best effort and may come back not-found or blocked (notes sit
// behind a JS challenge), which the engine records faithfully.
func frodoPath(typ, id string) (string, bool) {
	switch typ {
	case "movie", "tv":
		return "/api/v2/movie/" + id, true
	case "book":
		return "/api/v2/book/" + id, true
	case "music":
		return "/api/v2/music/" + id, true
	case "game":
		return "/api/v2/game/" + id, true
	case "personage":
		return "/api/v2/elessar/subject/" + id, true
	case "note":
		return "/api/v2/note/" + id, true
	case "review":
		return "/api/v2/review/" + id, true
	case "celebrity":
		return "/api/v2/celebrity/" + id, true
	default:
		return "", false
	}
}

// frodoMediaAlt returns the sibling media path to retry when the Frodo API
// rejected a movie/tv subject as being on the wrong endpoint (code 996). Both
// movie and tv subjects are first tried on /movie/, so the alternate is /tv/.
func frodoMediaAlt(typ, id string, err error) (string, bool) {
	var fe *douban.FrodoError
	if !errors.As(err, &fe) || fe.Code != 996 {
		return "", false
	}
	switch typ {
	case "movie", "tv":
		return "/api/v2/tv/" + id, true
	default:
		return "", false
	}
}

func hostOf(rawURL string) string {
	if u, err := url.Parse(rawURL); err == nil {
		return u.Hostname()
	}
	return rawURL
}

// hostLimiter spaces requests per host so distinct hosts can be fetched
// concurrently while each one stays within the configured delay.
type hostLimiter struct {
	delay time.Duration
	mu    sync.Mutex
	gates map[string]*gate
}

type gate struct {
	mu   sync.Mutex
	last time.Time
}

func newHostLimiter(delay time.Duration) *hostLimiter {
	return &hostLimiter{delay: delay, gates: map[string]*gate{}}
}

func (h *hostLimiter) wait(ctx context.Context, host string) {
	if h.delay <= 0 {
		return
	}
	h.mu.Lock()
	g := h.gates[host]
	if g == nil {
		g = &gate{}
		h.gates[host] = g
	}
	h.mu.Unlock()

	g.mu.Lock()
	defer g.mu.Unlock()
	if wait := h.delay - time.Since(g.last); wait > 0 {
		select {
		case <-ctx.Done():
		case <-time.After(wait):
		}
	}
	g.last = time.Now()
}
