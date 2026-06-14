package douban

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	reBookTitle = regexp.MustCompile(`(?s)property="v:itemreviewed"[^>]*>(.*?)<`)
	reBookAvg   = regexp.MustCompile(`(?s)property="v:average"[^>]*>(.*?)<`)
	reBookVotes = regexp.MustCompile(`(?s)property="v:votes"[^>]*>(.*?)<`)
	reBookInfo  = regexp.MustCompile(`(?s)<div id="info"[^>]*>(.*?)</div>`)
	reBookField = regexp.MustCompile(`(?s)<span class="pl">(.*?)</span>(.*?)(?:<br|<span class="pl">|$)`)
	reBookCover = regexp.MustCompile(`(?s)id="mainpic".*?<img[^>]+src="([^"]+)"`)
	reBookIntro = regexp.MustCompile(`(?s)<div class="intro">(.*?)</div>`)
)

// Book fetches the full record for one book subject. The argument may be a bare
// id or a pasted book.douban.com URL.
func (c *Client) Book(ctx context.Context, idOrURL string) (*Book, error) {
	id := subjectID(idOrURL)
	if id == "" {
		id = strings.TrimSpace(idOrURL)
	}
	if id == "" || strings.ContainsAny(id, "/?") {
		return nil, fmt.Errorf("douban: invalid book id %q", idOrURL)
	}

	u := fmt.Sprintf("%s/subject/%s/", c.host("book"), id)
	body, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}
	h := string(body)

	b := &Book{
		ID:     id,
		Title:  firstGroup(reBookTitle, h),
		URL:    u,
		Cover:  firstMatch(reBookCover, h),
		Rating: bookRating(h),
	}

	info := parseBookInfo(h)
	b.Author = info["作者"]
	b.Translator = info["译者"]
	b.Publisher = info["出版社"]
	b.Producer = info["出品方"]
	b.OriginalTitle = info["原作名"]
	b.PubDate = info["出版年"]
	b.Pages = info["页数"]
	b.Price = info["定价"]
	b.Binding = info["装帧"]
	b.Series = info["丛书"]
	b.ISBN = info["ISBN"]

	if m := reBookIntro.FindStringSubmatch(h); m != nil {
		b.Summary = truncRunes(clean(m[1]), 500)
	}
	return b, nil
}

// bookRating combines the microdata average and vote count into one string.
func bookRating(h string) string {
	avg, _ := strconv.ParseFloat(firstGroup(reBookAvg, h), 64)
	votes, _ := strconv.Atoi(firstGroup(reBookVotes, h))
	return fmtRating(avg, votes)
}

// parseBookInfo turns the <div id="info"> block into a label->value map. Labels
// are the Chinese field names with the trailing colon removed.
func parseBookInfo(h string) map[string]string {
	out := map[string]string{}
	m := reBookInfo.FindStringSubmatch(h)
	if m == nil {
		return out
	}
	for _, f := range reBookField.FindAllStringSubmatch(m[1], -1) {
		// The colon may sit inside the label span or just after it, so trim it
		// from both the key's tail and the value's head.
		key := strings.TrimRight(clean(f[1]), ": ")
		val := strings.TrimLeft(clean(f[2]), ": ")
		if key != "" && val != "" {
			out[key] = val
		}
	}
	return out
}

// firstMatch returns the first capture group of re against s (uncleaned), or "".
func firstMatch(re *regexp.Regexp, s string) string {
	if m := re.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return ""
}
