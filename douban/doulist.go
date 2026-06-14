package douban

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	reDLTitle    = regexp.MustCompile(`(?s)<div class="title">\s*<a[^>]*>(.*?)</a>`)
	reDLNums     = regexp.MustCompile(`(?s)class="rating_nums">(.*?)<`)
	reDLAbstract = regexp.MustCompile(`(?s)<div class="abstract">(.*?)</div>`)
)

// Doulist returns the subjects in a curated list (豆列) in list order. The
// argument may be a bare id or a pasted www.douban.com/doulist URL. It walks the
// paged ?start= listing until limit items are collected (default 100).
func (c *Client) Doulist(ctx context.Context, idOrURL string, limit int) ([]DoulistItem, error) {
	id := doulistID(idOrURL)
	if id == "" {
		return nil, fmt.Errorf("douban: invalid doulist id %q", idOrURL)
	}
	if limit <= 0 {
		limit = 100
	}

	var out []DoulistItem
	for start := 0; len(out) < limit; start += 25 {
		u := fmt.Sprintf("%s/doulist/%s/?start=%d", c.host("www"), id, start)
		body, err := c.get(ctx, u)
		if err != nil {
			return nil, err
		}
		page := parseDoulist(string(body), len(out)+1)
		if len(page) == 0 {
			break
		}
		out = append(out, page...)
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func parseDoulist(h string, startRank int) []DoulistItem {
	var out []DoulistItem
	rank := startRank
	for _, chunk := range strings.Split(h, `class="doulist-item"`)[1:] {
		url := firstURLSubject(chunk)
		if url == "" {
			continue
		}
		votes := 0
		if v := reTop250Votes.FindStringSubmatch(chunk); v != nil {
			votes, _ = strconv.Atoi(v[1])
		}
		avg, _ := strconv.ParseFloat(firstGroup(reDLNums, chunk), 64)
		out = append(out, DoulistItem{
			Rank:     rank,
			ID:       subjectID(url),
			Title:    firstGroup(reDLTitle, chunk),
			Rating:   fmtRating(avg, votes),
			Abstract: truncRunes(firstGroup(reDLAbstract, chunk), 100),
			URL:      subjectURL(url),
			Cover:    posterImg(chunk),
		})
		rank++
	}
	return out
}

var (
	reDoulist = regexp.MustCompile(`/doulist/(\d+)/?`)
	rePoster  = regexp.MustCompile(`<img[^>]+src="([^"]*doubanio\.com/view/[^"]+)"`)
)

// posterImg returns the subject poster (a doubanio /view/ image) from a chunk,
// preferring it over decorative icons; falls back to the first image.
func posterImg(chunk string) string {
	if m := rePoster.FindStringSubmatch(chunk); m != nil {
		return m[1]
	}
	return firstMatch(reImg, chunk)
}

// doulistID extracts a doulist id from a bare id or a pasted URL.
func doulistID(idOrURL string) string {
	if m := reDoulist.FindStringSubmatch(idOrURL); m != nil {
		return m[1]
	}
	s := strings.TrimSpace(idOrURL)
	if s != "" && !strings.ContainsAny(s, "/?") {
		return s
	}
	return ""
}
