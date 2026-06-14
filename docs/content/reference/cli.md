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

## Lookup commands

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

## Mirror commands

These build and query a local copy of the catalog. See the
[mirror guide](../../guides/mirror/) for the full workflow.

| Command | What it does |
|---|---|
| `seed sitemap` | Seed the frontier from the sitemap, banded by entity type |
| `seed ids` | Seed a contiguous id range for one entity type |
| `seed url` | Seed one or more explicit URLs |
| `seed list` | Seed URLs from a file, one per line |
| `crawl` | Drain pending frontier URLs into the mirror; resumable |
| `export` | Stream normalized records as JSONL |
| `info` | Show the data dir, record/frontier counts, and disk usage |
| `queue` | Inspect frontier rows by status and type |
| `reset-failed` | Requeue failed rows for another crawl |

### Mirror flags

The mirror commands share three flags on top of the persistent set:

| Flag | Meaning |
|---|---|
| `--data` | Mirror directory (default `$DOUBAN_DATA`, else `$HOME/data/douban`) |
| `--frodo-key` | Override the Frodo API key (or `$DOUBAN_FRODO_KEY`) |
| `--frodo-secret` | Override the Frodo API secret (or `$DOUBAN_FRODO_SECRET`) |

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
detail pages redirect to a security challenge, so the lookup commands reach
movie detail through `suggest` and the list commands rather than a per-subject
fetch. The mirror reaches the same sealed surfaces through the signed Frodo API.
