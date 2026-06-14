package cli

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/tamnd/douban-cli/douban"
	"github.com/tamnd/douban-cli/mirror"
	"github.com/tamnd/douban-cli/mirror/crawl"
	"github.com/tamnd/douban-cli/mirror/sitemap"
	"github.com/tamnd/douban-cli/mirror/store"
)

// addMirrorFlags binds the data dir and Frodo credential overrides to a command.
func (a *App) addMirrorFlags(cmd *cobra.Command) {
	f := cmd.Flags()
	f.StringVar(&a.dataDir, "data", "", "mirror directory (default $DOUBAN_DATA or $HOME/data/douban)")
	f.StringVar(&a.frodoKey, "frodo-key", "", "override the Frodo API key (or $DOUBAN_FRODO_KEY)")
	f.StringVar(&a.frodoSecret, "frodo-secret", "", "override the Frodo API secret (or $DOUBAN_FRODO_SECRET)")
}

// dataPath resolves the mirror directory: --data, then $DOUBAN_DATA, then
// $HOME/data/douban.
func (a *App) dataPath() string {
	if a.dataDir != "" {
		return a.dataDir
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

// openStore opens the mirror store at the resolved data path.
func (a *App) openStore() (*store.Store, error) {
	st, err := store.Open(a.dataPath())
	if err != nil {
		return nil, codeError(exitError, err)
	}
	return st, nil
}

// frodoClient builds a Frodo client from defaults plus flag/env overrides.
func (a *App) frodoClient() *douban.FrodoClient {
	cfg := douban.DefaultFrodoConfig()
	cfg.APIKey = firstNonEmpty(a.frodoKey, os.Getenv("DOUBAN_FRODO_KEY"), cfg.APIKey)
	cfg.Secret = firstNonEmpty(a.frodoSecret, os.Getenv("DOUBAN_FRODO_SECRET"), cfg.Secret)
	cfg.UserAgent = a.cfg.UserAgent
	return douban.NewFrodoClient(cfg)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// --- seed ---

func (a *App) seedCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "seed",
		Short: "Add URLs to the crawl frontier",
		Long: `Populate the crawl frontier from Douban's sitemap, an id range, a single
URL, or a file of URLs. Seeding is idempotent: a URL already in the frontier
keeps its status and history.`,
	}
	a.addMirrorFlags(cmd)
	cmd.AddCommand(a.seedSitemapCmd(), a.seedIDsCmd(), a.seedURLCmd(), a.seedListCmd())
	return cmd
}

func (a *App) seedSitemapCmd() *cobra.Command {
	var bands []string
	var since bool
	cmd := &cobra.Command{
		Use:   "sitemap",
		Short: "Seed from the site sitemap, optionally banded by entity type",
		Long: `Enumerate URLs from Douban's single sitemap backbone and add the entity
URLs to the frontier. Sitemaps are partitioned into contiguous bands by entity
type, so --band fetches only the types you want instead of all children.

  douban seed sitemap --band subject --limit 5000
  douban seed sitemap --band celebrity --band musician
  douban seed sitemap --since           # the daily updated feed

Known bands: ` + strings.Join(bandNames(), ", "),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			st, err := a.openStore()
			if err != nil {
				return err
			}
			defer func() { _ = st.Close() }()

			indexURL := sitemap.IndexURL
			if since {
				indexURL = sitemap.UpdatedIndexURL
			}
			a.progressf("fetching sitemap index %s...", indexURL)
			children, err := sitemap.Index(cmd.Context(), a.client, indexURL)
			if err != nil {
				return codeError(exitError, err)
			}
			if !since && len(bands) > 0 {
				children = selectBands(children, bands)
			}
			a.progressf("walking %d child sitemaps...", len(children))

			limit := a.limit
			added := 0
			for i, child := range children {
				if limit > 0 && added >= limit {
					break
				}
				locs, err := sitemap.Children(cmd.Context(), a.client, child)
				if err != nil {
					a.progressf("child %d (%s): %v", i, child, err)
					continue
				}
				for _, loc := range locs {
					if limit > 0 && added >= limit {
						break
					}
					if a.enqueueURL(st, loc) {
						added++
					}
				}
				a.progressf("child %d/%d: %d urls seeded", i+1, len(children), added)
			}
			_, _ = fmt.Fprintf(os.Stdout, "seeded %d urls\n", added)
			if added == 0 {
				return codeError(exitNoData, nil)
			}
			return nil
		},
	}
	cmd.Flags().StringSliceVar(&bands, "band", nil, "limit to these entity bands (repeatable)")
	cmd.Flags().BoolVar(&since, "since", false, "use the daily updated index instead of the full one")
	a.addMirrorFlags(cmd)
	return cmd
}

