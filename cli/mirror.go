package cli

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"

	"github.com/tamnd/douban-cli/douban"
	"github.com/tamnd/douban-cli/mirror"
	"github.com/tamnd/douban-cli/mirror/crawl"
	"github.com/tamnd/douban-cli/mirror/sitemap"
	"github.com/tamnd/douban-cli/mirror/store"
)

// registerMirror adds the local-mirror commands. They are escape hatches, not
// kit operations: a stateful, resumable crawl into a SQLite store is not the
// emit-records shape, so it lives on the CLI only and is absent from the API and
// MCP surfaces.
func registerMirror(app *kit.App) {
	app.AddCommand(seedCmd())
	app.AddCommand(crawlCmd())
	app.AddCommand(exportCmd())
	app.AddCommand(infoCmd())
	app.AddCommand(queueCmd())
	app.AddCommand(resetFailedCmd())
}

// mirrorEnv carries the per-run handles a mirror command needs: the resolved
// state (config, renderer), the shared douban client, and the data dir and
// Frodo credential overrides parsed from the command's own flags.
type mirrorEnv struct {
	st          *kit.State
	client      *douban.Client
	dataDir     string
	frodoKey    string
	frodoSecret string
}

func mirrorFrom(ctx context.Context, f *mirrorFlags) *mirrorEnv {
	return &mirrorEnv{
		st:          kit.FromContext(ctx),
		client:      kit.MustClient[*douban.Client](ctx),
		dataDir:     f.data,
		frodoKey:    f.frodoKey,
		frodoSecret: f.frodoSecret,
	}
}

func (e *mirrorEnv) dataPath() string { return dataPathOf(e.dataDir) }

func (e *mirrorEnv) openStore() (*store.Store, error) {
	st, err := store.Open(e.dataPath())
	if err != nil {
		return nil, errs.Wrap(errs.KindGeneric, err, "open store")
	}
	return st, nil
}

func (e *mirrorEnv) frodoClient() *douban.FrodoClient {
	return frodoClientFrom(e.frodoKey, e.frodoSecret)
}

func (e *mirrorEnv) limit() int {
	if e.st != nil {
		return e.st.Globals.Limit
	}
	return 0
}

func (e *mirrorEnv) progressf(format string, args ...any) {
	if e.st != nil && e.st.Config.Quiet {
		return
	}
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
}

// crawlDelay honors --rate when the operator set it, else returns 0 so the
// engine keeps its polite web Crawl-delay default (5s).
func (e *mirrorEnv) crawlDelay() time.Duration {
	if e.st != nil && e.st.Config.Rate != douban.DefaultConfig().Rate {
		return e.st.Config.Rate
	}
	return 0
}

// mirrorFlags holds the data dir and Frodo credential overrides every mirror
// command shares.
type mirrorFlags struct {
	data        string
	frodoKey    string
	frodoSecret string
}

func (m *mirrorFlags) bind(f *kit.FlagSet) {
	f.StringVar(&m.data, "data", "", "mirror directory (default $DOUBAN_DATA or $HOME/data/douban)")
	f.StringVar(&m.frodoKey, "frodo-key", "", "override the Frodo API key (or $DOUBAN_FRODO_KEY)")
	f.StringVar(&m.frodoSecret, "frodo-secret", "", "override the Frodo API secret (or $DOUBAN_FRODO_SECRET)")
}

// --- seed ---

func seedCmd() kit.Command {
	return kit.Command{
		Use:   "seed",
		Short: "Add URLs to the crawl frontier",
		Long: `Populate the crawl frontier from Douban's sitemap, an id range, a single
URL, or a file of URLs. Seeding is idempotent: a URL already in the frontier
keeps its status and history.`,
		Sub: []kit.Command{seedSitemapCmd(), seedIDsCmd(), seedURLCmd(), seedListCmd()},
	}
}

