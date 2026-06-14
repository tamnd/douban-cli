// Package cli builds the douban command tree on top of the douban library.
package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
	"github.com/tamnd/douban-cli/douban"
)

// Build metadata, set via -ldflags at release time.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// exit codes.
const (
	exitError  = 1
	exitUsage  = 2
	exitNoData = 3
)

// ExitError carries a process exit code up to main.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return fmt.Sprintf("exit %d", e.Code)
}

func (e *ExitError) Unwrap() error { return e.Err }

func codeError(code int, err error) error { return &ExitError{Code: code, Err: err} }

// App holds shared state threaded through every command.
type App struct {
	client *douban.Client
	cfg    douban.Config

	output   string
	fields   []string
	noHeader bool
	template string
	limit    int
	quiet    bool

	// mirror flags
	dataDir     string
	frodoKey    string
	frodoSecret string
}

// Root builds the root command and its subtree.
func Root() *cobra.Command {
	app := &App{cfg: douban.DefaultConfig()}

	root := &cobra.Command{
		Use:   "douban",
		Short: "Crawl Douban (豆瓣) books and movies into structured records",
		Long: `douban reads Douban (豆瓣) through its public pages: search, suggest,
book detail, the film charts, now-playing, and curated lists (豆列).
No API key is required. It returns records as table, JSON, JSONL,
CSV, TSV, or URLs.

douban is an independent tool and is not affiliated with Douban.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			return app.setup()
		},
	}

	pf := root.PersistentFlags()
	pf.StringVarP(&app.output, "output", "o", "auto", "output: table|json|jsonl|csv|tsv|url (auto=table on TTY, jsonl piped)")
	pf.StringSliceVar(&app.fields, "fields", nil, "comma-separated columns to include")
	pf.BoolVar(&app.noHeader, "no-header", false, "omit the header row in table/csv/tsv")
	pf.StringVar(&app.template, "template", "", "Go text/template applied per record")
	pf.IntVarP(&app.limit, "limit", "n", 0, "limit number of records (0 = command default)")
	pf.BoolVarP(&app.quiet, "quiet", "q", false, "suppress progress on stderr")

	pf.DurationVar(&app.cfg.Rate, "delay", app.cfg.Rate, "minimum spacing between requests")
	pf.DurationVar(&app.cfg.Timeout, "timeout", app.cfg.Timeout, "per-request timeout")
	pf.IntVar(&app.cfg.Retries, "retries", app.cfg.Retries, "retry attempts on 429/5xx")
	pf.StringVar(&app.cfg.UserAgent, "user-agent", app.cfg.UserAgent, "User-Agent sent with each request")

	root.AddCommand(
		app.searchCmd(),
		app.suggestCmd(),
		app.bookCmd(),
		app.top250Cmd(),
		app.chartCmd(),
		app.nowplayingCmd(),
		app.doulistCmd(),
		app.seedCmd(),
		app.crawlCmd(),
		app.exportCmd(),
		app.infoCmd(),
		app.queueCmd(),
		app.resetFailedCmd(),
		newVersionCmd(),
	)
	return root
}

func (a *App) setup() error {
	if a.output == "" || a.output == "auto" {
		if isatty.IsTerminal(os.Stdout.Fd()) {
			a.output = string(FormatTable)
		} else {
			a.output = string(FormatJSONL)
		}
	}
	if !Format(a.output).Valid() {
		return codeError(exitUsage, fmt.Errorf("unknown output format %q", a.output))
	}
	a.client = douban.NewClient(a.cfg)
	return nil
}

func (a *App) render(records any) error {
	r := NewRenderer(os.Stdout, Format(a.output), a.fields, a.noHeader, a.template)
	return r.Render(records)
}

func (a *App) renderOrEmpty(records any, n int) error {
	if err := a.render(records); err != nil {
		return err
	}
	if n == 0 {
		return codeError(exitNoData, nil)
	}
	return nil
}

func (a *App) progressf(format string, args ...any) {
	if a.quiet {
		return
	}
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
}

func (a *App) effectiveLimit(def int) int {
	if a.limit > 0 {
		return a.limit
	}
	return def
}

// fail maps a library error to a process exit: a not-found becomes exit 3
// (empty), anything else exit 1.
func (a *App) fail(err error) error {
	if errors.Is(err, douban.ErrNotFound) {
		return codeError(exitNoData, nil)
	}
	return codeError(exitError, err)
}
