package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tamnd/douban-cli/douban"
	"github.com/tamnd/douban-cli/mirror/store"
)

func TestRootHasMirrorCommands(t *testing.T) {
	root := Root()
	have := map[string]bool{}
	for _, c := range root.Commands() {
		have[c.Name()] = true
	}
	for _, w := range []string{"seed", "crawl", "export", "info", "queue", "reset-failed"} {
		if !have[w] {
			t.Errorf("missing mirror subcommand %q", w)
		}
	}
}

func TestSeedHasSubcommands(t *testing.T) {
	a := &App{}
	have := map[string]bool{}
	for _, c := range a.seedCmd().Commands() {
		have[c.Name()] = true
	}
	for _, w := range []string{"sitemap", "ids", "url", "list"} {
		if !have[w] {
			t.Errorf("missing seed subcommand %q", w)
		}
	}
}

func TestMirrorFlagsBound(t *testing.T) {
	a := &App{}
	cc := a.crawlCmd()
	for _, f := range []string{"data", "frodo-key", "frodo-secret", "type", "source", "concurrency", "retry-failed"} {
		if cc.Flags().Lookup(f) == nil {
			t.Errorf("crawl missing flag %q", f)
		}
	}
	ec := a.exportCmd()
	for _, f := range []string{"data", "type", "out"} {
		if ec.Flags().Lookup(f) == nil {
			t.Errorf("export missing flag %q", f)
		}
	}
	qc := a.queueCmd()
	for _, f := range []string{"data", "status", "type"} {
		if qc.Flags().Lookup(f) == nil {
			t.Errorf("queue missing flag %q", f)
		}
	}
}

func TestDataPath(t *testing.T) {
	// flag wins
	a := &App{dataDir: "/tmp/explicit"}
	if got := a.dataPath(); got != "/tmp/explicit" {
		t.Errorf("flag path = %q", got)
	}
	// env fallback
	t.Setenv("DOUBAN_DATA", "/tmp/fromenv")
	b := &App{}
	if got := b.dataPath(); got != "/tmp/fromenv" {
		t.Errorf("env path = %q", got)
	}
}

func TestCanonicalURL(t *testing.T) {
	cases := map[string][2]string{
		"book":      {"1084336", "https://book.douban.com/subject/1084336/"},
		"movie":     {"1292052", "https://movie.douban.com/subject/1292052/"},
		"celebrity": {"1054521", "https://movie.douban.com/celebrity/1054521/"},
		"personage": {"27259736", "https://www.douban.com/personage/27259736/"},
	}
	for typ, want := range cases {
		got, ok := canonicalURL(typ, want[0])
		if !ok || got != want[1] {
			t.Errorf("canonicalURL(%q,%q) = %q,%v want %q", typ, want[0], got, ok, want[1])
		}
	}
	if _, ok := canonicalURL("nonsense", "1"); ok {
		t.Error("unknown type should not classify")
	}
}

func TestSeedIDsAndQueue(t *testing.T) {
	dir := t.TempDir()
	a := &App{cfg: douban.DefaultConfig(), output: "json", dataDir: dir, quiet: true}
	if err := a.setup(); err != nil {
		t.Fatal(err)
	}

	cmd := a.seedCmd()
	cmd.SetArgs([]string{"ids", "--type", "book", "--from", "100", "--to", "104", "--data", dir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("seed ids: %v", err)
	}

	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	rows, err := st.QueueRows("pending", "book", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 5 {
		t.Fatalf("seeded %d rows, want 5", len(rows))
	}
	if rows[0].URL != "https://book.douban.com/subject/100/" {
		t.Errorf("url = %q", rows[0].URL)
	}
}

func TestSeedListAndInfo(t *testing.T) {
	dir := t.TempDir()
	listFile := filepath.Join(dir, "urls.txt")
	content := "https://book.douban.com/subject/200/\n# a comment\n\nhttps://movie.douban.com/subject/300/\n"
	if err := os.WriteFile(listFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	a := &App{cfg: douban.DefaultConfig(), output: "json", quiet: true}
	if err := a.setup(); err != nil {
		t.Fatal(err)
	}
	cmd := a.seedCmd()
	cmd.SetArgs([]string{"list", listFile, "--data", dir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("seed list: %v", err)
	}

	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	fc, err := st.FrontierCounts()
	if err != nil {
		t.Fatal(err)
	}
	if fc["book"]["pending"] != 1 || fc["movie"]["pending"] != 1 {
		t.Errorf("frontier counts = %v, want one book and one movie", fc)
	}
}

func TestResetFailedCommand(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Enqueue(store.Frontier{
		URL: "https://book.douban.com/subject/9/", EntityType: "book", EntityID: "9", Source: store.SourceHTML,
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.Complete("https://book.douban.com/subject/9/", store.StatusFailed, 500, "boom", "", ""); err != nil {
		t.Fatal(err)
	}
	_ = st.Close()

	a := &App{cfg: douban.DefaultConfig(), output: "json", quiet: true}
	if err := a.setup(); err != nil {
		t.Fatal(err)
	}
	cmd := a.resetFailedCmd()
	cmd.SetArgs([]string{"--data", dir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("reset-failed: %v", err)
	}

	st2, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st2.Close() }()
	rows, err := st2.QueueRows("pending", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("pending after reset = %d, want 1", len(rows))
	}
}

func TestHumanBytes(t *testing.T) {
	cases := map[int64]string{
		512:             "512 B",
		2048:            "2.0 KiB",
		5 * 1024 * 1024: "5.0 MiB",
	}
	for n, want := range cases {
		if got := humanBytes(n); got != want {
			t.Errorf("humanBytes(%d) = %q, want %q", n, got, want)
		}
	}
}
