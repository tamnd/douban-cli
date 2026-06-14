package cli

import (
	"slices"
	"testing"

	"github.com/tamnd/any-cli/kit"
)

// TestAppHasLookupOps asserts the seven lookup commands are registered as kit
// operations, so the CLI, the HTTP API, and the MCP server all expose them.
func TestAppHasLookupOps(t *testing.T) {
	app := NewApp()
	have := map[string]bool{}
	for _, op := range app.Ops() {
		have[op.Meta().Name] = true
	}
	for _, w := range []string{"search", "suggest", "book", "top250", "chart", "nowplaying", "doulist"} {
		if !have[w] {
			t.Errorf("missing operation %q", w)
		}
	}
}

// TestLookupOpsGrouped checks every lookup op sits in the read group so help
// lists them together.
func TestLookupOpsGrouped(t *testing.T) {
	for _, op := range NewApp().Ops() {
		if op.Meta().Group != "read" {
			t.Errorf("op %q group = %q, want read", op.Meta().Name, op.Meta().Group)
		}
	}
}

// TestUserAgentFinalize verifies the domain-global --user-agent flag is folded
// onto the resolved config's Extra map, where ClientFromConfig reads it.
func TestUserAgentFinalize(t *testing.T) {
	b := &builder{userAgent: "douban-cli/test"}
	var c kit.Config
	b.finalize(&c)
	if c.Extra["user-agent"] != "douban-cli/test" {
		t.Errorf("user-agent not folded into config: %v", c.Extra)
	}
}

func TestVersionCmd(t *testing.T) {
	cmd := versionCmd()
	if cmd.Use != "version" {
		t.Errorf("version Use = %q", cmd.Use)
	}
	if cmd.Run == nil {
		t.Fatal("version command has no Run")
	}
}

func TestMirrorCommandsRegistered(t *testing.T) {
	// The constructors are the source of truth for the escape-hatch tree; the
	// App stores them privately, so assert the constructors directly.
	got := []string{
		seedCmd().Use, crawlCmd().Use, exportCmd().Use,
		infoCmd().Use, queueCmd().Use, resetFailedCmd().Use,
	}
	for _, w := range []string{"seed", "crawl", "export", "info", "queue", "reset-failed"} {
		if !slices.Contains(got, w) {
			t.Errorf("missing mirror command %q (have %v)", w, got)
		}
	}
}
