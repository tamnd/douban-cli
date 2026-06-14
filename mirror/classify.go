// Package mirror holds the shared routing logic for the Douban mirror crawler:
// turning a URL into the entity it represents and the source best suited to
// fetch it. The store, sitemap, and crawl packages build on this classification.
package mirror

import (
	"net/url"
	"regexp"
	"strings"
)

// Source values mirror store.Source* and name the fetch surface a URL is best
// reached through. Kept as plain strings to avoid importing the store here.
const (
	SourceFrodo  = "frodo"
	SourceHTML   = "html"
	SourceMobile = "mobile"
)

// Class is the result of routing a URL: what entity it is, its id, and which
// source should fetch it.
type Class struct {
	EntityType string
	EntityID   string
	Source     string
	Host       string
}

// rule maps a host + path prefix to an entity type and its preferred source.
type rule struct {
	host string // exact hostname, or "" to match any douban host
	re   *regexp.Regexp
	typ  string
	src  string
}

// rules are evaluated in order; the first match wins, so more specific paths
// (group/topic before group, location/drama before subject) come first. The
// patterns are deliberately unanchored at the end so per-entity subpages
// (a celebrity's /photos, /awards) classify under the same id.
var rules = []rule{
	{"", regexp.MustCompile(`^/group/topic/(\d+)`), "group-topic", SourceHTML},
	{"www.douban.com", regexp.MustCompile(`^/group/(\d+)`), "group", SourceHTML},
	{"", regexp.MustCompile(`^/review/(\d+)`), "review", SourceFrodo},
	{"www.douban.com", regexp.MustCompile(`^/event/(\d+)`), "event", SourceHTML},
	{"www.douban.com", regexp.MustCompile(`^/people/(\d+)`), "people", SourceHTML},
	{"www.douban.com", regexp.MustCompile(`^/note/(\d+)`), "note", SourceFrodo},
	{"www.douban.com", regexp.MustCompile(`^/personage/(\d+)`), "personage", SourceFrodo},
	{"www.douban.com", regexp.MustCompile(`^/doulist/(\d+)`), "doulist", SourceHTML},
	{"www.douban.com", regexp.MustCompile(`^/location/drama/(\d+)`), "drama", SourceHTML},
	{"www.douban.com", regexp.MustCompile(`^/photos/album/(\d+)`), "photo-album", SourceHTML},
	{"", regexp.MustCompile(`^/game/(\d+)`), "game", SourceFrodo},
	{"movie.douban.com", regexp.MustCompile(`^/celebrity/(\d+)`), "celebrity", SourceFrodo},
	{"music.douban.com", regexp.MustCompile(`^/musician/(\d+)`), "musician", SourceHTML},
	// Subjects share one global id space; the host names the media type.
	{"book.douban.com", regexp.MustCompile(`^/subject/(\d+)`), "book", SourceHTML},
	{"movie.douban.com", regexp.MustCompile(`^/subject/(\d+)`), "movie", SourceFrodo},
	{"music.douban.com", regexp.MustCompile(`^/subject/(\d+)`), "music", SourceHTML},
	{"9.douban.com", regexp.MustCompile(`^/subject/(\d+)`), "thing", SourceHTML},
	{"www.douban.com", regexp.MustCompile(`^/subject/(\d+)`), "subject", SourceHTML},
}

// Classify routes a URL to its entity. It returns ok=false for URLs outside the
// recognized entity space (index pages, tag browse, location landing pages).
func Classify(rawURL string) (Class, bool) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Host == "" {
		return Class{}, false
	}
	host := u.Hostname()
	if !strings.HasSuffix(host, "douban.com") {
		return Class{}, false
	}
	path := u.Path
	for _, r := range rules {
		if r.host != "" && r.host != host {
			continue
		}
		if m := r.re.FindStringSubmatch(path); m != nil {
			return Class{EntityType: r.typ, EntityID: m[1], Source: r.src, Host: host}, true
		}
	}
	return Class{}, false
}
