package cli

import (
	"github.com/tamnd/douban-cli/pkg/render"
)

// Format aliases so command code reads cleanly.
type Format = render.Format

const (
	FormatTable    = render.FormatTable
	FormatMarkdown = render.FormatMarkdown
	FormatJSON     = render.FormatJSON
	FormatJSONL    = render.FormatJSONL
	FormatCSV      = render.FormatCSV
	FormatTSV      = render.FormatTSV
	FormatURL      = render.FormatURL
	FormatRaw      = render.FormatRaw
)
