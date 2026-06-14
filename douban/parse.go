package douban

import (
	"encoding/json"
	"fmt"
	"regexp"
)

var reData = regexp.MustCompile(`(?s)window\.__DATA__\s*=\s*(.*?);`)

// extractData pulls the window.__DATA__ JSON out of an HTML body and
// unmarshals it into a doubanData struct.
func extractData(body []byte) (*doubanData, error) {
	m := reData.FindSubmatch(body)
	if m == nil {
		return nil, fmt.Errorf("douban: window.__DATA__ not found in page")
	}
	var d doubanData
	if err := json.Unmarshal(m[1], &d); err != nil {
		return nil, fmt.Errorf("douban: unmarshal __DATA__: %w", err)
	}
	return &d, nil
}
