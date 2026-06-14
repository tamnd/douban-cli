package douban

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// muxClient wires a fresh Client to a test server whose mux serves every Douban
// surface. cfg.BaseURL maps all subdomains to the one server.
func muxClient(t *testing.T) *Client {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/j/subject_suggest", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(suggestJSON))
	})
	mux.HandleFunc("/subject/1084336/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(bookHTML))
	})
	mux.HandleFunc("/subject/404/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/top250", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("start") == "0" {
			_, _ = w.Write([]byte(top250HTML))
			return
		}
		_, _ = w.Write([]byte("<ol class=\"grid_view\"></ol>"))
	})
	mux.HandleFunc("/chart", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(chartHTML))
	})
	mux.HandleFunc("/cinema/nowplaying/beijing/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(nowplayingHTML))
	})
	mux.HandleFunc("/doulist/240962/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("start") == "0" {
			_, _ = w.Write([]byte(doulistHTML))
			return
		}
		_, _ = w.Write([]byte("<div class=\"doulist\"></div>"))
	})

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	cfg := DefaultConfig()
	cfg.BaseURL = ts.URL
	cfg.Rate = 0
	return NewClient(cfg)
}

func TestSuggest(t *testing.T) {
	c := muxClient(t)
	got, err := c.Suggest(context.Background(), "matrix", "movie", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d suggestions, want 1", len(got))
	}
	s := got[0]
	if s.ID != "1291843" || s.Title != "黑客帝国" || s.SubTitle != "The Matrix" || s.Year != "1999" {
		t.Errorf("suggestion = %+v", s)
	}
	if strings.Contains(s.URL, "?") {
		t.Errorf("url not trimmed: %q", s.URL)
	}
}

func TestSuggestUnknownType(t *testing.T) {
	c := muxClient(t)
	if _, err := c.Suggest(context.Background(), "x", "music", 0); err == nil {
		t.Fatal("want error for unsupported suggest type")
	}
}

func TestBook(t *testing.T) {
	c := muxClient(t)
	b, err := c.Book(context.Background(), "https://book.douban.com/subject/1084336/")
	if err != nil {
		t.Fatal(err)
	}
	if b.ID != "1084336" {
		t.Errorf("id = %q", b.ID)
	}
	if b.Title != "小王子" {
		t.Errorf("title = %q", b.Title)
	}
	if b.Rating != "9.1 (799156)" {
		t.Errorf("rating = %q, want 9.1 (799156)", b.Rating)
	}
	if b.Author != "圣埃克苏佩里" {
		t.Errorf("author = %q", b.Author)
	}
	if b.Translator != "马振骋" {
		t.Errorf("translator = %q", b.Translator)
	}
	if b.Publisher != "人民文学出版社" {
		t.Errorf("publisher = %q", b.Publisher)
	}
	if b.ISBN != "9787020042494" {
		t.Errorf("isbn = %q", b.ISBN)
	}
	if b.Pages != "97" || b.Price != "22.00元" || b.Binding != "平装" {
		t.Errorf("info = pages %q price %q binding %q", b.Pages, b.Price, b.Binding)
	}
	if b.Summary == "" {
		t.Error("summary empty")
	}
}

