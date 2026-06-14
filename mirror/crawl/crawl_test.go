package crawl

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/tamnd/douban-cli/douban"
	"github.com/tamnd/douban-cli/mirror/store"
)

// rewriteClient sends a real douban URL to the test server, preserving the path.
// It lets the engine fetch absolute douban URLs while the test serves them.
type rewriteClient struct{ base string }

func (rc rewriteClient) Get(ctx context.Context, raw string) ([]byte, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	b, _ := url.Parse(rc.base)
	u.Scheme, u.Host = b.Scheme, b.Host
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return nil, douban.ErrNotFound
	}
	if resp.StatusCode == http.StatusForbidden {
		return nil, douban.ErrForbidden
	}
	return io.ReadAll(resp.Body)
}

func testServer() *httptest.Server {
	mux := http.NewServeMux()
	// Frodo API surfaces.
	mux.HandleFunc("/api/v2/movie/1292052", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":"1292052","title":"肖申克的救赎","type":"movie"}`))
	})
	mux.HandleFunc("/api/v2/elessar/subject/27260288", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":"27260288","title":"演员","type":"personage"}`))
	})
	// A TV series rejects the /movie/ endpoint with code 996 and serves on /tv/.
	mux.HandleFunc("/api/v2/movie/35651341", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":996,"msg":"invalid_request_996","request":"GET /api/v2/tv/35651341"}`))
	})
	mux.HandleFunc("/api/v2/tv/35651341", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":"35651341","title":"三体","type":"tv"}`))
	})
	mux.HandleFunc("/api/v2/note/1", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"code":1234,"msg":"need_login","request":"GET /api/v2/note/1"}`))
	})
	// HTML book page with a link to another entity, a sub-page link that must
	// collapse to the canonical subject, and a non-entity link.
	mux.HandleFunc("/subject/1084336/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><body>
			<h1>未来简史</h1>
			<a href="https://music.douban.com/subject/999/">a soundtrack</a>
			<a href="https://music.douban.com/subject/999/reviews?sort=new">its reviews</a>
			<a href="https://book.douban.com/tag/小说">tag browse</a>
		</body></html>`))
	})
	// A login-gated people profile answers 403.
	mux.HandleFunc("/people/777/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	return httptest.NewServer(mux)
}

func TestEngineRun(t *testing.T) {
	srv := testServer()
	defer srv.Close()

	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = st.Close() }()

	seeds := []store.Frontier{
		{URL: "https://book.douban.com/subject/1084336/", EntityType: "book", EntityID: "1084336", Source: store.SourceHTML},
		{URL: "https://movie.douban.com/subject/1292052/", EntityType: "movie", EntityID: "1292052", Source: store.SourceFrodo},
		{URL: "https://www.douban.com/personage/27260288/", EntityType: "personage", EntityID: "27260288", Source: store.SourceFrodo},
		{URL: "https://www.douban.com/note/1/", EntityType: "note", EntityID: "1", Source: store.SourceFrodo},
	}
	for _, s := range seeds {
		if _, err := st.Enqueue(s); err != nil {
			t.Fatalf("Enqueue: %v", err)
		}
	}

	frodo := douban.NewFrodoClient(douban.FrodoConfig{BaseURL: srv.URL, Rate: 0, Retries: 0})
	eng := New(st, rewriteClient{base: srv.URL}, frodo, Config{
		Concurrency: 2,
		Limit:       4, // only the seeds; the discovered link stays pending
		HTMLDelay:   0,
	})

	stats, err := eng.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Processed != 4 {
		t.Fatalf("processed = %d, want 4", stats.Processed)
	}
	if stats.Done != 3 {
		t.Errorf("done = %d, want 3", stats.Done)
	}
	if stats.Blocked != 1 {
		t.Errorf("blocked = %d, want 1 (note)", stats.Blocked)
	}
	if stats.Discovered != 1 {
		t.Errorf("discovered = %d, want 1 (music link)", stats.Discovered)
	}

	// Raw artifacts written for both sources.
	if _, err := st.ReadRaw("raw/frodo/movie/52/1292052.json.gz"); err != nil {
		t.Errorf("movie raw missing: %v", err)
	}
	if _, err := st.ReadRaw("raw/html/book/36/1084336.html.gz"); err != nil {
		t.Errorf("book raw missing: %v", err)
	}

	// Records present for the fetched entities.
	rc, _ := st.RecordCounts()
	if rc["movie"] != 1 || rc["book"] != 1 || rc["personage"] != 1 {
		t.Errorf("record counts = %v", rc)
	}

	// Frontier transitions.
	fc, _ := st.FrontierCounts()
	if fc["note"][store.StatusBlocked] != 1 {
		t.Errorf("note not blocked: %v", fc["note"])
	}
	if fc["book"][store.StatusDone] != 1 {
		t.Errorf("book not done: %v", fc["book"])
	}

	// The discovered music link is enqueued and still pending (under the limit).
	pend, _ := st.Pending(nil, "", 100)
	var foundMusic bool
	for _, p := range pend {
		if strings.Contains(p.URL, "music.douban.com/subject/999") {
			foundMusic = true
			if p.EntityType != "music" || p.Source != store.SourceHTML {
				t.Errorf("discovered classify = %+v", p)
			}
		}
	}
	if !foundMusic {
		t.Error("discovered music link not enqueued")
	}
}

