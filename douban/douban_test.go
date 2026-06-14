package douban

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// mockHTML wraps JSON in a minimal Douban-style HTML page.
func mockHTML(dataJSON string) string {
	return `<!DOCTYPE html><html><head><title>搜索</title></head><body>
<script>
window.__DATA__ = ` + dataJSON + `;
</script>
</body></html>`
}

const mockDataJSON = `{"count":3,"total":100,"items":[
  {"tpl_name":"search_subject","title":"Python编程","url":"https://book.douban.com/subject/1/","abstract":"作者 / 出版社 / 2023","rating":{"value":9.1,"count":707,"star_count":4.5}},
  {"tpl_name":"nav_link","title":"下一页","url":"https://search.douban.com/next"},
  {"tpl_name":"search_subject","title":"流浪地球","url":"https://movie.douban.com/subject/2/","abstract":"导演: 郭帆 / 2019","rating":{"value":0,"count":0,"star_count":0}}
]}`

func newTestClient(ts *httptest.Server) *Client {
	cfg := DefaultConfig()
	cfg.BaseURL = ts.URL
	cfg.Rate = 0
	return NewClient(cfg)
}

func TestSearchSendsUserAgent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("request carried no User-Agent")
		}
		_, _ = w.Write([]byte(mockHTML(mockDataJSON)))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.Search(context.Background(), "python", "book", 10)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSearchFiltersNonSubjectItems(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(mockHTML(mockDataJSON)))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	results, err := c.Search(context.Background(), "python", "book", 10)
	if err != nil {
		t.Fatal(err)
	}
	// Only 2 of the 3 items have tpl_name == "search_subject"
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2 (nav_link must be filtered)", len(results))
	}
}

func TestSearchRating(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(mockHTML(mockDataJSON)))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	results, err := c.Search(context.Background(), "test", "book", 10)
	if err != nil {
		t.Fatal(err)
	}

	// First result has rating 9.1 with 707 reviews
	if results[0].Rating != "9.1 (707)" {
		t.Errorf("rating = %q, want %q", results[0].Rating, "9.1 (707)")
	}
	// Second result has zero rating
	if results[1].Rating != "-" {
		t.Errorf("rating = %q, want %q", results[1].Rating, "-")
	}
}

func TestSearchRank(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(mockHTML(mockDataJSON)))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	results, err := c.Search(context.Background(), "test", "book", 10)
	if err != nil {
		t.Fatal(err)
	}
	if results[0].Rank != 1 {
		t.Errorf("rank[0] = %d, want 1", results[0].Rank)
	}
	if results[1].Rank != 2 {
		t.Errorf("rank[1] = %d, want 2", results[1].Rank)
	}
}

func TestSearchLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(mockHTML(mockDataJSON)))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	results, err := c.Search(context.Background(), "test", "book", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results with limit=1, want 1", len(results))
	}
}

func TestSearchAbstractTruncation(t *testing.T) {
	longAbstract := "这是一个非常长的摘要文本，用于测试截断功能是否正常工作。这个文本超过一百个字符，应该被截断并添加省略号在末尾以便显示。这里有更多的文字来确保超过限制。作者名字 / 出版社名称 / 2023年出版 / 定价99.00元 / 北京出版集团出版。"
	data := `{"count":1,"total":1,"items":[
		{"tpl_name":"search_subject","title":"Test","url":"https://example.com/","abstract":"` + longAbstract + `","rating":{"value":0,"count":0,"star_count":0}}
	]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(mockHTML(data)))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	results, err := c.Search(context.Background(), "test", "book", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("got 0 results")
	}
	runes := []rune(results[0].Abstract)
	// Truncated to 100 runes + "…"
	if len(runes) != 101 {
		t.Errorf("abstract rune length = %d, want 101 (100 + ellipsis)", len(runes))
	}
}

func TestSearchMovieEndpoint(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(mockHTML(`{"count":0,"total":0,"items":[]}`)))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.Search(context.Background(), "流浪地球", "movie", 10)
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/movie/subject_search" {
		t.Errorf("path = %q, want /movie/subject_search", gotPath)
	}
}

func TestSearchMusicEndpoint(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(mockHTML(`{"count":0,"total":0,"items":[]}`)))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.Search(context.Background(), "周杰伦", "music", 10)
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/music/subject_search" {
		t.Errorf("path = %q, want /music/subject_search", gotPath)
	}
}

func TestSearchUnknownType(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Rate = 0
	c := NewClient(cfg)
	_, err := c.Search(context.Background(), "test", "podcast", 10)
	if err == nil {
		t.Fatal("expected error for unknown type, got nil")
	}
}

func TestExtractDataMissing(t *testing.T) {
	_, err := extractData([]byte("<html>no data here</html>"))
	if err == nil {
		t.Fatal("expected error when __DATA__ missing, got nil")
	}
}

func TestSearchRetriesOn503(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(mockHTML(`{"count":0,"total":0,"items":[]}`)))
	}))
	defer srv.Close()

	cfg := DefaultConfig()
	cfg.BaseURL = srv.URL
	cfg.Rate = 0
	cfg.Retries = 5
	c := NewClient(cfg)

	_, err := c.Search(context.Background(), "test", "book", 10)
	if err != nil {
		t.Fatal(err)
	}
	if hits != 3 {
		t.Errorf("server saw %d hits, want 3 (2 503s then success)", hits)
	}
}
