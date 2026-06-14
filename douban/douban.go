// Package douban is the library behind the douban command: the HTTP client,
// request shaping, and typed data models for Douban (豆瓣).
//
// The client fetches Douban subject-search pages over HTTPS and extracts
// structured results from the embedded window.__DATA__ JSON. No API key or
// authentication is required. It sets a real User-Agent, paces requests, and
// retries transient 429/5xx errors with exponential backoff.
package douban

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// DefaultUserAgent is the browser-like User-Agent sent with every request.
const DefaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

// Config holds constructor parameters.
type Config struct {
	BaseURL   string
	UserAgent string
	Rate      time.Duration
	Timeout   time.Duration
	Retries   int
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		BaseURL:   "https://search.douban.com",
		UserAgent: DefaultUserAgent,
		Rate:      500 * time.Millisecond,
		Retries:   3,
		Timeout:   30 * time.Second,
	}
}

// Client talks to Douban's search pages.
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

// Search fetches Douban search results for query and searchType.
// searchType must be one of "book", "movie", or "music".
// Up to limit results are returned; pass 0 to use a reasonable default (15).
func (c *Client) Search(ctx context.Context, query, searchType string, limit int) ([]Result, error) {
	if limit <= 0 {
		limit = 15
	}

	endpoint, cat, err := searchEndpoint(c.cfg.BaseURL, searchType)
	if err != nil {
		return nil, err
	}

	u := fmt.Sprintf("%s?search_text=%s&cat=%s", endpoint, url.QueryEscape(query), cat)
	body, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}

	data, err := extractData(body)
	if err != nil {
		return nil, err
	}

	var results []Result
	rank := 0
	for _, item := range data.Items {
		if item.TplName != "search_subject" {
			continue
		}
		rank++
		results = append(results, toResult(item, rank))
		if rank >= limit {
			break
		}
	}
	return results, nil
}

// searchEndpoint returns the path prefix and cat code for searchType.
func searchEndpoint(baseURL, searchType string) (string, string, error) {
	switch searchType {
	case "book", "":
		return baseURL + "/book/subject_search", "1001", nil
	case "movie":
		return baseURL + "/movie/subject_search", "1002", nil
	case "music":
		return baseURL + "/music/subject_search", "1003", nil
	default:
		return "", "", fmt.Errorf("douban: unknown type %q (want book, movie, or music)", searchType)
	}
}

// toResult converts a wire item to a Result.
func toResult(item doubanItem, rank int) Result {
	rating := "-"
	if item.Rating.Value != 0 {
		rating = fmt.Sprintf("%.1f (%d)", item.Rating.Value, item.Rating.Count)
	}

	abstract := item.Abstract
	if len([]rune(abstract)) > 100 {
		runes := []rune(abstract)
		abstract = string(runes[:100]) + "…"
	}

	return Result{
		Rank:     rank,
		Title:    item.Title,
		Rating:   rating,
		Abstract: abstract,
		URL:      item.URL,
	}
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
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
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
