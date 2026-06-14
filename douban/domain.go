package douban

import (
	"context"
	"errors"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

// Domain is the douban driver for the any-cli/kit framework. It declares the
// lookup operations once, and kit derives the CLI subcommands, the HTTP API
// routes, and the MCP tools from that single registry. The same Domain powers
// the standalone douban binary and a multi-domain host that mounts it.
type Domain struct{}

func init() { kit.Register(Domain{}) }

// Info describes the domain: its URI scheme, the hosts it owns, and the identity
// the standalone binary reuses for help and version.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme:  "douban",
		Aliases: []string{"db"},
		Hosts: []string{
			"douban.com", "www.douban.com",
			"book.douban.com", "movie.douban.com", "music.douban.com",
		},
		Identity: Identity(),
	}
}

// Identity is the fixed description of the douban CLI, shared by the domain and
// the standalone composition root so help and version read the same everywhere.
func Identity() kit.Identity {
	return kit.Identity{
		Binary: "douban",
		Short:  "Crawl Douban (豆瓣) books and movies into structured records",
		Long: `douban reads Douban (豆瓣) through its public pages: search, suggest,
book detail, the film charts, now-playing, and curated lists (豆列).
No API key is required. It returns records as a table, JSON, JSONL,
CSV, TSV, or URLs, and serves the same operations over HTTP and MCP.

douban is an independent tool and is not affiliated with Douban.`,
		Site: "https://www.douban.com",
		Repo: "https://github.com/tamnd/douban-cli",
	}
}

// Register installs the client factory and every lookup operation on the app.
// The mirror subsystem is wired as escape-hatch commands by the cli package,
// since it is CLI-only (stateful crawl, not the emit-records shape).
func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)
	app.CommandGroup("read", "Look up Douban records")

	kit.Handle(app, kit.OpMeta{
		Name: "search", Group: "read",
		Summary: "Search Douban for books, movies, or music",
		URIType: "result",
	}, search)

	kit.Handle(app, kit.OpMeta{
		Name: "suggest", Group: "read",
		Summary: "Fast typeahead lookup (the reliable way to find a film's id)",
		URIType: "suggestion",
	}, suggest)

	kit.Handle(app, kit.OpMeta{
		Name: "book", Group: "read", Single: true,
		Summary: "Show full detail for one book",
		URIType: "book", Resolver: true,
		Args: []kit.Arg{{Name: "ref", Help: "book id or book.douban.com URL"}},
	}, getBook)

	kit.Handle(app, kit.OpMeta{
		Name: "top250", Group: "read",
		Summary: "The Douban Top 250 films, in rank order",
		URIType: "movie",
	}, top250)

	kit.Handle(app, kit.OpMeta{
		Name: "chart", Group: "read",
		Summary: "The current Douban film ranking board",
		URIType: "movie",
	}, chart)

	kit.Handle(app, kit.OpMeta{
		Name: "nowplaying", Group: "read",
		Summary: "Films in cinemas now (or upcoming) for a city",
		URIType: "movie",
	}, nowplaying)

	kit.Handle(app, kit.OpMeta{
		Name: "doulist", Group: "read",
		Summary: "List the subjects in a curated list (豆列)",
		URIType: "doulist_item",
		Args:    []kit.Arg{{Name: "ref", Help: "doulist id or www.douban.com/doulist URL"}},
	}, doulist)
}

// newClient builds the douban HTTP client from the resolved kit config. It is
// the factory kit calls once per run; the value is injected into handler fields
// tagged kit:"inject".
func newClient(_ context.Context, cfg kit.Config) (any, error) {
	return ClientFromConfig(cfg), nil
}

// ClientFromConfig maps the framework config onto a douban.Config and returns a
// client. The mirror commands reuse it so the CLI surface and the crawler share
// one notion of rate, timeout, retries, and User-Agent.
func ClientFromConfig(cfg kit.Config) *Client {
	dc := DefaultConfig()
	if cfg.Rate > 0 {
		dc.Rate = cfg.Rate
	}
	if cfg.Retries >= 0 {
		dc.Retries = cfg.Retries
	}
	if cfg.Timeout > 0 {
		dc.Timeout = cfg.Timeout
	}
	if ua := cfg.Extra["user-agent"]; ua != "" {
		dc.UserAgent = ua
	}
	return NewClient(dc)
}

// Defaults seeds the framework baseline with douban's own values, so an unset
// --rate or --timeout uses the douban default rather than the generic kit one.
// It is passed to kit.New via kit.WithDefaults.
func Defaults(c *kit.Config) {
	def := DefaultConfig()
	c.Rate = def.Rate
	c.Retries = def.Retries
	c.Timeout = def.Timeout
	c.UserAgent = def.UserAgent
}

// mapErr translates a library error into a kit error so the exit code matches
// the rest of the fleet: a missing subject reads as "no results" (exit 3), and
// everything else falls through to the generic failure (exit 1).
func mapErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrNotFound) {
		return errs.NoResults("not found")
	}
	return err
}

// limitOr returns the operator's --limit when set, else the command's own
// default fetch count. kit also caps emission at --limit, so this only bounds
// how much the client fetches up front.
func limitOr(limit, def int) int {
	if limit > 0 {
		return limit
	}
	return def
}
