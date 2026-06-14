package douban

import (
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"strings"
)

var (
	reSubject = regexp.MustCompile(`/subject/(\d+)/`)
	reTag     = regexp.MustCompile(`<[^>]+>`)
	reSpace   = regexp.MustCompile(`\s+`)
)

// extractData pulls the window.__DATA__ JSON out of an HTML body and
// unmarshals it into a doubanData struct. The object is isolated by
// brace-matching, not a regex, because Douban embeds HTML entities (whose
// trailing ';' would fool a non-greedy match) inside the JSON strings.
func extractData(body []byte) (*doubanData, error) {
	obj, ok := jsonObject(string(body), "window.__DATA__")
	if !ok {
		return nil, fmt.Errorf("douban: window.__DATA__ not found in page")
	}
	var d doubanData
	if err := json.Unmarshal([]byte(obj), &d); err != nil {
		return nil, fmt.Errorf("douban: unmarshal __DATA__: %w", err)
	}
	return &d, nil
}

// jsonObject returns the first balanced {...} object that appears after marker,
// honoring string and escape state so braces or semicolons inside strings do not
// confuse the scan.
func jsonObject(s, marker string) (string, bool) {
	i := strings.Index(s, marker)
	if i < 0 {
		return "", false
	}
	start := strings.IndexByte(s[i:], '{')
	if start < 0 {
		return "", false
	}
	start += i

	depth, inStr, esc := 0, false, false
	for j := start; j < len(s); j++ {
		c := s[j]
		switch {
		case esc:
			esc = false
		case c == '\\' && inStr:
			esc = true
		case c == '"':
			inStr = !inStr
		case inStr:
			// inside a string: ignore structural characters
		case c == '{':
			depth++
		case c == '}':
			depth--
			if depth == 0 {
				return s[start : j+1], true
			}
		}
	}
	return "", false
}

// subjectID returns the numeric subject id embedded in a Douban URL, or "".
func subjectID(rawURL string) string {
	if m := reSubject.FindStringSubmatch(rawURL); m != nil {
		return m[1]
	}
	return ""
}

// subjectURL strips query and fragment from a subject URL.
func subjectURL(rawURL string) string {
	if i := strings.IndexAny(rawURL, "?#"); i >= 0 {
		return rawURL[:i]
	}
	return rawURL
}

// clean strips HTML tags, decodes entities, and collapses whitespace.
func clean(s string) string {
	s = reTag.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	s = strings.ReplaceAll(s, " ", " ") // nbsp -> space (\s does not match it)
	s = reSpace.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// firstGroup returns the first capture group of re against s, cleaned, or "".
func firstGroup(re *regexp.Regexp, s string) string {
	if m := re.FindStringSubmatch(s); m != nil {
		return clean(m[1])
	}
	return ""
}

// truncRunes shortens s to at most n runes, appending an ellipsis when cut.
func truncRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// fmtRating formats a value/count pair the way every record renders ratings:
// "9.1 (799156)", or "-" when there is no score.
func fmtRating(value float64, count int) string {
	if value <= 0 {
		return "-"
	}
	return fmt.Sprintf("%.1f (%d)", value, count)
}
