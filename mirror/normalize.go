package mirror

import (
	"encoding/json"
	"html"
	"regexp"
	"strconv"
	"strings"
)

// Normalized is the uniform record every entity maps to, whatever its source.
// It hoists the fields worth querying to the top level and keeps the complete
// source alongside them so nothing is lost: Frodo documents land in Data
// verbatim, and HTML structured-data blobs land in Meta, JSONLD, and Fields.
type Normalized struct {
	EntityType string            `json:"entity_type"`
	EntityID   string            `json:"entity_id"`
	Source     string            `json:"source"`
	URL        string            `json:"url,omitempty"`
	Title      string            `json:"title,omitempty"`
	Year       string            `json:"year,omitempty"`
	Cover      string            `json:"cover,omitempty"`
	Intro      string            `json:"intro,omitempty"`
	Rating     *Rating           `json:"rating,omitempty"`
	Fields     map[string]string `json:"fields,omitempty"`
	Meta       map[string]string `json:"meta,omitempty"`
	JSONLD     []json.RawMessage `json:"jsonld,omitempty"`
	Data       json.RawMessage   `json:"data,omitempty"`
}

// Rating is the canonical score shape shared by Frodo and HTML.
type Rating struct {
	Value float64 `json:"value,omitempty"`
	Count int     `json:"count,omitempty"`
	Max   float64 `json:"max,omitempty"`
}

// NormalizeFrodo maps a Frodo JSON document to the uniform record, hoisting the
// common fields and keeping the full object under Data. It returns an error only
// when the body is not a JSON object, so the caller can fall back to raw bytes.
func NormalizeFrodo(entityType, id, url string, body []byte) (json.RawMessage, error) {
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, err
	}
	n := Normalized{
		EntityType: entityType,
		EntityID:   id,
		Source:     SourceFrodo,
		URL:        firstStr(url, str(m["url"]), str(m["alt"]), str(m["sharing_url"])),
		Title:      firstStr(str(m["title"]), str(m["name"])),
		Year:       numOrStr(m["year"]),
		Cover:      frodoCover(m),
		Intro:      firstStr(str(m["intro"]), str(m["card_subtitle"]), str(m["abstract"])),
		Rating:     frodoRating(m),
		Data:       json.RawMessage(body),
	}
	return json.Marshal(n)
}

// NormalizeHTML maps an HTML page to the uniform record on a best-effort basis,
// capturing the title, Open Graph and named meta, every JSON-LD block, the
// microdata rating, and the subject #info label/value pairs. It never errors;
// the raw artifact remains the lossless copy.
func NormalizeHTML(entityType, id, url string, body []byte) json.RawMessage {
	s := string(body)
	n := Normalized{
		EntityType: entityType,
		EntityID:   id,
		Source:     SourceHTML,
		URL:        url,
		Meta:       extractMeta(s),
		JSONLD:     extractJSONLD(s),
		Fields:     extractInfo(s),
		Rating:     htmlRating(s),
	}
	n.Title = firstStr(n.Meta["og:title"], extractTitle(s))
	n.Cover = n.Meta["og:image"]
	n.Intro = firstStr(n.Meta["og:description"], n.Meta["description"])
	if v, ok := n.Fields["出版年"]; ok {
		n.Year = v
	}
	out, _ := json.Marshal(n)
	return out
}

// --- Frodo helpers ---

func frodoRating(m map[string]any) *Rating {
	r := asMap(m["rating"])
	if r == nil {
		return nil
	}
	out := &Rating{
		Value: num(r["value"]),
		Count: int(num(r["count"])),
		Max:   num(r["max"]),
	}
	if out.Value == 0 && out.Count == 0 {
		return nil
	}
	return out
}

func frodoCover(m map[string]any) string {
	if p := asMap(m["pic"]); p != nil {
		if c := firstStr(str(p["large"]), str(p["normal"]), str(p["small"])); c != "" {
			return c
		}
	}
	if c := asMap(m["cover"]); c != nil {
		if v := firstStr(str(c["url"]), str(c["image"])); v != "" {
			return v
		}
	}
	if img := asMap(m["image"]); img != nil {
		if v := firstStr(str(img["large"]), str(img["normal"])); v != "" {
			return v
		}
	}
	return firstStr(str(m["cover_url"]), str(m["pic_url"]))
}

// --- HTML helpers ---

