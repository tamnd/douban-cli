// Package douban is the library behind the douban command: the HTTP client,
// request shaping, and typed data models for Douban (豆瓣).
//
// The client reads Douban through the public web surfaces that serve without a
// login: subject search, the j/subject_suggest JSON endpoints, book subject
// pages, the Top 250 / chart / now-playing film lists, and curated doulists. No
// API key or authentication is required. It sets a real User-Agent, paces
// requests, and retries transient 429/5xx errors with exponential backoff.
//
// Douban's subdomains differ in anti-bot posture: book subject pages serve
// fully, but movie subject and celebrity pages redirect to a sec.douban.com
// challenge. Film data is therefore reached through suggest and the list pages
// rather than per-film detail pages.
package douban

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// DefaultUserAgent is the browser-like User-Agent sent with every request.
const DefaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

// ErrNotFound is returned when Douban has no subject, list, or page for an id.
var ErrNotFound = errors.New("douban: not found")

// Config holds constructor parameters.
type Config struct {
	// BaseURL overrides the host for tests. When empty the client talks to the
	// real https://<sub>.douban.com subdomains; when set, every host resolves to
	// this single base and is dispatched by path.
	BaseURL   string
	UserAgent string
	Rate      time.Duration
	Timeout   time.Duration
	Retries   int
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		UserAgent: DefaultUserAgent,
		Rate:      500 * time.Millisecond,
		Retries:   3,
		Timeout:   30 * time.Second,
	}
}

// Client talks to Douban's public web surfaces.
type Client struct {
	cfg        Config
	httpClient *http.Client
	mu         sync.Mutex
	last       time.Time
}

// NewClient returns a Client configured with cfg.
func NewClient(cfg Config) *Client {
	return &Client{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: cfg.Timeout},
	}
}

// host returns the base URL for a Douban subdomain ("search", "movie", "book",
// "www"). When cfg.BaseURL is set (tests) every subdomain maps to it.
func (c *Client) host(sub string) string {
	if c.cfg.BaseURL != "" {
		return c.cfg.BaseURL
	}
	return "https://" + sub + ".douban.com"
}

// Get fetches a URL and returns the body, pacing and retrying as configured.
// It is the exported entry point used by the mirror crawler to fetch arbitrary
// Douban URLs (sitemaps, subject pages); the lookup commands use the typed
// helpers built on the same internal get.
func (c *Client) Get(ctx context.Context, rawURL string) ([]byte, error) {
	return c.get(ctx, rawURL)
}

// get fetches a URL and returns the body, pacing and retrying as configured.
func (c *Client) get(ctx context.Context, rawURL string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.cfg.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		b, retry, err := c.do(ctx, rawURL)
		if err == nil {
			return b, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get %s: %w", rawURL, lastErr)
}

func (c *Client) do(ctx context.Context, rawURL string) ([]byte, bool, error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/json")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	switch {
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	case resp.StatusCode == http.StatusNotFound:
		return nil, false, ErrNotFound
	case resp.StatusCode != http.StatusOK:
		return nil, false, fmt.Errorf("http %d", resp.StatusCode)
	}

	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, true, err
	}
	return b, false, nil
}

func (c *Client) pace() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cfg.Rate <= 0 {
		return
	}
	if wait := c.cfg.Rate - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

func backoff(attempt int) time.Duration {
	d := time.Duration(attempt) * 500 * time.Millisecond
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}
