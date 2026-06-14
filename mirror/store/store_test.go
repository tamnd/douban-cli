package store

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func openTemp(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestEnqueueDedup(t *testing.T) {
	s := openTemp(t)
	f := Frontier{URL: "https://book.douban.com/subject/1084336/", EntityType: "book", EntityID: "1084336", Source: SourceHTML}

	added, err := s.Enqueue(f)
	if err != nil || !added {
		t.Fatalf("first enqueue added=%v err=%v", added, err)
	}
	added, err = s.Enqueue(f)
	if err != nil || added {
		t.Fatalf("second enqueue added=%v err=%v, want false", added, err)
	}

	pending, err := s.NextPending(10)
	if err != nil {
		t.Fatalf("NextPending: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("pending = %d, want 1", len(pending))
	}
	if pending[0].EntityID != "1084336" {
		t.Errorf("entity id = %q", pending[0].EntityID)
	}
}

func TestPriorityOrder(t *testing.T) {
	s := openTemp(t)
	_, _ = s.Enqueue(Frontier{URL: "a", EntityType: "book", EntityID: "1", Source: SourceHTML, Priority: 1})
	_, _ = s.Enqueue(Frontier{URL: "b", EntityType: "book", EntityID: "2", Source: SourceHTML, Priority: 9})
	_, _ = s.Enqueue(Frontier{URL: "c", EntityType: "book", EntityID: "3", Source: SourceHTML, Priority: 5})

	got, err := s.NextPending(10)
	if err != nil {
		t.Fatalf("NextPending: %v", err)
	}
	order := []string{got[0].URL, got[1].URL, got[2].URL}
	want := []string{"b", "c", "a"}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order = %v, want %v", order, want)
		}
	}
}

func TestCompleteTransition(t *testing.T) {
	s := openTemp(t)
	_, _ = s.Enqueue(Frontier{URL: "u", EntityType: "movie", EntityID: "1292052", Source: SourceFrodo})

	if err := s.Complete("u", StatusDone, 200, "", "raw/frodo/movie/52/1292052.json.gz", "deadbeef"); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if p, _ := s.NextPending(10); len(p) != 0 {
		t.Fatalf("still pending after done: %d", len(p))
	}
	rows, err := s.QueueRows(StatusDone, "", 10)
	if err != nil {
		t.Fatalf("QueueRows: %v", err)
	}
	if len(rows) != 1 || rows[0].Attempts != 1 || rows[0].ContentHash == "deadbeef" && rows[0].RawPath == "" {
		t.Fatalf("unexpected row %+v", rows[0])
	}
	if rows[0].HTTPStatus != 200 {
		t.Errorf("http status = %d", rows[0].HTTPStatus)
	}
}

func TestResetFailed(t *testing.T) {
	s := openTemp(t)
	_, _ = s.Enqueue(Frontier{URL: "u", EntityType: "note", EntityID: "1", Source: SourceFrodo})
	_ = s.Complete("u", StatusFailed, 500, "boom", "", "")

	n, err := s.ResetFailed("")
	if err != nil {
		t.Fatalf("ResetFailed: %v", err)
	}
	if n != 1 {
		t.Fatalf("reset = %d, want 1", n)
	}
	if p, _ := s.NextPending(10); len(p) != 1 {
		t.Fatalf("not requeued: %d pending", len(p))
	}
}

func TestRawRoundTrip(t *testing.T) {
	s := openTemp(t)
	payload := []byte(`{"id":"1292052","title":"肖申克的救赎"}`)

	rel, hash, err := s.WriteRaw(SourceFrodo, "movie", "1292052", "json", payload)
	if err != nil {
		t.Fatalf("WriteRaw: %v", err)
	}
	if rel != filepath.Join("raw", "frodo", "movie", "52", "1292052.json.gz") {
		t.Errorf("rel path = %q", rel)
	}
	if _, err := os.Stat(filepath.Join(s.Dir(), rel)); err != nil {
		t.Fatalf("artifact missing: %v", err)
	}
	got, err := s.ReadRaw(rel)
	if err != nil {
		t.Fatalf("ReadRaw: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("round-trip mismatch: %s", got)
	}
	if len(hash) != 64 {
		t.Errorf("hash = %q", hash)
	}
}

func TestRecordsAndExport(t *testing.T) {
	s := openTemp(t)
	_ = s.PutRecord(Record{EntityType: "book", EntityID: "1", Source: SourceFrodo, JSON: json.RawMessage(`{"id":"1","t":"a"}`)})
	_ = s.PutRecord(Record{EntityType: "book", EntityID: "2", Source: SourceHTML, JSON: json.RawMessage(`{"id":"2","t":"b"}`)})
	_ = s.PutRecord(Record{EntityType: "movie", EntityID: "9", Source: SourceFrodo, JSON: json.RawMessage(`{"id":"9","t":"m"}`)})
	// Upsert: same key overwrites.
	_ = s.PutRecord(Record{EntityType: "book", EntityID: "1", Source: SourceHTML, JSON: json.RawMessage(`{"id":"1","t":"a2"}`)})

	counts, err := s.RecordCounts()
	if err != nil {
		t.Fatalf("RecordCounts: %v", err)
	}
	if counts["book"] != 2 || counts["movie"] != 1 {
		t.Fatalf("counts = %v", counts)
	}

	var buf bytes.Buffer
	n, err := s.ExportJSONL(context.Background(), "book", &buf)
	if err != nil {
		t.Fatalf("ExportJSONL: %v", err)
	}
	if n != 2 {
		t.Fatalf("exported %d, want 2", n)
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], `"a2"`) {
		t.Fatalf("export lines = %v", lines)
	}
}

func TestMeta(t *testing.T) {
	s := openTemp(t)
	if _, ok, _ := s.GetMeta("cursor"); ok {
		t.Fatal("cursor should be unset")
	}
	if err := s.SetMeta("cursor", "band:subject:42"); err != nil {
		t.Fatalf("SetMeta: %v", err)
	}
	if err := s.SetMeta("cursor", "band:subject:88"); err != nil {
		t.Fatalf("SetMeta overwrite: %v", err)
	}
	v, ok, err := s.GetMeta("cursor")
	if err != nil || !ok || v != "band:subject:88" {
		t.Fatalf("GetMeta = %q ok=%v err=%v", v, ok, err)
	}
}

func TestFrontierCounts(t *testing.T) {
	s := openTemp(t)
	_, _ = s.Enqueue(Frontier{URL: "a", EntityType: "book", EntityID: "1", Source: SourceHTML})
	_, _ = s.Enqueue(Frontier{URL: "b", EntityType: "book", EntityID: "2", Source: SourceHTML})
	_ = s.Complete("a", StatusDone, 200, "", "", "")

	counts, err := s.FrontierCounts()
	if err != nil {
		t.Fatalf("FrontierCounts: %v", err)
	}
	if counts["book"][StatusDone] != 1 || counts["book"][StatusPending] != 1 {
		t.Fatalf("counts = %v", counts)
	}
}