func seedSitemapCmd() kit.Command {
	f := &mirrorFlags{}
	var bands []string
	var since bool
	return kit.Command{
		Use:   "sitemap",
		Short: "Seed from the site sitemap, optionally banded by entity type",
		Long: `Enumerate URLs from Douban's single sitemap backbone and add the entity
URLs to the frontier. Sitemaps are partitioned into contiguous bands by entity
type, so --band fetches only the types you want instead of all children.

  douban seed sitemap --band subject --limit 5000
  douban seed sitemap --band celebrity --band musician
  douban seed sitemap --since           # the daily updated feed

Known bands: ` + strings.Join(bandNames(), ", "),
		Args: kit.NoArgs,
		Flags: func(fs *kit.FlagSet) {
			fs.StringSliceVar(&bands, "band", nil, "limit to these entity bands (repeatable)")
			fs.BoolVar(&since, "since", false, "use the daily updated index instead of the full one")
			f.bind(fs)
		},
		Run: func(ctx context.Context, _ []string) error {
			env := mirrorFrom(ctx, f)
			st, err := env.openStore()
			if err != nil {
				return err
			}
			defer func() { _ = st.Close() }()

			indexURL := sitemap.IndexURL
			if since {
				indexURL = sitemap.UpdatedIndexURL
			}
			env.progressf("fetching sitemap index %s...", indexURL)
			children, err := sitemap.Index(ctx, env.client, indexURL)
			if err != nil {
				return errs.Wrap(errs.KindGeneric, err, "sitemap index")
			}
			if !since && len(bands) > 0 {
				children = selectBands(children, bands)
			}
			env.progressf("walking %d child sitemaps...", len(children))

			limit := env.limit()
			added := 0
			for i, child := range children {
				if limit > 0 && added >= limit {
					break
				}
				locs, err := sitemap.Children(ctx, env.client, child)
				if err != nil {
					env.progressf("child %d (%s): %v", i, child, err)
					continue
				}
				for _, loc := range locs {
					if limit > 0 && added >= limit {
						break
					}
					if enqueueURL(st, loc) {
						added++
					}
				}
				env.progressf("child %d/%d: %d urls seeded", i+1, len(children), added)
			}
			_, _ = fmt.Fprintf(os.Stdout, "seeded %d urls\n", added)
			if added == 0 {
				return errs.NoResults("no urls seeded")
			}
			return nil
		},
	}
}

func seedIDsCmd() kit.Command {
	f := &mirrorFlags{}
	var typ string
	var from, to int
	return kit.Command{
		Use:   "ids --type T --from N --to M",
		Short: "Seed a contiguous id range for one entity type",
		Long: `Seed every id in [from, to] for one entity type. Useful for the dense,
low-cardinality id spaces (book subjects, movie celebrities) where id-stride
enumeration catches ids newer than the last sitemap snapshot.

  douban seed ids --type book --from 1084336 --to 1084340
  douban seed ids --type celebrity --from 1601851 --to 1601861`,
		Args: kit.NoArgs,
		Flags: func(fs *kit.FlagSet) {
			fs.StringVar(&typ, "type", "", "entity type (book, movie, music, celebrity, personage, ...)")
			fs.IntVar(&from, "from", 0, "first id (inclusive)")
			fs.IntVar(&to, "to", 0, "last id (inclusive)")
			f.bind(fs)
		},
		Run: func(ctx context.Context, _ []string) error {
			env := mirrorFrom(ctx, f)
			st, err := env.openStore()
			if err != nil {
				return err
			}
			defer func() { _ = st.Close() }()

			added, err := seedIDs(st, typ, from, to)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(os.Stdout, "seeded %d urls\n", added)
			if added == 0 {
				return errs.NoResults("no urls seeded")
			}
			return nil
		},
	}
}

func seedURLCmd() kit.Command {
	f := &mirrorFlags{}
	return kit.Command{
		Use:   "url <url>...",
		Short: "Seed one or more explicit URLs",
		Args:  kit.MinimumNArgs(1),
		Flags: f.bind,
		Run: func(ctx context.Context, args []string) error {
			env := mirrorFrom(ctx, f)
			st, err := env.openStore()
			if err != nil {
				return err
			}
			defer func() { _ = st.Close() }()
			added := 0
			for _, u := range args {
				if enqueueURL(st, u) {
					added++
				} else {
					env.progressf("skip (unclassified): %s", u)
				}
			}
			_, _ = fmt.Fprintf(os.Stdout, "seeded %d urls\n", added)
			if added == 0 {
				return errs.NoResults("no urls seeded")
			}
			return nil
		},
	}
}

func seedListCmd() kit.Command {
	f := &mirrorFlags{}
	return kit.Command{
		Use:   "list <file>",
		Short: "Seed URLs from a file, one per line",
		Args:  kit.ExactArgs(1),
		Flags: f.bind,
		Run: func(ctx context.Context, args []string) error {
			env := mirrorFrom(ctx, f)
			st, err := env.openStore()
			if err != nil {
				return err
			}
			defer func() { _ = st.Close() }()
			added, err := seedFromList(st, args[0])
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(os.Stdout, "seeded %d urls\n", added)
			if added == 0 {
				return errs.NoResults("no urls seeded")
			}
			return nil
		},
	}
}