func (a *App) seedIDsCmd() *cobra.Command {
	var typ string
	var from, to int
	cmd := &cobra.Command{
		Use:   "ids --type T --from N --to M",
		Short: "Seed a contiguous id range for one entity type",
		Long: `Seed every id in [from, to] for one entity type. Useful for the dense,
low-cardinality id spaces (book subjects, movie celebrities) where id-stride
enumeration catches ids newer than the last sitemap snapshot.

  douban seed ids --type book --from 1084336 --to 1084340
  douban seed ids --type celebrity --from 1601851 --to 1601861`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if typ == "" || to < from {
				return codeError(exitUsage, fmt.Errorf("need --type and --from <= --to"))
			}
			st, err := a.openStore()
			if err != nil {
				return err
			}
			defer func() { _ = st.Close() }()

			added := 0
			for id := from; id <= to; id++ {
				u, ok := canonicalURL(typ, strconv.Itoa(id))
				if !ok {
					return codeError(exitUsage, fmt.Errorf("unknown entity type %q", typ))
				}
				if a.enqueueURL(st, u) {
					added++
				}
			}
			_, _ = fmt.Fprintf(os.Stdout, "seeded %d urls\n", added)
			if added == 0 {
				return codeError(exitNoData, nil)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&typ, "type", "", "entity type (book, movie, music, celebrity, personage, ...)")
	cmd.Flags().IntVar(&from, "from", 0, "first id (inclusive)")
	cmd.Flags().IntVar(&to, "to", 0, "last id (inclusive)")
	a.addMirrorFlags(cmd)
	return cmd
}

func (a *App) seedURLCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "url <url>...",
		Short: "Seed one or more explicit URLs",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			st, err := a.openStore()
			if err != nil {
				return err
			}
			defer func() { _ = st.Close() }()
			added := 0
			for _, u := range args {
				if a.enqueueURL(st, u) {
					added++
				} else {
					a.progressf("skip (unclassified): %s", u)
				}
			}
			_, _ = fmt.Fprintf(os.Stdout, "seeded %d urls\n", added)
			if added == 0 {
				return codeError(exitNoData, nil)
			}
			return nil
		},
	}
	a.addMirrorFlags(cmd)
	return cmd
}

func (a *App) seedListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list <file>",
		Short: "Seed URLs from a file, one per line",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			data, err := os.ReadFile(args[0])
			if err != nil {
				return codeError(exitError, err)
			}
			st, err := a.openStore()
			if err != nil {
				return err
			}
			defer func() { _ = st.Close() }()
			added := 0
			for _, line := range strings.Split(string(data), "\n") {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				if a.enqueueURL(st, line) {
					added++
				}
			}
			_, _ = fmt.Fprintf(os.Stdout, "seeded %d urls\n", added)
			if added == 0 {
				return codeError(exitNoData, nil)
			}
			return nil
		},
	}
	a.addMirrorFlags(cmd)
	return cmd
}

// enqueueURL classifies a URL and adds it to the frontier, returning whether a
// new row was created.
func (a *App) enqueueURL(st *store.Store, rawURL string) bool {
	c, ok := mirror.Classify(rawURL)
	if !ok {
		return false
	}
	added, err := st.Enqueue(store.Frontier{
		URL: rawURL, EntityType: c.EntityType, EntityID: c.EntityID, Source: c.Source,
	})
	return err == nil && added
}

// --- crawl ---

