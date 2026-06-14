package mirror

import (
	"encoding/json"
	"testing"
)

func TestNormalizeFrodo(t *testing.T) {
	body := []byte(`{
		"id":"1292052",
		"title":"肖申克的救赎",
		"year":"1994",
		"intro":"一场关于希望的故事",
		"url":"https://movie.douban.com/subject/1292052/",
		"pic":{"large":"https://img.douban.com/l.jpg","normal":"https://img.douban.com/n.jpg"},
		"rating":{"value":9.7,"count":2800000,"max":10},
		"extra":{"keep":"everything"}
	}`)
	raw, err := NormalizeFrodo("movie", "1292052", "https://movie.douban.com/subject/1292052/", body)
	if err != nil {
		t.Fatalf("NormalizeFrodo: %v", err)
	}
	var n Normalized
	if err := json.Unmarshal(raw, &n); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if n.EntityType != "movie" || n.EntityID != "1292052" || n.Source != SourceFrodo {
		t.Errorf("envelope = %+v", n)
	}
	if n.Title != "肖申克的救赎" || n.Year != "1994" {
		t.Errorf("title/year = %q %q", n.Title, n.Year)
	}
	if n.Cover != "https://img.douban.com/l.jpg" {
		t.Errorf("cover = %q", n.Cover)
	}
	if n.Rating == nil || n.Rating.Value != 9.7 || n.Rating.Count != 2800000 {
		t.Errorf("rating = %+v", n.Rating)
	}
	// The full source survives under Data.
	var data map[string]any
	if err := json.Unmarshal(n.Data, &data); err != nil {
		t.Fatalf("data unmarshal: %v", err)
	}
	if asMap(data["extra"])["keep"] != "everything" {
		t.Errorf("data did not preserve source: %v", data["extra"])
	}
}

func TestNormalizeFrodoNonObject(t *testing.T) {
	if _, err := NormalizeFrodo("movie", "1", "", []byte(`[1,2,3]`)); err == nil {
		t.Fatal("want error for non-object body")
	}
}

func TestNormalizeFrodoYearNumber(t *testing.T) {
	raw, err := NormalizeFrodo("game", "1", "", []byte(`{"title":"X","year":2007}`))
	if err != nil {
		t.Fatalf("NormalizeFrodo: %v", err)
	}
	var n Normalized
	_ = json.Unmarshal(raw, &n)
	if n.Year != "2007" {
		t.Errorf("year = %q, want 2007", n.Year)
	}
}

func TestNormalizeHTML(t *testing.T) {
	page := `<html><head>
		<title>小王子 (豆瓣)</title>
		<meta property="og:title" content="小王子" />
		<meta property="og:image" content="https://img.douban.com/cover.jpg" />
		<meta property="og:description" content="献给大人的童话" />
		<meta name="keywords" content="小王子,圣埃克苏佩里" />
		<script type="application/ld+json">{"@type":"Book","name":"小王子","isbn":"9787020042494"}</script>
		</head><body>
		<div id="info">
			<span class="pl">作者</span> <a href="/author/1/">圣埃克苏佩里</a><br/>
			<span class="pl">出版社:</span> 人民文学出版社<br/>
			<span class="pl">出版年:</span> 2003-8<br/>
		</div>
		<strong property="v:average">9.0</strong>
		<span property="v:votes">123456</span>
		</body></html>`

	raw := NormalizeHTML("book", "1084336", "https://book.douban.com/subject/1084336/", []byte(page))
	var n Normalized
	if err := json.Unmarshal(raw, &n); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if n.Title != "小王子" {
		t.Errorf("title = %q", n.Title)
	}
	if n.Cover != "https://img.douban.com/cover.jpg" {
		t.Errorf("cover = %q", n.Cover)
	}
	if n.Intro != "献给大人的童话" {
		t.Errorf("intro = %q", n.Intro)
	}
	if n.Meta["keywords"] == "" {
		t.Errorf("keywords meta missing: %v", n.Meta)
	}
	if len(n.JSONLD) != 1 {
		t.Fatalf("jsonld = %d blocks, want 1", len(n.JSONLD))
	}
	var ld map[string]any
	_ = json.Unmarshal(n.JSONLD[0], &ld)
	if ld["isbn"] != "9787020042494" {
		t.Errorf("jsonld isbn = %v", ld["isbn"])
	}
	if n.Fields["作者"] != "圣埃克苏佩里" {
		t.Errorf("info 作者 = %q", n.Fields["作者"])
	}
	if n.Fields["出版社"] != "人民文学出版社" {
		t.Errorf("info 出版社 = %q", n.Fields["出版社"])
	}
	if n.Year != "2003-8" {
		t.Errorf("year = %q (from 出版年)", n.Year)
	}
	if n.Rating == nil || n.Rating.Value != 9.0 || n.Rating.Count != 123456 || n.Rating.Max != 10 {
		t.Errorf("rating = %+v", n.Rating)
	}
}

func TestNormalizeHTMLSparse(t *testing.T) {
	// A page with none of the structured carriers still produces a valid record.
	raw := NormalizeHTML("group", "5", "https://www.douban.com/group/topic/5/", []byte(`<html><body>hi</body></html>`))
	var n Normalized
	if err := json.Unmarshal(raw, &n); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if n.EntityType != "group" || n.EntityID != "5" {
		t.Errorf("envelope = %+v", n)
	}
}