// --- crawl ---

func crawlCmd() kit.Command {
	f := &mirrorFlags{}
	var types []string
	var source string
	var concurrency int
	var retryFailed bool
	return kit.Command{
		Use:   "crawl",
		Short: "Crawl pending frontier URLs into the mirror",
		Long: `Drain the frontier: fetch each pending URL through the source that serves
it, archive the raw bytes, record the normalized entity, and enqueue the links
it discovers. The crawl is resumable, so an interrupted run picks up where it
left off, and --limit bounds a single pass.

  douban crawl --limit 20
  douban crawl --type movie --source frodo --concurrency 4
  douban crawl --retry-failed`,
		Args:  kit.NoArgs,
		Write: true,
		Flags: func(fs *kit.FlagSet) {
			fs.StringSliceVar(&types, "type", nil, "restrict to these entity types")
			fs.StringVar(&source, "source", "", "restrict to a source (frodo|html)")
			fs.IntVar(&concurrency, "concurrency", 4, "number of concurrent workers")
			fs.BoolVar(&retryFailed, "retry-failed", false, "requeue failed urls before crawling")
			f.bind(fs)
		},
		Run: func(ctx context.Context, _ []string) error {
			env := mirrorFrom(ctx, f)
			st, err := env.openStore()
			if err != nil {
				return err
			}
			defer func() { _ = st.Close() }()

			if retryFailed {
				n, err := st.ResetFailed(firstType(types))
				if err != nil {
					return errs.Wrap(errs.KindGeneric, err, "reset failed")
				}
				env.progressf("requeued %d failed urls", n)
			}

			eng := crawl.New(st, env.client, env.frodoClient(), crawl.Config{
				Concurrency: concurrency,
				Limit:       env.limit(),
				HTMLDelay:   env.crawlDelay(),
				Types:       types,
				Source:      source,
				Logf:        env.progressf,
			})
			env.progressf("crawling...")
			stats, err := eng.Run(ctx)
			if err != nil {
				return errs.Wrap(errs.KindGeneric, err, "crawl")
			}
			_, _ = fmt.Fprintf(os.Stdout,
				"processed %d: %d done, %d blocked, %d skipped, %d failed, %d discovered\n",
				stats.Processed, stats.Done, stats.Blocked, stats.Skipped, stats.Failed, stats.Discovered)
			if stats.Processed == 0 {
				return errs.NoResults("nothing to crawl")
			}
			return nil
		},
	}
}

// --- export ---

func exportCmd() kit.Command {
	f := &mirrorFlags{}
	var typ, out string
	return kit.Command{
		Use:   "export",
		Short: "Export normalized records as JSONL",
		Long: `Stream normalized records as JSONL. With no --out the records go to stdout;
with --out DIR they are written to DIR/<type>.jsonl.

  douban export --type movie | jq .
  douban export --out ./export`,
		Args: kit.NoArgs,
		Flags: func(fs *kit.FlagSet) {
			fs.StringVar(&typ, "type", "", "restrict to one entity type")
			fs.StringVar(&out, "out", "", "write to DIR/<type>.jsonl instead of stdout")
			f.bind(fs)
		},
		Run: func(ctx context.Context, _ []string) error {
			env := mirrorFrom(ctx, f)
			st, err := env.openStore()
			if err != nil {
				return err
			}
			defer func() { _ = st.Close() }()

			if out == "" {
				n, err := st.ExportJSONL(ctx, typ, os.Stdout)
				if err != nil {
					return errs.Wrap(errs.KindGeneric, err, "export")
				}
				if n == 0 {
					return errs.NoResults("no records")
				}
				return nil
			}

			if err := os.MkdirAll(out, 0o755); err != nil {
				return errs.Wrap(errs.KindGeneric, err, "create export dir")
			}
			name := typ
			if name == "" {
				name = "all"
			}
			path := filepath.Join(out, name+".jsonl")
			file, err := os.Create(path)
			if err != nil {
				return errs.Wrap(errs.KindGeneric, err, "create export file")
			}
			defer func() { _ = file.Close() }()
			n, err := st.ExportJSONL(ctx, typ, file)
			if err != nil {
				return errs.Wrap(errs.KindGeneric, err, "export")
			}
			_, _ = fmt.Fprintf(os.Stdout, "wrote %d records to %s\n", n, path)
			if n == 0 {
				return errs.NoResults("no records")
			}
			return nil
		},
	}
}

