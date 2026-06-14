package sitemap

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"testing"

	"github.com/tamnd/douban-cli/mirror"
)

func TestParseIndex(t *testing.T) {
	doc := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <sitemap><loc>https://www.douban.com/sitemap.xml.gz</loc></sitemap>
  <sitemap><loc>https://www.douban.com/sitemap1.xml.gz</loc></sitemap>
  <sitemap><loc>  https://www.douban.com/sitemap2.xml.gz  </loc></sitemap>
</sitemapindex>`)
	got, err := ParseIndex(doc)
	if err != nil {
		t.Fatalf("ParseIndex: %v", err)
	}
	if len(got) != 3 || got[2] != "https://www.douban.com/sitemap2.xml.gz" {
		t.Fatalf("got %v", got)
	}
}

func TestParseURLSetAndClassify(t *testing.T) {
	doc := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>https://book.douban.com/subject/1084336/</loc></url>
  <url><loc>https://movie.douban.com/subject/1292052/</loc></url>
  <url><loc>https://www.douban.com/note/37082541/</loc></url>
</urlset>`)
	locs, err := ParseURLSet(doc)
	if err != nil {
		t.Fatalf("ParseURLSet: %v", err)
	}
	if len(locs) != 3 {
		t.Fatalf("locs = %d, want 3", len(locs))
	}
	c, ok := mirror.Classify(locs[1])
	if !ok || c.EntityType != "movie" || c.Source != mirror.SourceFrodo {
		t.Fatalf("classify movie = %+v ok=%v", c, ok)
	}
}

func TestGunzipPassthrough(t *testing.T) {
	plain := []byte("<urlset></urlset>")
	if out, err := Gunzip(plain); err != nil || !bytes.Equal(out, plain) {
		t.Fatalf("passthrough: out=%s err=%v", out, err)
	}

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	_, _ = gz.Write(plain)
	_ = gz.Close()
	out, err := Gunzip(buf.Bytes())
	if err != nil || !bytes.Equal(out, plain) {
		t.Fatalf("gunzip: out=%s err=%v", out, err)
	}
}

// stubFetcher serves canned bytes per URL.
type stubFetcher map[string][]byte

func (s stubFetcher) Get(_ context.Context, url string) ([]byte, error) {
	if b, ok := s[url]; ok {
		return b, nil
	}
	return nil, fmt.Errorf("no stub for %s", url)
}

func TestIndexAndChildren(t *testing.T) {
	index := []byte(`<sitemapindex><sitemap><loc>https://www.douban.com/sitemap.xml.gz</loc></sitemap></sitemapindex>`)

	var child bytes.Buffer
	gz := gzip.NewWriter(&child)
	_, _ = gz.Write([]byte(`<urlset><url><loc>https://book.douban.com/subject/1/</loc></url></urlset>`))
	_ = gz.Close()

	f := stubFetcher{
		IndexURL:                                index,
		"https://www.douban.com/sitemap.xml.gz": child.Bytes(),
	}
	children, err := Index(context.Background(), f, IndexURL)
	if err != nil || len(children) != 1 {
		t.Fatalf("Index = %v err=%v", children, err)
	}
	locs, err := Children(context.Background(), f, children[0])
	if err != nil || len(locs) != 1 || locs[0] != "https://book.douban.com/subject/1/" {
		t.Fatalf("Children = %v err=%v", locs, err)
	}
}

func TestSelectBand(t *testing.T) {
	children := make([]string, 300)
	for i := range children {
		children[i] = fmt.Sprintf("c%d", i)
	}
	sub := SelectBand(children, "subject")
	if len(sub) != 88 || sub[0] != "c0" || sub[87] != "c87" {
		t.Fatalf("subject band = %d [%s..]", len(sub), sub[0])
	}
	// A band starting beyond the (truncated) list clamps to empty.
	if got := SelectBand(children, "people"); got != nil {
		t.Fatalf("people band on short list = %v, want nil", got)
	}
	if got := SelectBand(children, "nope"); got != nil {
		t.Fatalf("unknown band = %v, want nil", got)
	}
}
