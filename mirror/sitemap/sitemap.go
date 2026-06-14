// Package sitemap enumerates Douban's crawlable URL space from its sitemap
// backbone. There is exactly one backbone for the whole site
// (www.douban.com/sitemap_index.xml); every subdomain's robots.txt points at it.
// The index lists 10,706 gzipped child sitemaps of 50,000 URLs each, partitioned
// into contiguous bands by entity type, plus a daily updated index for freshness.
package sitemap

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/xml"
	"fmt"
	"io"
)

// The site-wide sitemap entry points.
const (
	IndexURL        = "https://www.douban.com/sitemap_index.xml"
	UpdatedIndexURL = "https://www.douban.com/sitemap_updated_index.xml"
)

// Band names a contiguous run of child sitemaps holding one entity type. The
// range is [Start, End) over the ordered child list. Boundaries drift as the
// catalog grows, so a crawl should sniff edge files before trusting them.
type Band struct {
	Name  string
	Start int
	End   int
}

// Bands is the approximate entity partition of the full index (probed 2026-06-14).
var Bands = []Band{
	{"subject", 0, 88},
	{"review", 88, 285},
	{"group-topic", 285, 4890},
	{"event", 4890, 4925},
	{"people", 4925, 9995},
	{"note", 9995, 10635},
	{"celebrity", 10640, 10704},
	{"musician", 10705, 10706},
}

// BandByName returns the named band, or ok=false if unknown.
func BandByName(name string) (Band, bool) {
	for _, b := range Bands {
		if b.Name == name {
			return b, true
		}
	}
	return Band{}, false
}

// Fetcher retrieves a URL's bytes. *douban.Client and *douban.FrodoClient both
// satisfy a compatible shape; the crawl layer adapts whichever it uses.
type Fetcher interface {
	Get(ctx context.Context, url string) ([]byte, error)
}

// urlset and sitemapindex mirror the sitemaps.org schema.
type urlset struct {
	XMLName xml.Name `xml:"urlset"`
	URLs    []struct {
		Loc string `xml:"loc"`
	} `xml:"url"`
}

type sitemapindex struct {
	XMLName  xml.Name `xml:"sitemapindex"`
	Sitemaps []struct {
		Loc string `xml:"loc"`
	} `xml:"sitemap"`
}

// ParseIndex parses a sitemap index document into its ordered child loc URLs.
func ParseIndex(data []byte) ([]string, error) {
	var idx sitemapindex
	if err := xml.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("sitemap: parse index: %w", err)
	}
	out := make([]string, 0, len(idx.Sitemaps))
	for _, s := range idx.Sitemaps {
		if loc := trim(s.Loc); loc != "" {
			out = append(out, loc)
		}
	}
	return out, nil
}

// ParseURLSet parses a (decompressed) child sitemap into its loc URLs.
func ParseURLSet(data []byte) ([]string, error) {
	var us urlset
	if err := xml.Unmarshal(data, &us); err != nil {
		return nil, fmt.Errorf("sitemap: parse urlset: %w", err)
	}
	out := make([]string, 0, len(us.URLs))
	for _, u := range us.URLs {
		if loc := trim(u.Loc); loc != "" {
			out = append(out, loc)
		}
	}
	return out, nil
}

// Gunzip decompresses gzip data, passing already-plain XML through unchanged so
// callers need not know whether a child arrived compressed.
func Gunzip(data []byte) ([]byte, error) {
	if len(data) < 2 || data[0] != 0x1f || data[1] != 0x8b {
		return data, nil
	}
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("sitemap: gunzip: %w", err)
	}
	defer func() { _ = gz.Close() }()
	return io.ReadAll(gz)
}

// Index fetches and parses the child loc list from an index URL.
func Index(ctx context.Context, f Fetcher, indexURL string) ([]string, error) {
	body, err := f.Get(ctx, indexURL)
	if err != nil {
		return nil, err
	}
	return ParseIndex(body)
}

// Children fetches a child sitemap, decompresses it, and returns its loc URLs.
func Children(ctx context.Context, f Fetcher, childURL string) ([]string, error) {
	body, err := f.Get(ctx, childURL)
	if err != nil {
		return nil, err
	}
	plain, err := Gunzip(body)
	if err != nil {
		return nil, err
	}
	return ParseURLSet(plain)
}

// SelectBand returns the children of the named band from an ordered index list,
// clamped to the list bounds. An unknown band name returns nil.
func SelectBand(children []string, name string) []string {
	b, ok := BandByName(name)
	if !ok {
		return nil
	}
	start, end := b.Start, b.End
	if start > len(children) {
		start = len(children)
	}
	if end > len(children) {
		end = len(children)
	}
	if start >= end {
		return nil
	}
	return children[start:end]
}

func trim(s string) string {
	return string(bytes.TrimSpace([]byte(s)))
}