func (a *App) crawlCmd() *cobra.Command {
	var types []string
	var source string
	var concurrency int
	var retryFailed bool
	cmd := &cobra.Command{
		Use:   "crawl",
		Short: "Crawl pending frontier URLs into the mirror",
		Long: `Drain the frontier: fetch each pending URL through the source that serves
it, archive the raw bytes, record the normalized entity, and enqueue the links
it discovers. The crawl is resumable, so an interrupted run picks up where it
left off, and --limit bounds a single pass.

  douban crawl --limit 20
  douban crawl --type movie --source frodo --concurrency 4
  douban crawl --retry-failed`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			st, err := a.openStore()
			if err != nil {
				return err
			}
			defer func() { _ = st.Close() }()

			if retryFailed {
				n, err := st.ResetFailed(firstType(types))
				if err != nil {
					return codeError(exitError, err)
				}
				a.progressf("requeued %d failed urls", n)
			}

			eng := crawl.New(st, a.client, a.frodoClient(), crawl.Config{
				Concurrency: concurrency,
				Limit:       a.limit,
				HTMLDelay:   a.crawlDelay(),
				Types:       types,
				Source:      source,
				Logf: func(format string, args ...any) {
					a.progressf(format, args...)
				},
			})
			a.progressf("crawling...")
			stats, err := eng.Run(cmd.Context())
			if err != nil {
				return codeError(exitError, err)
			}
			_, _ = fmt.Fprintf(os.Stdout,
				"processed %d: %d done, %d blocked, %d skipped, %d failed, %d discovered\n",
				stats.Processed, stats.Done, stats.Blocked, stats.Skipped, stats.Failed, stats.Discovered)
			if stats.Processed == 0 {
				return codeError(exitNoData, nil)
			}
			return nil
		},
	}
	cmd.Flags().StringSliceVar(&types, "type", nil, "restrict to these entity types")
	cmd.Flags().StringVar(&source, "source", "", "restrict to a source (frodo|html)")
	cmd.Flags().IntVar(&concurrency, "concurrency", 4, "number of concurrent workers")
	cmd.Flags().BoolVar(&retryFailed, "retry-failed", false, "requeue failed urls before crawling")
	a.addMirrorFlags(cmd)
	return cmd
}

// crawlDelay honors --delay when the operator set it, else defaults to the
// polite web Crawl-delay by leaving the engine default (5s) in place.
func (a *App) crawlDelay() time.Duration {
	if a.cfg.Rate != douban.DefaultConfig().Rate {
		return a.cfg.Rate
	}
	return 0
}

// --- export ---

func (a *App) exportCmd() *cobra.Command {
	var typ, out string
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export normalized records as JSONL",
		Long: `Stream normalized records as JSONL. With no --out the records go to stdout;
with --out DIR they are written to DIR/<type>.jsonl.

  douban export --type movie | jq .
  douban export --out ./export`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			st, err := a.openStore()
			if err != nil {
				return err
			}
			defer func() { _ = st.Close() }()

			if out == "" {
				n, err := st.ExportJSONL(cmd.Context(), typ, os.Stdout)
				if err != nil {
					return codeError(exitError, err)
				}
				if n == 0 {
					return codeError(exitNoData, nil)
				}
				return nil
			}

			if err := os.MkdirAll(out, 0o755); err != nil {
				return codeError(exitError, err)
			}
			name := typ
			if name == "" {
				name = "all"
			}
			path := filepath.Join(out, name+".jsonl")
			f, err := os.Create(path)
			if err != nil {
				return codeError(exitError, err)
			}
			defer func() { _ = f.Close() }()
			n, err := st.ExportJSONL(cmd.Context(), typ, f)
			if err != nil {
				return codeError(exitError, err)
			}
			_, _ = fmt.Fprintf(os.Stdout, "wrote %d records to %s\n", n, path)
			if n == 0 {
				return codeError(exitNoData, nil)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&typ, "type", "", "restrict to one entity type")
	cmd.Flags().StringVar(&out, "out", "", "write to DIR/<type>.jsonl instead of stdout")
	a.addMirrorFlags(cmd)
	return cmd
}

// --- info ---

