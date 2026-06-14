package douban

import (
	"context"
	"errors"
	"testing"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

// TestDomainRegistersOps builds a kit App, registers the douban domain, and
// asserts the seven lookup operations are present. Those same ops back the CLI,
// the HTTP API, and the MCP server.
func TestDomainRegistersOps(t *testing.T) {
	app := kit.New(Identity())
	Domain{}.Register(app)
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

func TestClientFromConfig(t *testing.T) {
	def := DefaultConfig()
	c := ClientFromConfig(kit.Config{
		Rate: 0, Retries: -1, Timeout: 0,
		Extra: map[string]string{"user-agent": "x/1"},
	})
	// Zero/negative framework values keep the douban defaults.
	if c.cfg.Rate != def.Rate {
		t.Errorf("rate = %v, want default %v", c.cfg.Rate, def.Rate)
	}
	if c.cfg.UserAgent != "x/1" {
		t.Errorf("user-agent = %q, want x/1", c.cfg.UserAgent)
	}
}

func TestDefaults(t *testing.T) {
	var c kit.Config
	Defaults(&c)
	def := DefaultConfig()
	if c.Rate != def.Rate || c.Timeout != def.Timeout || c.UserAgent != def.UserAgent {
		t.Errorf("Defaults did not seed douban baseline: %+v", c)
	}
}

func TestMapErr(t *testing.T) {
	if mapErr(nil) != nil {
		t.Error("nil should map to nil")
	}
	if got := mapErr(ErrNotFound); errs.KindOf(got) != errs.KindNoResults {
		t.Errorf("ErrNotFound maps to %v, want no-results", errs.KindOf(got))
	}
	boom := errors.New("boom")
	if got := mapErr(boom); !errors.Is(got, boom) {
		t.Errorf("other error not passed through: %v", got)
	}
}

func TestLimitOr(t *testing.T) {
	if got := limitOr(0, 15); got != 15 {
		t.Errorf("limitOr(0,15) = %d, want 15", got)
	}
	if got := limitOr(5, 15); got != 5 {
		t.Errorf("limitOr(5,15) = %d, want 5", got)
	}
}

// TestHandlersEmit drives the handlers against the surface mux, confirming the
// operation layer threads input to the client and forwards each record to emit.
func TestHandlersEmit(t *testing.T) {
	c := muxClient(t)
	ctx := context.Background()

	var sug []Suggestion
	if err := suggest(ctx, suggestIn{Client: c, Query: "matrix", Type: "movie"}, func(s Suggestion) error {
		sug = append(sug, s)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(sug) != 1 || sug[0].Title != "黑客帝国" {
		t.Errorf("suggest emitted %+v", sug)
	}

	var book *Book
	if err := getBook(ctx, bookIn{Client: c, Ref: "https://book.douban.com/subject/1084336/"}, func(b *Book) error {
		book = b
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if book == nil || book.ID != "1084336" {
		t.Errorf("getBook emitted %+v", book)
	}

	var doul []DoulistItem
	if err := doulist(ctx, doulistIn{Client: c, Ref: "240962"}, func(it DoulistItem) error {
		doul = append(doul, it)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(doul) == 0 {
		t.Error("doulist emitted nothing")
	}
}

// TestGetBookNotFound confirms a missing subject reaches the handler as the
// no-results kind, so the CLI exits 3 rather than 1.
func TestGetBookNotFound(t *testing.T) {
	c := muxClient(t)
	err := getBook(context.Background(), bookIn{Client: c, Ref: "404"}, func(*Book) error { return nil })
	if errs.KindOf(err) != errs.KindNoResults {
		t.Fatalf("err kind = %v, want no-results", errs.KindOf(err))
	}
}