// TestEngineResume runs a second pass that picks up the link discovered in the
// TestEngineForbiddenIsBlocked checks that a 403 (a wall, login-gated or
// anti-bot) is recorded as blocked, not failed, so reset-failed does not churn
// on it forever.
func TestEngineForbiddenIsBlocked(t *testing.T) {
	srv := testServer()
	defer srv.Close()

	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = st.Close() }()

	_, _ = st.Enqueue(store.Frontier{
		URL: "https://www.douban.com/people/777/", EntityType: "people", EntityID: "777", Source: store.SourceHTML})

	eng := New(st, rewriteClient{base: srv.URL}, nil, Config{Concurrency: 1, HTMLDelay: 0})
	stats, err := eng.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Blocked != 1 || stats.Failed != 0 {
		t.Fatalf("blocked=%d failed=%d, want blocked=1 failed=0", stats.Blocked, stats.Failed)
	}
	rows, _ := st.QueueRows("blocked", "people", 10)
	if len(rows) != 1 || rows[0].HTTPStatus != 403 {
		t.Errorf("blocked row = %+v, want one with http 403", rows)
	}
}

// TestEngineMovieToTVFallback covers a TV series seeded from a
// movie.douban.com/subject/ URL: the /movie/ endpoint answers code 996, and the
// engine must retry /tv/ and record the entity as done rather than blocked.
func TestEngineMovieToTVFallback(t *testing.T) {
	srv := testServer()
	defer srv.Close()

	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = st.Close() }()

	_, _ = st.Enqueue(store.Frontier{
		URL: "https://movie.douban.com/subject/35651341/", EntityType: "movie", EntityID: "35651341", Source: store.SourceFrodo})

	frodo := douban.NewFrodoClient(douban.FrodoConfig{BaseURL: srv.URL, Rate: 0, Retries: 0})
	eng := New(st, rewriteClient{base: srv.URL}, frodo, Config{Concurrency: 1})

	stats, err := eng.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Done != 1 || stats.Blocked != 0 {
		t.Fatalf("done=%d blocked=%d, want done=1 blocked=0 (tv fallback)", stats.Done, stats.Blocked)
	}
	if rc, _ := st.RecordCounts(); rc["movie"] != 1 {
		t.Errorf("tv series not recorded: %v", rc)
	}
}

// first, confirming the frontier resumes cleanly.
func TestEngineResume(t *testing.T) {
	srv := testServer()
	defer srv.Close()
	// Serve the discovered subject this time.
	mux := srv.Config.Handler.(*http.ServeMux)
	mux.HandleFunc("/subject/999/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><body><h1>soundtrack</h1></body></html>`))
	})

	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = st.Close() }()

	_, _ = st.Enqueue(store.Frontier{
		URL: "https://book.douban.com/subject/1084336/", EntityType: "book", EntityID: "1084336", Source: store.SourceHTML})

	eng := New(st, rewriteClient{base: srv.URL}, nil, Config{Concurrency: 1, HTMLDelay: 0})

	// First pass fetches the book and discovers the music link.
	s1, err := eng.Run(context.Background())
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if s1.Discovered != 1 {
		t.Fatalf("first run discovered = %d, want 1", s1.Discovered)
	}
	// No limit, so the same Run drains the discovered link too.
	rc, _ := st.RecordCounts()
	if rc["music"] != 1 {
		t.Errorf("music not crawled on drain: %v", rc)
	}
	if pend, _ := st.Pending(nil, "", 10); len(pend) != 0 {
		t.Errorf("frontier not drained: %d pending", len(pend))
	}
}