var (
	titleRE   = regexp.MustCompile(`(?is)<title>(.*?)</title>`)
	metaTagRE = regexp.MustCompile(`(?is)<meta\b[^>]*>`)
	attrRE    = regexp.MustCompile(`(?is)([\w:-]+)\s*=\s*"([^"]*)"`)
	ldRE      = regexp.MustCompile(`(?is)<script[^>]*type="application/ld\+json"[^>]*>(.*?)</script>`)
	avgRE     = regexp.MustCompile(`(?is)property="v:average"[^>]*>\s*([\d.]+)`)
	votesRE   = regexp.MustCompile(`(?is)property="v:votes"[^>]*>\s*(\d+)`)
	bestRE    = regexp.MustCompile(`(?is)content="([\d.]+)"\s+property="v:best"`)
	tagRE     = regexp.MustCompile(`<[^>]+>`)
	spaceRE   = regexp.MustCompile(`\s+`)
)

func extractTitle(s string) string {
	t := firstGroup(titleRE, s)
	// Pages end the title with the site name; drop it.
	for _, suf := range []string{" (豆瓣)", "(豆瓣)", "_豆瓣"} {
		t = strings.TrimSuffix(t, suf)
	}
	return strings.TrimSpace(t)
}

func extractMeta(s string) map[string]string {
	out := map[string]string{}
	for _, tag := range metaTagRE.FindAllString(s, -1) {
		attrs := map[string]string{}
		for _, a := range attrRE.FindAllStringSubmatch(tag, -1) {
			attrs[strings.ToLower(a[1])] = a[2]
		}
		key := attrs["property"]
		if key == "" {
			key = attrs["name"]
		}
		content, ok := attrs["content"]
		if key == "" || !ok {
			continue
		}
		if strings.HasPrefix(key, "og:") || key == "description" || key == "keywords" {
			out[key] = htmlUnescape(content)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func extractJSONLD(s string) []json.RawMessage {
	var out []json.RawMessage
	for _, m := range ldRE.FindAllStringSubmatch(s, -1) {
		blob := strings.TrimSpace(m[1])
		if json.Valid([]byte(blob)) {
			out = append(out, json.RawMessage(blob))
		}
	}
	return out
}

// extractInfo parses the subject #info block: a run of
// <span class="pl">label</span> value pairs separated by <br>.
func extractInfo(s string) map[string]string {
	parts := strings.Split(s, `<span class="pl">`)
	if len(parts) < 2 {
		return nil
	}
	out := map[string]string{}
	for _, p := range parts[1:] {
		end := strings.Index(p, "</span>")
		if end < 0 {
			continue
		}
		label := strings.Trim(stripHTML(p[:end]), " :：")
		rest := p[end+len("</span>"):]
		if i := strings.Index(rest, "<br"); i >= 0 {
			rest = rest[:i]
		}
		val := stripHTML(rest)
		if label != "" && val != "" {
			out[label] = val
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func htmlRating(s string) *Rating {
	avg := firstGroup(avgRE, s)
	if avg == "" {
		return nil
	}
	r := &Rating{Value: atof(avg), Count: atoi(firstGroup(votesRE, s)), Max: atof(firstGroup(bestRE, s))}
	if r.Max == 0 {
		r.Max = 10
	}
	return r
}

// --- small shared helpers ---

func stripHTML(s string) string {
	s = tagRE.ReplaceAllString(s, " ")
	s = htmlUnescape(s)
	s = strings.ReplaceAll(s, " ", " ")
	s = spaceRE.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

func htmlUnescape(s string) string { return html.UnescapeString(s) }

func firstGroup(re *regexp.Regexp, s string) string {
	if m := re.FindStringSubmatch(s); m != nil {
		return stripHTML(m[1])
	}
	return ""
}

func firstStr(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func num(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case json.Number:
		f, _ := n.Float64()
		return f
	default:
		return 0
	}
}

func numOrStr(v any) string {
	switch n := v.(type) {
	case string:
		return n
	case float64:
		if n == float64(int64(n)) {
			return strconv.FormatInt(int64(n), 10)
		}
		return strconv.FormatFloat(n, 'f', -1, 64)
	default:
		return ""
	}
}

func atof(s string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return f
}

func atoi(s string) int {
	i, _ := strconv.Atoi(strings.TrimSpace(s))
	return i
}
