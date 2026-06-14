package douban

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	reTop250Avg   = regexp.MustCompile(`(?s)property="v:average"[^>]*>(.*?)<`)
	reTop250Votes = regexp.MustCompile(`(\d+)人评价`)
	reTop250Quote = regexp.MustCompile(`(?s)class="quote">.*?<span[^>]*>(.*?)</span>`)
	reTitleSpan   = regexp.MustCompile(`(?s)<span class="title">(.*?)</span>`)
	reOtherSpan   = regexp.MustCompile(`(?s)<span class="other">(.*?)</span>`)
	reEm          = regexp.MustCompile(`<em[^>]*>(\d+)</em>`)
	reImg         = regexp.MustCompile(`<img[^>]+src="([^"]+)"`)
	reBdP         = regexp.MustCompile(`(?s)<p[^>]*>(.*?)</p>`)
	reCover       = regexp.MustCompile(`(?s)<img[^>]+src="([^"]+)"`)
	reChartNums   = regexp.MustCompile(`(?s)class="rating_nums">(.*?)<`)
	reChartLink   = regexp.MustCompile(`(?s)class="pl2">\s*<a[^>]*>(.*?)</a>`)
	reDataAttr    = regexp.MustCompile(`(?s)<li\b[^>]*\bclass="list-item"[^>]*>`)
)

