// Package cli builds the douban command tree on top of the douban library.
package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/charmbracelet/x/term"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
	"github.com/tamnd/douban-cli/douban"
	"github.com/tamnd/douban-cli/pkg/render"
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
	color    string
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
	pf.StringVarP(&app.output, "output", "o", "auto", "output: table|markdown|json|jsonl|csv|tsv|url|raw (auto=table on TTY, jsonl piped)")
	pf.StringVar(&app.color, "color", "auto", "color: auto|always|never (auto colors a terminal, honors NO_COLOR)")
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
	switch a.color {
	case "", "auto", "always", "never":
	default:
		return codeError(exitUsage, fmt.Errorf("unknown color mode %q (want auto|always|never)", a.color))
	}
	a.client = douban.NewClient(a.cfg)
	return nil
}

func (a *App) render(records any) error {
	tty := isatty.IsTerminal(os.Stdout.Fd())
	width := 0
	if Format(a.output) == FormatTable || Format(a.output) == FormatMarkdown {
		// Only the grid formats shrink to fit; machine formats stay lossless.
		width = termWidth()
	}
	r, err := render.New(render.Options{
		Format:   Format(a.output),
		Fields:   a.fields,
		NoHeader: a.noHeader,
		Template: a.template,
		Color:    colorEnabled(a.color, tty),
		IsTTY:    tty,
		Width:    width,
		Writer:   os.Stdout,
	})
	if err != nil {
		return codeError(exitUsage, err)
	}
	return r.Render(records)
}

// colorEnabled resolves --color against the terminal and the NO_COLOR
// convention: auto colors only an interactive terminal, always forces it on,
// never disables it. A pipe is not a TTY, so auto keeps `douban ... | jq` plain.
func colorEnabled(mode string, tty bool) bool {
	switch mode {
	case "always":
		return true
	case "never":
		return false
	default:
		return tty && os.Getenv("NO_COLOR") == ""
	}
}

// termWidth reports the terminal width in columns, or 0 when stdout is not a
// terminal. COLUMNS wins when set so the width is scriptable and testable.
func termWidth() int {
	if v := os.Getenv("COLUMNS"); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n > 0 {
			return n
		}
	}
	if w, _, err := term.GetSize(os.Stdout.Fd()); err == nil && w > 0 {
		return w
	}
	return 0
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
