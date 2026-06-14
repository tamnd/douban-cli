package douban

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

// Frodo is the Douban mobile-app JSON API at frodo.douban.com/api/v2. Unlike the
// desktop web, it serves clean structured JSON for every subject type, including
// the movie/TV/celebrity detail that the desktop redirects to sec.douban.com. It
// requires an HMAC-SHA1 request signature; signing it is what lets the mirror
// read those surfaces.
//
// Signing recipe (verified live): sig = base64(HMAC-SHA1(secret,
// "GET&"+urlencode(path)+"&"+ts)) where path includes the /api prefix and ts is
// unix seconds. The request carries apikey, _ts, and _sig query parameters.

const (
	// DefaultFrodoBase is the Frodo API origin.
	DefaultFrodoBase = "https://frodo.douban.com"
	// DefaultFrodoKey and DefaultFrodoSecret are a recognized Frodo app key pair.
	// They can be overridden via FrodoConfig (or CLI flags) if Douban rotates them.
	DefaultFrodoKey    = "0dad551ec0f84ed02907ff5c42e8ec70"
	DefaultFrodoSecret = "bf7dddc7c9cfe6f7"
	// DefaultFrodoUA is the Frodo Android client User-Agent.
	DefaultFrodoUA = "com.douban.frodo/7.21.0(214) Android/29 product/blueline vendor/Google model/Pixel 3 rom/android network/wifi  platform/mobile nd/1"
)

// FrodoConfig configures a FrodoClient.
type FrodoConfig struct {
	// BaseURL overrides the API origin for tests. Empty means DefaultFrodoBase.
	BaseURL   string
	APIKey    string
	Secret    string
	UserAgent string
	Rate      time.Duration
	Timeout   time.Duration
	Retries   int
}

// DefaultFrodoConfig returns a working configuration. The Frodo host tolerates a
// faster pace than the web hosts, so Rate defaults below the web Crawl-delay.
func DefaultFrodoConfig() FrodoConfig {
	return FrodoConfig{
		BaseURL:   DefaultFrodoBase,
		APIKey:    DefaultFrodoKey,
		Secret:    DefaultFrodoSecret,
		UserAgent: DefaultFrodoUA,
		Rate:      time.Second,
		Timeout:   30 * time.Second,
		Retries:   3,
	}
}

// FrodoError is the API's error envelope: a JSON body carrying a numeric code,
// e.g. 996 (signature error), 997 (unknown apikey), 1062 (rate limited).
type FrodoError struct {
	Status    int    `json:"-"`
	Code      int    `json:"code"`
	Request   string `json:"request"`
	Msg       string `json:"msg"`
	Localized string `json:"localized_message"`
}

func (e *FrodoError) Error() string {
	msg := e.Msg
	if msg == "" {
		msg = e.Localized
	}
	return fmt.Sprintf("frodo: http %d code %d: %s (%s)", e.Status, e.Code, msg, e.Request)
}

// FrodoClient calls the signed Frodo API.
type FrodoClient struct {
	cfg        FrodoConfig
	httpClient *http.Client
	now        func() time.Time
	mu         sync.Mutex
	last       time.Time
}

// NewFrodoClient returns a FrodoClient. Empty config fields fall back to the
// DefaultFrodoConfig values.
func NewFrodoClient(cfg FrodoConfig) *FrodoClient {
	def := DefaultFrodoConfig()
	if cfg.APIKey == "" {
		cfg.APIKey = def.APIKey
	}
	if cfg.Secret == "" {
		cfg.Secret = def.Secret
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = def.UserAgent
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = def.Timeout
	}
	return &FrodoClient{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: cfg.Timeout},
		now:        time.Now,
	}
}