func TestBookNotFound(t *testing.T) {
	c := muxClient(t)
	_, err := c.Book(context.Background(), "404")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestBookInvalidID(t *testing.T) {
	c := muxClient(t)
	if _, err := c.Book(context.Background(), "not/an/id"); err == nil {
		t.Fatal("want error for invalid id")
	}
}

func TestTop250(t *testing.T) {
	c := muxClient(t)
	films, err := c.Top250(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(films) != 1 {
		t.Fatalf("got %d films, want 1", len(films))
	}
	m := films[0]
	if m.Rank != 1 || m.ID != "1292052" || m.Title != "肖申克的救赎" {
		t.Errorf("film = %+v", m)
	}
	if m.AKA != "The Shawshank Redemption / 月黑高飞(港)" {
		t.Errorf("aka = %q", m.AKA)
	}
	if m.Directors != "弗兰克·德拉邦特" {
		t.Errorf("directors = %q", m.Directors)
	}
	if m.Cast != "蒂姆·罗宾斯" {
		t.Errorf("cast = %q", m.Cast)
	}
	if m.Year != "1994" || m.Region != "美国" || m.Genres != "犯罪 剧情" {
		t.Errorf("meta = year %q region %q genres %q", m.Year, m.Region, m.Genres)
	}
	if m.Rating != "9.7 (3295420)" {
		t.Errorf("rating = %q", m.Rating)
	}
	if m.Quote != "希望让人自由。" {
		t.Errorf("quote = %q", m.Quote)
	}
}

func TestChart(t *testing.T) {
	c := muxClient(t)
	films, err := c.Chart(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(films) != 2 {
		t.Fatalf("got %d films, want 2", len(films))
	}
	if films[0].Title != "后室" || films[0].AKA != "后室(电影版)" {
		t.Errorf("rated row = %+v", films[0])
	}
	if films[0].Rating != "8.5 (1234)" {
		t.Errorf("rating = %q", films[0].Rating)
	}
	if films[1].Rating != "-" {
		t.Errorf("unrated rating = %q, want -", films[1].Rating)
	}
}

func TestNowPlaying(t *testing.T) {
	c := muxClient(t)
	now, err := c.NowPlaying(context.Background(), "beijing", false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(now) != 1 || now[0].Title != "火遮眼" {
		t.Fatalf("nowplaying = %+v", now)
	}
	if now[0].Rating != "8.2 (55986)" {
		t.Errorf("rating = %q", now[0].Rating)
	}
	if now[0].Directors != "谷垣健治" || now[0].Duration != "113分钟" {
		t.Errorf("row = %+v", now[0])
	}

	coming, err := c.NowPlaying(context.Background(), "beijing", true, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(coming) != 1 || coming[0].Title != "那山那水" {
		t.Fatalf("coming = %+v", coming)
	}
	if coming[0].Rating != "-" {
		t.Errorf("upcoming rating = %q, want -", coming[0].Rating)
	}
}

func TestDoulist(t *testing.T) {
	c := muxClient(t)
	items, err := c.Doulist(context.Background(), "https://www.douban.com/doulist/240962/", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}
	it := items[0]
	if it.Rank != 1 || it.ID != "1292052" || it.Title != "肖申克的救赎" {
		t.Errorf("item = %+v", it)
	}
	if it.Rating != "9.7 (3295423)" {
		t.Errorf("rating = %q", it.Rating)
	}
	if !strings.Contains(it.Abstract, "导演") {
		t.Errorf("abstract = %q", it.Abstract)
	}
}

func TestDoulistInvalidID(t *testing.T) {
	c := muxClient(t)
	if _, err := c.Doulist(context.Background(), "https://www.douban.com/", 0); err == nil {
		t.Fatal("want error for url without a doulist id")
	}
}

func TestSearchEnriched(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(mockHTML(mockDataJSON)))
	}))
	defer srv.Close()
	c := newTestClient(srv)
	got, err := c.Search(context.Background(), "python", "book", 10)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].ID != "1" || got[0].Type != "book" {
		t.Errorf("first = id %q type %q", got[0].ID, got[0].Type)
	}
	if got[1].Type != "movie" {
		t.Errorf("second type = %q, want movie", got[1].Type)
	}
}

// --- fixtures, shaped after real Douban responses ---

const suggestJSON = `[{"episode":"","img":"https://img.example/p.jpg","title":"黑客帝国","url":"https://movie.douban.com/subject/1291843/?suggest=matrix","type":"movie","year":"1999","sub_title":"The Matrix","id":"1291843"}]`

