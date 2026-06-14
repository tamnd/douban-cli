package douban

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"
)

// TestFrodoSign locks the signing recipe to a known vector computed by hand:
// secret bf7dddc7c9cfe6f7, path /api/v2/movie/1292052, ts 1700000000.
func TestFrodoSign(t *testing.T) {
	f := NewFrodoClient(FrodoConfig{Secret: "bf7dddc7c9cfe6f7"})
	got := f.sign("/api/v2/movie/1292052", 1700000000)
	want := "6Jvrc63BOBqIRrQO5XdiGmUWg+4="
	if got != want {
		t.Fatalf("sign = %q, want %q", got, want)
	}
}

// TestFrodoSignedURL checks that apikey, _ts, and _sig ride along url-encoded.
func TestFrodoSignedURL(t *testing.T) {
	f := NewFrodoClient(FrodoConfig{
		BaseURL: "https://frodo.douban.com",
		APIKey:  "testkey",
		Secret:  "bf7dddc7c9cfe6f7",
	})
	f.now = func() time.Time { return time.Unix(1700000000, 0) }

	raw := f.signedURL("/api/v2/movie/1292052", url.Values{"count": {"5"}})
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}
	if u.Path != "/api/v2/movie/1292052" {
		t.Fatalf("path = %q", u.Path)
	}
	q := u.Query()
	if q.Get("apikey") != "testkey" {
		t.Errorf("apikey = %q", q.Get("apikey"))
	}
	if q.Get("_ts") != "1700000000" {
		t.Errorf("_ts = %q", q.Get("_ts"))
	}
	if q.Get("_sig") != "6Jvrc63BOBqIRrQO5XdiGmUWg+4=" {
		t.Errorf("_sig = %q", q.Get("_sig"))
	}
	if q.Get("count") != "5" {
		t.Errorf("count = %q", q.Get("count"))
	}
}

// TestFrodoGet runs a Movie fetch against an httptest server that validates the
// signed request and replies with the captured Frodo movie fixture.
func TestFrodoGet(t *testing.T) {
	fixture, err := os.ReadFile("testdata/frodo_movie_1292052.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/movie/1292052" {
			http.Error(w, "wrong path", http.StatusNotFound)
			return
		}
		q := r.URL.Query()
		if q.Get("apikey") == "" || q.Get("_ts") == "" || q.Get("_sig") == "" {
			http.Error(w, "missing signature params", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}))
	defer srv.Close()

	f := NewFrodoClient(FrodoConfig{BaseURL: srv.URL, Rate: 0})
	body, err := f.Movie(context.Background(), "1292052")
	if err != nil {
		t.Fatalf("Movie: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		t.Fatalf("fixture not valid JSON through client: %v", err)
	}
	if obj["id"] != "1292052" {
		t.Errorf("id = %v, want 1292052", obj["id"])
	}
}

// TestFrodoNotFound maps a 404 to ErrNotFound.
func TestFrodoNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no", http.StatusNotFound)
	}))
	defer srv.Close()

	f := NewFrodoClient(FrodoConfig{BaseURL: srv.URL, Rate: 0, Retries: 0})
	if _, err := f.Movie(context.Background(), "1"); err != ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// TestFrodoAPIError decodes the error envelope on a non-retryable 4xx.
func TestFrodoAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":996,"request":"GET /api/v2/movie/1","msg":"invalid_signature"}`))
	}))
	defer srv.Close()

	f := NewFrodoClient(FrodoConfig{BaseURL: srv.URL, Rate: 0, Retries: 0})
	_, err := f.Movie(context.Background(), "1")
	fe, ok := err.(*FrodoError)
	if !ok {
		t.Fatalf("err = %T %v, want *FrodoError", err, err)
	}
	if fe.Code != 996 || fe.Status != http.StatusBadRequest {
		t.Errorf("FrodoError = %+v", fe)
	}
}