func (a *App) infoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "info",
		Short: "Show mirror location, counts, and disk usage",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			st, err := a.openStore()
			if err != nil {
				return err
			}
			defer func() { _ = st.Close() }()

			dir := a.dataPath()
			_, _ = fmt.Fprintf(os.Stdout, "data dir: %s\n", dir)
			_, _ = fmt.Fprintf(os.Stdout, "database: %s\n", st.DBPath())
			_, _ = fmt.Fprintf(os.Stdout, "disk:     %s\n", humanBytes(dirSize(dir)))

			recs, err := st.RecordCounts()
			if err != nil {
				return codeError(exitError, err)
			}
			_, _ = fmt.Fprintf(os.Stdout, "\nrecords by type:\n")
			for _, t := range sortedKeys(recs) {
				_, _ = fmt.Fprintf(os.Stdout, "  %-14s %d\n", t, recs[t])
			}

			fc, err := st.FrontierCounts()
			if err != nil {
				return codeError(exitError, err)
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
	a.addMirrorFlags(cmd)
	return cmd
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
	Error      string `json:"error,omitempty"`
}

func (a *App) queueCmd() *cobra.Command {
	var status, typ string
	cmd := &cobra.Command{
		Use:   "queue",
		Short: "Inspect frontier rows by status and type",
		Long: `List frontier rows, optionally filtered by status and entity type.

  douban queue --status failed
  douban queue --status blocked --type note -n 50`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			st, err := a.openStore()
			if err != nil {
				return err
			}
			defer func() { _ = st.Close() }()

			rows, err := st.QueueRows(status, typ, a.effectiveLimit(100))
			if err != nil {
				return codeError(exitError, err)
			}
			out := make([]QueueRow, 0, len(rows))
			for _, r := range rows {
				out = append(out, QueueRow{
					URL: r.URL, EntityType: r.EntityType, EntityID: r.EntityID,
					Source: r.Source, Status: r.Status, Attempts: r.Attempts,
					HTTPStatus: r.HTTPStatus, Error: r.Error,
				})
			}
			return a.renderOrEmpty(out, len(out))
		},
	}
	cmd.Flags().StringVar(&status, "status", "", "filter by status (pending|done|failed|blocked|skipped)")
	cmd.Flags().StringVar(&typ, "type", "", "filter by entity type")
	a.addMirrorFlags(cmd)
	return cmd
}

// --- reset-failed ---

func (a *App) resetFailedCmd() *cobra.Command {
	var typ string
	cmd := &cobra.Command{
		Use:   "reset-failed",
		Short: "Requeue failed frontier rows for another crawl",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			st, err := a.openStore()
			if err != nil {
				return err
			}
			defer func() { _ = st.Close() }()
			n, err := st.ResetFailed(typ)
			if err != nil {
				return codeError(exitError, err)
			}
			_, _ = fmt.Fprintf(os.Stdout, "requeued %d urls\n", n)
			if n == 0 {
				return codeError(exitNoData, nil)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&typ, "type", "", "restrict to one entity type")
	a.addMirrorFlags(cmd)
	return cmd
}

// --- shared helpers ---

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

// canonicalURL builds the canonical Douban URL for an entity id, used by id-range
// seeding. The classifier then derives the source from the URL.
func canonicalURL(typ, id string) (string, bool) {
	switch typ {
	case "book":
		return "https://book.douban.com/subject/" + id + "/", true
	case "movie", "tv":
		return "https://movie.douban.com/subject/" + id + "/", true
	case "music":
		return "https://music.douban.com/subject/" + id + "/", true
	case "thing":
		return "https://9.douban.com/subject/" + id + "/", true
	case "subject":
		return "https://www.douban.com/subject/" + id + "/", true
	case "game":
		return "https://www.douban.com/game/" + id + "/", true
	case "celebrity":
		return "https://movie.douban.com/celebrity/" + id + "/", true
	case "musician":
		return "https://music.douban.com/musician/" + id + "/", true
	case "personage":
		return "https://www.douban.com/personage/" + id + "/", true
	case "doulist":
		return "https://www.douban.com/doulist/" + id + "/", true
	case "drama":
		return "https://www.douban.com/location/drama/" + id + "/", true
	case "people":
		return "https://www.douban.com/people/" + id + "/", true
	case "group-topic":
		return "https://www.douban.com/group/topic/" + id + "/", true
	case "group":
		return "https://www.douban.com/group/" + id + "/", true
	case "event":
		return "https://www.douban.com/event/" + id + "/", true
	case "note":
		return "https://www.douban.com/note/" + id + "/", true
	case "review":
		return "https://movie.douban.com/review/" + id + "/", true
	case "photo-album":
		return "https://www.douban.com/photos/album/" + id + "/", true
	default:
		return "", false
	}
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