// --- info ---

func infoCmd() kit.Command {
	f := &mirrorFlags{}
	return kit.Command{
		Use:   "info",
		Short: "Show mirror location, counts, and disk usage",
		Args:  kit.NoArgs,
		Flags: f.bind,
		Run: func(ctx context.Context, _ []string) error {
			env := mirrorFrom(ctx, f)
			st, err := env.openStore()
			if err != nil {
				return err
			}
			defer func() { _ = st.Close() }()

			dir := env.dataPath()
			_, _ = fmt.Fprintf(os.Stdout, "data dir: %s\n", dir)
			_, _ = fmt.Fprintf(os.Stdout, "database: %s\n", st.DBPath())
			_, _ = fmt.Fprintf(os.Stdout, "disk:     %s\n", humanBytes(dirSize(dir)))

			recs, err := st.RecordCounts()
			if err != nil {
				return errs.Wrap(errs.KindGeneric, err, "record counts")
			}
			_, _ = fmt.Fprintf(os.Stdout, "\nrecords by type:\n")
			for _, t := range sortedKeys(recs) {
				_, _ = fmt.Fprintf(os.Stdout, "  %-14s %d\n", t, recs[t])
			}

			fc, err := st.FrontierCounts()
			if err != nil {
				return errs.Wrap(errs.KindGeneric, err, "frontier counts")
			}
			_, _ = fmt.Fprintf(os.Stdout, "\nfrontier by type/status:\n")
			for _, t := range sortedKeys(fc) {
				parts := make([]string, 0, len(fc[t]))
				for _, s := range sortedKeys(fc[t]) {
					parts = append(parts, fmt.Sprintf("%s=%d", s, fc[t][s]))
				}
				_, _ = fmt.Fprintf(os.Stdout, "  %-14s %s\n", t, strings.Join(parts, " "))
			}
			return nil
		},
	}
}

// --- queue ---

// QueueRow is the rendered shape of a frontier row.
type QueueRow struct {
	URL        string `json:"url"`
	EntityType string `json:"entity_type"`
	EntityID   string `json:"entity_id"`
	Source     string `json:"source"`
	Status     string `json:"status"`
	Attempts   int    `json:"attempts"`
	HTTPStatus int    `json:"http_status,omitempty"`
	Error      string `json:"error,omitempty" table:",truncate"`
}

func queueCmd() kit.Command {
	f := &mirrorFlags{}
	var status, typ string
	return kit.Command{
		Use:   "queue",
		Short: "Inspect frontier rows by status and type",
		Long: `List frontier rows, optionally filtered by status and entity type.

  douban queue --status failed
  douban queue --status blocked --type note -n 50`,
		Args: kit.NoArgs,
		Flags: func(fs *kit.FlagSet) {
			fs.StringVar(&status, "status", "", "filter by status (pending|done|failed|blocked|skipped)")
			fs.StringVar(&typ, "type", "", "filter by entity type")
			f.bind(fs)
		},
		Run: func(ctx context.Context, _ []string) error {
			env := mirrorFrom(ctx, f)
			st, err := env.openStore()
			if err != nil {
				return err
			}
			defer func() { _ = st.Close() }()

			limit := env.limit()
			if limit == 0 {
				limit = 100
			}
			rows, err := st.QueueRows(status, typ, limit)
			if err != nil {
				return errs.Wrap(errs.KindGeneric, err, "queue rows")
			}
			r, err := env.st.Renderer(os.Stdout)
			if err != nil {
				return errs.Usage("%v", err)
			}
			for _, row := range rows {
				if err := r.Emit(QueueRow{
					URL: row.URL, EntityType: row.EntityType, EntityID: row.EntityID,
					Source: row.Source, Status: row.Status, Attempts: row.Attempts,
					HTTPStatus: row.HTTPStatus, Error: row.Error,
				}); err != nil {
					return err
				}
			}
			if err := r.Flush(); err != nil {
				return err
			}
			if len(rows) == 0 {
				return errs.NoResults("no rows")
			}
			return nil
		},
	}
}

// --- reset-failed ---

