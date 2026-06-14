package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/douban-cli/mirror/store"
)

// firstWord returns the command verb from a Use string like "ids --type T".
func firstWord(use string) string {
	if i := strings.IndexByte(use, ' '); i >= 0 {
		return use[:i]
	}
	return use
}

func TestSeedHasSubcommands(t *testing.T) {
	have := map[string]bool{}
	for _, c := range seedCmd().Sub {
		have[firstWord(c.Use)] = true
	}
	for _, w := range []string{"sitemap", "ids", "url", "list"} {
		if !have[w] {
			t.Errorf("missing seed subcommand %q", w)
		}
	}
}

func TestDataPathOf(t *testing.T) {
	if got := dataPathOf("/tmp/explicit"); got != "/tmp/explicit" {
		t.Errorf("flag path = %q", got)
	}
	t.Setenv("DOUBAN_DATA", "/tmp/fromenv")
	if got := dataPathOf(""); got != "/tmp/fromenv" {
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

func TestSeedIDs(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()

	added, err := seedIDs(st, "book", 100, 104)
	if err != nil {
		t.Fatalf("seedIDs: %v", err)
	}
	if added != 5 {
		t.Fatalf("added %d, want 5", added)
	}
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

func TestSeedIDsRejectsBadRange(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()

	if _, err := seedIDs(st, "", 1, 2); err == nil {
		t.Error("want error for missing type")
	}
	if _, err := seedIDs(st, "book", 5, 1); err == nil {
		t.Error("want error for from > to")
	}
}

func TestSeedFromList(t *testing.T) {
	dir := t.TempDir()
	listFile := filepath.Join(dir, "urls.txt")
	content := "https://book.douban.com/subject/200/\n# a comment\n\nhttps://movie.douban.com/subject/300/\n"
	if err := os.WriteFile(listFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()

	added, err := seedFromList(st, listFile)
	if err != nil {
		t.Fatalf("seedFromList: %v", err)
	}
	if added != 2 {
		t.Fatalf("added %d, want 2 (comment and blank skipped)", added)
	}
	fc, err := st.FrontierCounts()
	if err != nil {
		t.Fatal(err)
	}
	if fc["book"]["pending"] != 1 || fc["movie"]["pending"] != 1 {
		t.Errorf("frontier counts = %v, want one book and one movie", fc)
	}
}

func TestEnqueueURLSkipsUnclassified(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()

	if enqueueURL(st, "https://example.com/not-douban") {
		t.Error("unclassified URL should not be enqueued")
	}
	if !enqueueURL(st, "https://book.douban.com/subject/1084336/") {
		t.Error("a valid book URL should be enqueued")
	}
}

// TestFrodoClientKeepsAppUA guards the regression where the generic CLI
// User-Agent was forced onto the Frodo client, which made the host answer
// invalid_apikey (code 1062). The Frodo client must always carry an app UA and
// honor DOUBAN_FRODO_UA for rotation, independent of the CLI --user-agent value.
func TestFrodoClientKeepsAppUA(t *testing.T) {
	if ua := frodoClientFrom("", "").UserAgent(); !strings.Contains(ua, "com.douban.frodo") {
		t.Errorf("frodo UA = %q, want the app UA", ua)
	}
	t.Setenv("DOUBAN_FRODO_UA", "api-client/1 com.douban.frodo/9.9.9(999) custom")
	if ua := frodoClientFrom("", "").UserAgent(); ua != "api-client/1 com.douban.frodo/9.9.9(999) custom" {
		t.Errorf("DOUBAN_FRODO_UA not honored, got %q", ua)
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