const bookHTML = `<!DOCTYPE html><html><body>
<h1><span property="v:itemreviewed">小王子</span></h1>
<div id="mainpic"><a class="nbg" href="x"><img src="https://img.example/cover.jpg"></a></div>
<div id="info">
  <span class="pl"> 作者:</span> <a href="x">圣埃克苏佩里</a><br/>
  <span class="pl"> 译者:</span> 马振骋<br/>
  <span class="pl">出版社:</span> 人民文学出版社<br/>
  <span class="pl">出版年:</span> 2003-8<br/>
  <span class="pl">页数:</span> 97<br/>
  <span class="pl">定价:</span> 22.00元<br/>
  <span class="pl">装帧:</span> 平装<br/>
  <span class="pl">ISBN:</span> 9787020042494<br/>
</div>
<strong class="ll rating_num " property="v:average"> 9.1 </strong>
<span property="v:votes">799156</span>
<div class="intro"><p>献给大人的童话。</p></div>
</body></html>`

const top250HTML = `<ol class="grid_view">
<li><div class="item">
<div class="pic"><em class="">1</em><a href="https://movie.douban.com/subject/1292052/"><img width="100" alt="肖申克的救赎" src="https://img.example/p.jpg"></a></div>
<div class="info"><div class="hd"><a href="https://movie.douban.com/subject/1292052/">
<span class="title">肖申克的救赎</span><span class="title">&nbsp;/&nbsp;The Shawshank Redemption</span>
<span class="other">&nbsp;/&nbsp;月黑高飞(港)</span></a></div>
<div class="bd"><p>导演: 弗兰克·德拉邦特&nbsp;&nbsp;&nbsp;主演: 蒂姆·罗宾斯<br>1994&nbsp;/&nbsp;美国&nbsp;/&nbsp;犯罪 剧情</p>
<div><span class="rating_num" property="v:average">9.7</span><span>3295420人评价</span></div>
<p class="quote"><span>希望让人自由。</span></p></div></div>
</div></li></ol>`

const chartHTML = `<table class="ranking-list"><tbody>
<tr class="item"><td width="100"><a class="nbg" href="https://movie.douban.com/subject/36235977/" title="后室"><img src="https://img.example/c.jpg" /></a></td>
<td><div class="pl2"><a href="https://movie.douban.com/subject/36235977/">后室 / <span style="font-size:13px;">后室(电影版)</span></a>
<p>2026 / 切瓦特·埃加福</p>
<div class="star"><span class="rating_nums">8.5</span><span class="pl">(1234人评价)</span></div></div></td></tr>
<tr class="item"><td width="100"><a class="nbg" href="https://movie.douban.com/subject/99999/" title="未上映"><img src="https://img.example/u.jpg" /></a></td>
<td><div class="pl2"><a href="https://movie.douban.com/subject/99999/">未上映</a>
<p>2027 / 某导演</p>
<div class="star"><span class="rating_nums"></span><span class="pl">(暂无评分)</span></div></div></td></tr>
</tbody></table>`

const nowplayingHTML = `<div id="nowplaying"><ul class="lists">
<li id="36877245" class="list-item" data-title="火遮眼" data-score="8.2" data-star="40" data-release="2025" data-duration="113分钟" data-region="中国香港" data-director="谷垣健治" data-actors="谢苗 / 林科灯" data-category="nowplaying" data-votecount="55986" data-subject="36877245"></li>
</ul></div>
<div id="upcoming"><ul class="lists">
<li id="38502416" class="list-item" data-title="那山那水" data-wish="71" data-duration="86分钟" data-region="中国大陆" data-director="陈子隽" data-actors="" data-category="upcoming" data-subject="38502416"></li>
</ul></div>`

const doulistHTML = `<div class="article">
<div class="doulist-item"><div class="mod"><div class="hd"><span class="pos">1</span></div>
<div class="bd doulist-subject"><div class="post"><a href="https://movie.douban.com/subject/1292052/"><img src="https://img.example/d.jpg"></a></div>
<div class="title"><a href="https://movie.douban.com/subject/1292052/">肖申克的救赎</a></div>
<div class="rating"><span class="rating_nums">9.7</span><span>(3295423人评价)</span></div>
<div class="abstract">导演: 弗兰克·德拉邦特 主演: 蒂姆·罗宾斯 类型: 犯罪 / 剧情 年份: 1994</div>
</div></div></div>
</div>`