// sign computes the _sig value for a path signed at unix time ts.
func (f *FrodoClient) sign(path string, ts int64) string {
	raw := "GET&" + url.QueryEscape(path) + "&" + strconv.FormatInt(ts, 10)
	mac := hmac.New(sha1.New, []byte(f.cfg.Secret))
	_, _ = mac.Write([]byte(raw))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func (f *FrodoClient) base() string {
	if f.cfg.BaseURL != "" {
		return f.cfg.BaseURL
	}
	return DefaultFrodoBase
}

// signedURL builds the full signed request URL for path with extra params.
func (f *FrodoClient) signedURL(path string, params url.Values) string {
	ts := f.now().Unix()
	q := url.Values{}
	for k, vs := range params {
		for _, v := range vs {
			q.Add(k, v)
		}
	}
	q.Set("apikey", f.cfg.APIKey)
	q.Set("_ts", strconv.FormatInt(ts, 10))
	q.Set("_sig", f.sign(path, ts))
	return f.base() + path + "?" + q.Encode()
}

// Get fetches a signed API path and returns the raw JSON body. It paces and
// retries transient failures; a 404 maps to ErrNotFound and an API error
// envelope to *FrodoError.
func (f *FrodoClient) Get(ctx context.Context, path string, params url.Values) ([]byte, error) {
	var lastErr error
	retries := f.cfg.Retries
	for attempt := 0; attempt <= retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		b, retry, err := f.do(ctx, path, params)
		if err == nil {
			return b, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("frodo get %s: %w", path, lastErr)
}

func (f *FrodoClient) do(ctx context.Context, path string, params url.Values) ([]byte, bool, error) {
	f.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.signedURL(path, params), nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", f.cfg.UserAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, true, err
	}

	switch {
	case resp.StatusCode == http.StatusOK:
		return body, false, nil
	case resp.StatusCode == http.StatusNotFound:
		return nil, false, ErrNotFound
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		return nil, true, f.apiError(resp.StatusCode, body)
	default:
		return nil, false, f.apiError(resp.StatusCode, body)
	}
}

// apiError decodes the error envelope, falling back to a bare status error.
func (f *FrodoClient) apiError(status int, body []byte) error {
	var fe FrodoError
	if err := json.Unmarshal(body, &fe); err == nil && fe.Code != 0 {
		fe.Status = status
		return &fe
	}
	return fmt.Errorf("frodo: http %d", status)
}

func (f *FrodoClient) pace() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.cfg.Rate <= 0 {
		return
	}
	if wait := f.cfg.Rate - time.Since(f.last); wait > 0 {
		time.Sleep(wait)
	}
	f.last = f.now()
}

// --- Endpoint helpers (return raw JSON for archival; callers normalize) ---

// Subject fetches a subject of the given Frodo type ("movie", "book", "music",
// "game"). Movie covers TV as well.
func (f *FrodoClient) Subject(ctx context.Context, typ, id string) ([]byte, error) {
	return f.Get(ctx, "/api/v2/"+typ+"/"+id, nil)
}

// Movie, Book, Music, Game fetch a subject detail of that type.
func (f *FrodoClient) Movie(ctx context.Context, id string) ([]byte, error) {
	return f.Subject(ctx, "movie", id)
}
func (f *FrodoClient) Book(ctx context.Context, id string) ([]byte, error) {
	return f.Subject(ctx, "book", id)
}
func (f *FrodoClient) Music(ctx context.Context, id string) ([]byte, error) {
	return f.Subject(ctx, "music", id)
}
func (f *FrodoClient) Game(ctx context.Context, id string) ([]byte, error) {
	return f.Subject(ctx, "game", id)
}

// Personage fetches the rich celebrity/personage profile (elessar subject).
func (f *FrodoClient) Personage(ctx context.Context, id string) ([]byte, error) {
	return f.Get(ctx, "/api/v2/elessar/subject/"+id, nil)
}

// Interests returns the public per-user ratings and short comments for a subject.
func (f *FrodoClient) Interests(ctx context.Context, typ, id string, start, count int) ([]byte, error) {
	return f.Get(ctx, "/api/v2/"+typ+"/"+id+"/interests", pageParams(start, count))
}

// Reviews returns the long reviews for a subject.
func (f *FrodoClient) Reviews(ctx context.Context, typ, id string, start, count int) ([]byte, error) {
	return f.Get(ctx, "/api/v2/"+typ+"/"+id+"/reviews", pageParams(start, count))
}

// Photos returns the photo gallery for a subject.
func (f *FrodoClient) Photos(ctx context.Context, typ, id string, start, count int) ([]byte, error) {
	return f.Get(ctx, "/api/v2/"+typ+"/"+id+"/photos", pageParams(start, count))
}

// Rating returns the rating breakdown (star distribution, wish/doing/done) for a subject.
func (f *FrodoClient) Rating(ctx context.Context, typ, id string) ([]byte, error) {
	return f.Get(ctx, "/api/v2/"+typ+"/"+id+"/rating", nil)
}

// Celebrities returns the directors/actors credit list for a movie.
func (f *FrodoClient) Celebrities(ctx context.Context, id string) ([]byte, error) {
	return f.Get(ctx, "/api/v2/movie/"+id+"/celebrities", nil)
}

// SearchSubjects searches across subjects; filter results by item.target_type.
func (f *FrodoClient) SearchSubjects(ctx context.Context, q string, start, count int) ([]byte, error) {
	p := pageParams(start, count)
	p.Set("q", q)
	return f.Get(ctx, "/api/v2/search/subjects", p)
}

func pageParams(start, count int) url.Values {
	v := url.Values{}
	if start > 0 {
		v.Set("start", strconv.Itoa(start))
	}
	if count > 0 {
		v.Set("count", strconv.Itoa(count))
	}
	return v
}
