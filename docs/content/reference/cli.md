---
title: "CLI"
description: "Every command and subcommand, with the flags that matter."
weight: 10
---

```
douban <command> [subcommand] [flags]
```

Run `douban <command> --help` for the full flag list on any command. This
page is the map of the command tree.

## Commands

| Command | What it does |
|---|---|
| `search` | Cross-category search results (books, movies, music, more) |
| `suggest` | Fast autocomplete suggestions for a book or movie query |
| `book` | The full record for one book subject (bare id or pasted URL) |
| `top250` | The movie Top 250 chart |
| `chart` | The weekly new-movies chart |
| `nowplaying` | Movies now in theaters (`--coming` for upcoming) |
| `doulist` | The subjects in a curated list (豆列), in list order |
| `version` | Print the version and exit |

## Shared flags

Every command renders through one engine and shares these persistent flags:

| Flag | Meaning |
|---|---|
| `-o, --output` | `table` (default), `json`, `jsonl`, `csv`, `tsv`, `url` |
| `--fields` | Comma-separated columns to keep |
| `--no-header` | Drop the table/csv header row |
| `--template` | Go text/template applied per record |
| `--limit` | Cap the number of records |
| `--delay` | Pause between paged requests |
| `--timeout` | Per-request timeout |
| `--retries` | Retries on 429/5xx |
| `--user-agent` | Override the request User-Agent |
| `-q, --quiet` | Suppress progress output |

## What serves anonymously

Book subject pages and the search, suggest, chart, now-playing and doulist
surfaces serve fully over anonymous HTTPS. Movie *subject* and *celebrity*
detail pages redirect to a security challenge, so movie detail is reached
through `suggest` and the list commands rather than a per-subject fetch.
