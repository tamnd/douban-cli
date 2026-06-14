---
title: "Output formats"
description: "The output contract every command shares: formats, fields, and templates."
weight: 30
---

Every list command in the fleet renders through one formatter, so the same flags
work everywhere. Wire your commands through it as you add them, and this page
describes what users get. Pick a format with `-o`, or let douban choose:
a table when writing to a terminal, JSONL when piped.

## Formats

```bash
douban <command> -o table     # a colorful, rounded-border grid for reading
douban <command> -o markdown  # a GitHub pipe table to paste into docs
douban <command> -o jsonl     # one JSON object per line, for piping
douban <command> -o json      # a single JSON array
douban <command> -o csv       # spreadsheet friendly
douban <command> -o tsv       # tab-separated
douban <command> -o url       # just the URL column
douban <command> -o raw       # the underlying bytes, unformatted
```

| Format | Best for |
|---|---|
| `table` | Reading on a terminal (color, sized to your window) |
| `markdown` | Pasting into a README, issue, or pull request |
| `jsonl` | Piping into another tool, one object at a time |
| `json` | Loading a whole result as an array |
| `csv` / `tsv` | Spreadsheets and quick column math |
| `url` | Feeding URLs into other commands |
| `raw` | The unformatted bytes (response bodies, file contents) |

## Color

On a terminal the table draws a colored header and dimmed borders, and `json`
and `jsonl` are syntax highlighted. Color switches off the moment output is not
a terminal, so a pipe stays plain and parseable. Control it with `--color`:

```bash
douban <command> --color auto    # color a terminal, plain when piped (default)
douban <command> --color always  # force color on, e.g. piping into a pager
douban <command> --color never   # no color, ever
```

The `NO_COLOR` environment variable is honored: when it is set, `auto` stays
plain. A wide table is shrunk to fit your terminal; set `COLUMNS` to override the
width it targets.

## Narrowing columns

Keep only the fields you want:

```bash
douban <command> --fields id,title,url
```

`--no-header` drops the header row in `table` and `csv` output, which helps when
a downstream tool expects bare rows.

## Templating rows

For full control over each line, apply a Go text/template. Fields are the JSON
keys, capitalised:

```bash
douban <command> --template '{{.URL}} {{.Title}}'
```

## Why auto-detection helps

Because the default adapts to the destination, the same command reads well by
hand and parses cleanly in a pipe:

```bash
douban <command>            # a table, because this is a terminal
douban <command> | wc -l    # JSONL, because this is a pipe
```

You only reach for `-o` when you want something other than that default.
