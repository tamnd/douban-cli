package mirror

import "testing"

func TestClassify(t *testing.T) {
	cases := []struct {
		url string
		typ string
		id  string
		src string
	}{
		{"https://book.douban.com/subject/1084336/", "book", "1084336", SourceHTML},
		{"https://movie.douban.com/subject/1292052/", "movie", "1292052", SourceFrodo},
		{"https://music.douban.com/subject/1407971/", "music", "1407971", SourceHTML},
		{"https://9.douban.com/subject/9031117/", "thing", "9031117", SourceHTML},
		{"https://www.douban.com/subject/12345/", "subject", "12345", SourceHTML},
		{"https://movie.douban.com/review/13721811/", "review", "13721811", SourceFrodo},
		{"https://book.douban.com/review/9001/", "review", "9001", SourceFrodo},
		{"https://www.douban.com/group/topic/170217945/", "group-topic", "170217945", SourceHTML},
		{"https://www.douban.com/group/beijing/", "", "", ""}, // non-numeric group home
		{"https://www.douban.com/group/123/", "group", "123", SourceHTML},
		{"https://www.douban.com/event/36988490/", "event", "36988490", SourceHTML},
		{"https://www.douban.com/people/4828135/", "people", "4828135", SourceHTML},
		{"https://www.douban.com/note/37082541/", "note", "37082541", SourceFrodo},
		{"https://www.douban.com/personage/27260288/", "personage", "27260288", SourceFrodo},
		{"https://www.douban.com/doulist/240962/", "doulist", "240962", SourceHTML},
		{"https://www.douban.com/location/drama/24762809/", "drama", "24762809", SourceHTML},
		{"https://movie.douban.com/celebrity/1601851/", "celebrity", "1601851", SourceFrodo},
		{"https://movie.douban.com/celebrity/1601851/photos/", "celebrity", "1601851", SourceFrodo},
		{"https://music.douban.com/musician/145638/", "musician", "145638", SourceHTML},
		{"https://www.douban.com/photos/album/123/", "photo-album", "123", SourceHTML},
	}
	for _, c := range cases {
		got, ok := Classify(c.url)
		if c.typ == "" {
			if ok {
				t.Errorf("Classify(%q) = %+v, want no match", c.url, got)
			}
			continue
		}
		if !ok {
			t.Errorf("Classify(%q) returned no match", c.url)
			continue
		}
		if got.EntityType != c.typ || got.EntityID != c.id || got.Source != c.src {
			t.Errorf("Classify(%q) = {%s %s %s}, want {%s %s %s}",
				c.url, got.EntityType, got.EntityID, got.Source, c.typ, c.id, c.src)
		}
	}
}

// TestClassifyCanonical checks that an entity's sub-pages all resolve to the
// one canonical URL, so /comments, /reviews and /new_review collapse onto a
// single frontier row instead of each becoming a separate, login-gated fetch.
func TestClassifyCanonical(t *testing.T) {
	cases := map[string]string{
		"https://book.douban.com/subject/1084336/":             "https://book.douban.com/subject/1084336/",
		"https://book.douban.com/subject/1084336/comments":     "https://book.douban.com/subject/1084336/",
		"https://book.douban.com/subject/1084336/new_review":   "https://book.douban.com/subject/1084336/",
		"https://movie.douban.com/subject/1292052/reviews?x=1": "https://movie.douban.com/subject/1292052/",
		"https://movie.douban.com/celebrity/1601851/photos/":   "https://movie.douban.com/celebrity/1601851/",
	}
	for in, want := range cases {
		got, ok := Classify(in)
		if !ok {
			t.Errorf("Classify(%q) no match", in)
			continue
		}
		if got.Canonical != want {
			t.Errorf("Classify(%q).Canonical = %q, want %q", in, got.Canonical, want)
		}
	}
}

func TestClassifyRejectsNonDouban(t *testing.T) {
	for _, u := range []string{
		"https://example.com/subject/1/",
		"https://book.douban.com/tag/小说",
		"https://www.douban.com/location/beijing/",
		"not a url",
		"",
	} {
		if c, ok := Classify(u); ok {
			t.Errorf("Classify(%q) = %+v, want no match", u, c)
		}
	}
}
