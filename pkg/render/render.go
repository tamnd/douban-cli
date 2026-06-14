// Package render adapts the shared kit renderer to the douban command surface.
// It turns slices of record structs into one of the output formats douban
// supports: table, markdown, json, jsonl, csv, tsv, url, raw, and a Go
// text/template. The colorful rounded-border table and the JSON syntax
// highlighting come from github.com/tamnd/any-cli/kit/render; this file keeps the
// douban-facing Format names and the slice-at-a-time Render call the commands
// already use, so adopting the shared renderer changed no command code.
package render

import (
	"io"
	"reflect"

	kit "github.com/tamnd/any-cli/kit/render"
)

// Format is an output rendering format.
type Format string

const (
	FormatTable    Format = "table"
	FormatMarkdown Format = "markdown"
	FormatJSON     Format = "json"
	FormatJSONL    Format = "jsonl"
	FormatCSV      Format = "csv"
	FormatTSV      Format = "tsv"
	FormatURL      Format = "url"
	FormatRaw      Format = "raw"
)

// Valid reports whether f is one of the supported formats.
func (f Format) Valid() bool {
	switch f {
	case FormatTable, FormatMarkdown, FormatJSON, FormatJSONL, FormatCSV, FormatTSV, FormatURL, FormatRaw:
		return true
	}
	return false
}

// Options configure a Renderer.
type Options struct {
	Format   Format    // the output encoding
	Fields   []string  // projection: restrict and reorder columns by name
	NoHeader bool      // omit the header row in table/markdown/csv/tsv
	Template string    // a Go text/template applied per record
	Color    bool      // emit ANSI color (table accents, JSON highlighting)
	IsTTY    bool      // whether the writer is an interactive terminal
	Width    int       // shrink a too-wide table to this many columns (0 = natural)
	Writer   io.Writer // destination
}

// Renderer writes records in a chosen format. It wraps the kit renderer with the
// slice-at-a-time Render call douban's commands use, emitting each record and
// flushing the buffered grid formats in one call.
type Renderer struct {
	kr *kit.Renderer
}

// New builds a Renderer over o.Writer.
func New(o Options) (*Renderer, error) {
	kr, err := kit.New(kit.Options{
		Format:   kit.Format(o.Format),
		IsTTY:    o.IsTTY,
		Color:    o.Color,
		Fields:   o.Fields,
		NoHeader: o.NoHeader,
		Template: o.Template,
		Width:    o.Width,
		Writer:   o.Writer,
	})
	if err != nil {
		return nil, err
	}
	return &Renderer{kr: kr}, nil
}

// Render writes records (a slice of structs, or a single struct) in the
// configured format and flushes. Each element is emitted by struct reflection
// off its json (and optional table) tags.
func (r *Renderer) Render(records any) error {
	rv := reflect.ValueOf(records)
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return r.kr.Flush()
		}
		rv = rv.Elem()
	}
	if rv.Kind() == reflect.Slice {
		for i := 0; i < rv.Len(); i++ {
			if err := r.kr.Emit(rv.Index(i).Interface()); err != nil {
				return err
			}
		}
	} else if rv.IsValid() {
		if err := r.kr.Emit(rv.Interface()); err != nil {
			return err
		}
	}
	return r.kr.Flush()
}
