package cli

import (
	"bytes"
	"errors"
	"testing"

	"github.com/tamnd/douban-cli/douban"
)

func TestRootHasAllSubcommands(t *testing.T) {
	root := Root()
	want := []string{"search", "suggest", "book", "top250", "chart", "nowplaying", "doulist", "version"}
	have := map[string]bool{}
	for _, c := range root.Commands() {
		have[c.Name()] = true
	}
	for _, w := range want {
		if !have[w] {
			t.Errorf("missing subcommand %q", w)
		}
	}
}

func TestRootPersistentFlags(t *testing.T) {
	pf := Root().PersistentFlags()
	for _, name := range []string{"output", "fields", "no-header", "template", "limit", "quiet", "delay", "timeout", "retries", "user-agent"} {
		if pf.Lookup(name) == nil {
			t.Errorf("missing persistent flag %q", name)
		}
	}
}

func TestSetupRejectsBadFormat(t *testing.T) {
	a := &App{cfg: douban.DefaultConfig(), output: "yaml"}
	err := a.setup()
	if err == nil {
		t.Fatal("want error for bad format")
	}
	var ee *ExitError
	if !errors.As(err, &ee) || ee.Code != exitUsage {
		t.Fatalf("err = %v, want exitUsage", err)
	}
}

func TestSetupValidFormatBuildsClient(t *testing.T) {
	a := &App{cfg: douban.DefaultConfig(), output: "json"}
	if err := a.setup(); err != nil {
		t.Fatal(err)
	}
	if a.client == nil {
		t.Fatal("client not built")
	}
}

func TestEffectiveLimit(t *testing.T) {
	a := &App{}
	if got := a.effectiveLimit(15); got != 15 {
		t.Errorf("default = %d, want 15", got)
	}
	a.limit = 5
	if got := a.effectiveLimit(15); got != 5 {
		t.Errorf("override = %d, want 5", got)
	}
}

func TestFailMapsNotFound(t *testing.T) {
	a := &App{}
	var ee *ExitError
	if err := a.fail(douban.ErrNotFound); !errors.As(err, &ee) || ee.Code != exitNoData {
		t.Fatalf("notfound -> %v, want exitNoData", err)
	}
	if err := a.fail(errors.New("boom")); !errors.As(err, &ee) || ee.Code != exitError {
		t.Fatalf("other -> %v, want exitError", err)
	}
}

func TestExitError(t *testing.T) {
	base := errors.New("boom")
	ee := &ExitError{Code: 7, Err: base}
	if ee.Error() != "boom" {
		t.Errorf("Error() = %q", ee.Error())
	}
	if !errors.Is(ee, base) {
		t.Error("Unwrap broken")
	}
	if (&ExitError{Code: 3}).Error() != "exit 3" {
		t.Error("no-err message wrong")
	}
}

func TestVersionCommand(t *testing.T) {
	root := Root()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"version", "--short"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if out.String() == "" {
		t.Error("version --short printed nothing")
	}
}

func TestSearchRequiresQuery(t *testing.T) {
	root := Root()
	root.SetArgs([]string{"search"})
	if err := root.Execute(); err == nil {
		t.Fatal("want error when query missing")
	}
}

func TestBadFormatThroughExecute(t *testing.T) {
	root := Root()
	root.SetArgs([]string{"-o", "yaml", "search", "x"})
	err := root.Execute()
	var ee *ExitError
	if !errors.As(err, &ee) || ee.Code != exitUsage {
		t.Fatalf("err = %v, want exitUsage", err)
	}
}
