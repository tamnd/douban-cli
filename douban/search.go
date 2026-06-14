package douban

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// Search fetches Douban subject-search results for query and searchType.
// searchType must be one of "book", "movie", or "music". Up to limit results are
// returned; pass 0 to use a reasonable default (15).
func (c *Client) Search(ctx context.Context, query, searchType string, limit int) ([]Result, error) {
	if limit <= 0 {
		limit = 15
	}
	cat, ok := searchCat(searchType)
	if !ok {
		return nil, fmt.Errorf("douban: unknown type %q (want book, movie, or music)", searchType)
	}
	typ := searchType
	if typ == "" {
		typ = "book"
	}

	u := fmt.Sprintf("%s/%s/subject_search?search_text=%s&cat=%s",
		c.host("search"), typ, url.QueryEscape(query), cat)
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

// searchCat maps a content type to Douban's cat code.
func searchCat(searchType string) (string, bool) {
	switch searchType {
	case "book", "":
		return "1001", true
	case "movie":
		return "1002", true
	case "music":
		return "1003", true
	default:
		return "", false
	}
}

// toResult converts a wire search item to a Result.
func toResult(item doubanItem, rank int) Result {
	return Result{
		Rank:     rank,
		ID:       subjectID(item.URL),
		Type:     subjectType(item.URL),
		Title:    clean(item.Title),
		Rating:   fmtRating(item.Rating.Value, item.Rating.Count),
		Abstract: truncRunes(clean(item.Abstract), 100),
		URL:      subjectURL(item.URL),
		Cover:    item.CoverURL,
	}
}

// subjectType infers the catalog (book/movie/music) from a subject URL host.
func subjectType(rawURL string) string {
	switch {
	case strings.Contains(rawURL, "book.douban.com"):
		return "book"
	case strings.Contains(rawURL, "movie.douban.com"):
		return "movie"
	case strings.Contains(rawURL, "music.douban.com"):
		return "music"
	default:
		return ""
	}
}

// Suggest queries Douban's j/subject_suggest typeahead. suggestType selects the
// catalog ("movie" or "book"); movie is the reliable way to resolve a film id,
// since movie subject pages are sealed. Up to limit results are returned.
func (c *Client) Suggest(ctx context.Context, query, suggestType string, limit int) ([]Suggestion, error) {
	var sub string
	switch suggestType {
	case "movie", "":
		sub = "movie"
	case "book":
		sub = "book"
	default:
		return nil, fmt.Errorf("douban: unknown type %q (want movie or book)", suggestType)
	}

	u := fmt.Sprintf("%s/j/subject_suggest?q=%s", c.host(sub), url.QueryEscape(query))
	body, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}

	var wire []wireSuggest
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, fmt.Errorf("douban: unmarshal suggest: %w", err)
	}

	out := make([]Suggestion, 0, len(wire))
	for _, w := range wire {
		out = append(out, Suggestion{
			ID:       w.ID,
			Type:     w.Type,
			Title:    clean(w.Title),
			SubTitle: clean(w.SubTitle),
			Year:     w.Year,
			URL:      subjectURL(w.URL),
			Img:      w.Img,
		})
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}