func resetFailedCmd() kit.Command {
	f := &mirrorFlags{}
	var typ string
	return kit.Command{
		Use:   "reset-failed",
		Short: "Requeue failed frontier rows for another crawl",
		Args:  kit.NoArgs,
		Write: true,
		Flags: func(fs *kit.FlagSet) {
			fs.StringVar(&typ, "type", "", "restrict to one entity type")
			f.bind(fs)
		},
		Run: func(ctx context.Context, _ []string) error {
			env := mirrorFrom(ctx, f)
			st, err := env.openStore()
			if err != nil {
				return err
			}
			defer func() { _ = st.Close() }()
			n, err := st.ResetFailed(typ)
			if err != nil {
				return errs.Wrap(errs.KindGeneric, err, "reset failed")
			}
			_, _ = fmt.Fprintf(os.Stdout, "requeued %d urls\n", n)
			if n == 0 {
				return errs.NoResults("no failed urls")
			}
			return nil
		},
	}
}

// --- pure helpers (no kit context, so they are unit-testable directly) ---

// dataPathOf resolves the mirror directory: the --data flag, then $DOUBAN_DATA,
// then $HOME/data/douban.
func dataPathOf(dataDir string) string {
	if dataDir != "" {
		return dataDir
	}
	if env := os.Getenv("DOUBAN_DATA"); env != "" {
		return env
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", "douban-data")
	}
	return filepath.Join(home, "data", "douban")
}

// frodoClientFrom builds a Frodo client from defaults plus flag/env overrides.
func frodoClientFrom(key, secret string) *douban.FrodoClient {
	cfg := douban.DefaultFrodoConfig()
	cfg.APIKey = firstNonEmpty(key, os.Getenv("DOUBAN_FRODO_KEY"), cfg.APIKey)
	cfg.Secret = firstNonEmpty(secret, os.Getenv("DOUBAN_FRODO_SECRET"), cfg.Secret)
	// Frodo rejects any request whose User-Agent is not the app's (it answers
	// invalid_apikey, code 1062), so the generic CLI UA must not leak onto these
	// requests. Keep the Frodo app UA, overridable via DOUBAN_FRODO_UA for
	// rotation, the same way the key and secret are.
	cfg.UserAgent = firstNonEmpty(os.Getenv("DOUBAN_FRODO_UA"), cfg.UserAgent)
	return douban.NewFrodoClient(cfg)
}

// seedIDs seeds every id in [from, to] for one entity type.
func seedIDs(st *store.Store, typ string, from, to int) (int, error) {
	if typ == "" || to < from {
		return 0, errs.Usage("need --type and --from <= --to")
	}
	added := 0
	for id := from; id <= to; id++ {
		u, ok := canonicalURL(typ, strconv.Itoa(id))
		if !ok {
			return 0, errs.Usage("unknown entity type %q", typ)
		}
		if enqueueURL(st, u) {
			added++
		}
	}
	return added, nil
}

// seedFromList seeds URLs from a file, one per line, skipping blanks and
// comment lines.
func seedFromList(st *store.Store, path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, errs.Wrap(errs.KindGeneric, err, "read list")
	}
	added := 0
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if enqueueURL(st, line) {
			added++
		}
	}
	return added, nil
}

// enqueueURL classifies a URL and adds it to the frontier, returning whether a
// new row was created.
func enqueueURL(st *store.Store, rawURL string) bool {
	c, ok := mirror.Classify(rawURL)
	if !ok {
		return false
	}
	// Seed the canonical URL so a pasted sub-page (/comments, /reviews) seeds
	// the entity itself, matching how discovered links are enqueued.
	url := c.Canonical
	if url == "" {
		url = rawURL
	}
	added, err := st.Enqueue(store.Frontier{
		URL: url, EntityType: c.EntityType, EntityID: c.EntityID, Source: c.Source,
	})
	return err == nil && added
}

func bandNames() []string {
	out := make([]string, 0, len(sitemap.Bands))
	for _, b := range sitemap.Bands {
		out = append(out, b.Name)
	}
	return out
}

func selectBands(children, bands []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, name := range bands {
		for _, c := range sitemap.SelectBand(children, name) {
			if !seen[c] {
				seen[c] = true
				out = append(out, c)
			}
		}
	}
	return out
}

func firstType(types []string) string {
	if len(types) > 0 {
		return types[0]
	}
	return ""
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// canonicalURL builds the canonical Douban URL for an entity id, used by id-range
// seeding. It delegates to the mirror package, the single source of truth for
// the one URL that represents an entity.
func canonicalURL(typ, id string) (string, bool) {
	return mirror.CanonicalURL(typ, id)
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func dirSize(dir string) int64 {
	var total int64
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if info, err := d.Info(); err == nil && !d.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
