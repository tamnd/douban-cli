// Package cli builds the douban command tree on top of the douban library and
// the any-cli/kit framework. The lookup commands are declared once as kit
// operations in the douban package, so the CLI, the HTTP API (douban serve), and
// the MCP server (douban mcp) all derive from one registry. The mirror
// subsystem is wired here as escape-hatch commands, since a stateful crawl does
// not fit the emit-records shape of an operation.
package cli

import (
	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/douban-cli/douban"
)

// Build metadata, set via -ldflags at release time.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// builder holds the domain-global flags while the app is assembled, then folds
// them onto the resolved config in finalize.
type builder struct {
	userAgent string
}

// NewApp assembles the kit App: the douban domain installs the client factory
// and the lookup operations, this package adds the mirror and version commands,
// and kit provides the CLI, API, and MCP surfaces.
func NewApp() *kit.App {
	b := &builder{}
	id := douban.Identity()
	id.Version = Version

	app := kit.New(id, kit.WithDefaults(douban.Defaults))
	app.GlobalFlags(b.globals)
	app.Finalize(b.finalize)

	douban.Domain{}.Register(app)
	registerMirror(app)
	app.AddCommand(versionCmd())
	return app
}

func (b *builder) globals(f *kit.FlagSet) {
	f.StringVar(&b.userAgent, "user-agent", douban.DefaultUserAgent, "User-Agent sent with each request")
}

func (b *builder) finalize(c *kit.Config) {
	if c.Extra == nil {
		c.Extra = map[string]string{}
	}
	if b.userAgent != "" {
		c.Extra["user-agent"] = b.userAgent
	}
}