// Top250 returns the Douban Top 250 films in rank order. It walks the paged
// ?start= listing until limit rows are collected (default and maximum 250).
func (c *Client) Top250(ctx context.Context, limit int) ([]Movie, error) {
	if limit <= 0 || limit > 250 {
		limit = 250
	}
	var out []Movie
	for start := 0; start < limit; start += 25 {
		u := fmt.Sprintf("%s/top250?start=%d", c.host("movie"), start)
		body, err := c.get(ctx, u)
		if err != nil {
			return nil, err
		}
		page := parseTop250(string(body))
		if len(page) == 0 {
			break
		}
		out = append(out, page...)
		if len(out) >= limit {
			break
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func parseTop250(h string) []Movie {
	var out []Movie
	for _, chunk := range strings.Split(h, `<div class="item">`)[1:] {
		m := Movie{
			URL:   subjectURL(firstURLSubject(chunk)),
			ID:    subjectID(chunk),
			Cover: firstMatch(reImg, chunk),
		}
		if em := reEm.FindStringSubmatch(chunk); em != nil {
			m.Rank, _ = strconv.Atoi(em[1])
		}
		titles := reTitleSpan.FindAllStringSubmatch(chunk, -1)
		if len(titles) > 0 {
			m.Title = clean(titles[0][1])
		}
		var akas []string
		for _, t := range titles[1:] {
			akas = append(akas, strings.Trim(clean(t[1]), "/ "))
		}
		if o := reOtherSpan.FindStringSubmatch(chunk); o != nil {
			akas = append(akas, strings.Trim(clean(o[1]), "/ "))
		}
		m.AKA = strings.Join(filterEmpty(akas), " / ")

		if p := reBdP.FindStringSubmatch(chunk); p != nil {
			fillCredits(&m, p[1])
		}
		avg, _ := strconv.ParseFloat(firstGroup(reTop250Avg, chunk), 64)
		votes := 0
		if v := reTop250Votes.FindStringSubmatch(chunk); v != nil {
			votes, _ = strconv.Atoi(v[1])
		}
		m.Rating = fmtRating(avg, votes)
		m.Quote = firstGroup(reTop250Quote, chunk)
		if m.ID != "" {
			out = append(out, m)
		}
	}
	return out
}

// fillCredits parses a Top250 "导演: X 主演: Y<br> YEAR / REGION / GENRES" block.
func fillCredits(m *Movie, p string) {
	parts := strings.SplitN(p, "<br", 2)
	credits := clean(parts[0])
	if i := strings.Index(credits, "主演:"); i >= 0 {
		m.Directors = strings.TrimSpace(strings.TrimPrefix(clean(credits[:i]), "导演:"))
		m.Cast = strings.TrimSpace(strings.TrimPrefix(credits[i:], "主演:"))
	} else {
		m.Directors = strings.TrimSpace(strings.TrimPrefix(credits, "导演:"))
	}
	if len(parts) == 2 {
		rest := strings.TrimLeft(clean(parts[1]), ">/ ")
		meta := strings.Split(rest, "/")
		for i := range meta {
			meta[i] = strings.TrimSpace(meta[i])
		}
		if len(meta) > 0 {
			m.Year = meta[0]
		}
		if len(meta) > 1 {
			m.Region = meta[1]
		}
		if len(meta) > 2 {
			m.Genres = strings.Join(meta[2:], " / ")
		}
	}
}

// Chart returns the current Douban film ranking board (/chart).
func (c *Client) Chart(ctx context.Context) ([]Movie, error) {
	u := c.host("movie") + "/chart"
	body, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}
	h := string(body)
	var out []Movie
	for _, chunk := range strings.Split(h, `<tr class="item">`)[1:] {
		m := Movie{
			URL:   subjectURL(firstURLSubject(chunk)),
			ID:    subjectID(chunk),
			Cover: firstMatch(reCover, chunk),
		}
		if a := reChartLink.FindStringSubmatch(chunk); a != nil {
			title := clean(a[1])
			if i := strings.Index(title, "/"); i >= 0 {
				m.Title = strings.TrimSpace(title[:i])
				m.AKA = strings.TrimSpace(title[i+1:])
			} else {
				m.Title = title
			}
		}
		if p := reBdP.FindStringSubmatch(chunk); p != nil {
			m.Cast = clean(p[1])
		}
		avg, _ := strconv.ParseFloat(firstGroup(reChartNums, chunk), 64)
		votes := 0
		if v := reTop250Votes.FindStringSubmatch(chunk); v != nil {
			votes, _ = strconv.Atoi(v[1])
		}
		m.Rating = fmtRating(avg, votes)
		if m.ID != "" {
			out = append(out, m)
		}
	}
	return out, nil
}

// NowPlaying returns the films in cinemas now for a city (default beijing). When
// coming is true it returns the upcoming list from the same page instead.
func (c *Client) NowPlaying(ctx context.Context, city string, coming bool, limit int) ([]Movie, error) {
	if city == "" {
		city = "beijing"
	}
	u := fmt.Sprintf("%s/cinema/nowplaying/%s/", c.host("movie"), city)
	body, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}
	want := "nowplaying"
	if coming {
		want = "upcoming"
	}

	var out []Movie
	for _, loc := range reDataAttr.FindAllStringIndex(string(body), -1) {
		tag := string(body)[loc[0]:loc[1]]
		if dataAttr(tag, "category") != want {
			continue
		}
		id := dataAttr(tag, "subject")
		score, _ := strconv.ParseFloat(dataAttr(tag, "score"), 64)
		votes, _ := strconv.Atoi(dataAttr(tag, "votecount"))
		out = append(out, Movie{
			ID:        id,
			Title:     dataAttr(tag, "title"),
			Year:      dataAttr(tag, "release"),
			Region:    dataAttr(tag, "region"),
			Directors: dataAttr(tag, "director"),
			Cast:      dataAttr(tag, "actors"),
			Duration:  dataAttr(tag, "duration"),
			Rating:    fmtRating(score, votes),
			URL:       fmt.Sprintf("%s/subject/%s/", c.host("movie"), id),
		})
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

// dataAttr reads a data-<name> attribute value from an opening tag.
func dataAttr(tag, name string) string {
	re := regexp.MustCompile(`data-` + regexp.QuoteMeta(name) + `="([^"]*)"`)
	if m := re.FindStringSubmatch(tag); m != nil {
		return clean(m[1])
	}
	return ""
}

// firstURLSubject returns the first /subject/{id}/ URL found in a chunk.
func firstURLSubject(chunk string) string {
	if m := regexp.MustCompile(`https?://[^"']*/subject/\d+/`).FindString(chunk); m != "" {
		return m
	}
	return ""
}

func filterEmpty(in []string) []string {
	out := in[:0]
	for _, s := range in {
		if strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}
