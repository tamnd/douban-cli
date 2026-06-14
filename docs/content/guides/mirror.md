---
title: "Mirror Douban offline"
description: "Seed, crawl, and export a local copy of the catalog."
weight: 10
---

The mirror builds a local copy of Douban you can query without the network. It
crawls the catalog into a store on disk, keeping both the raw page bytes and a
normalized record per subject, and it resumes cleanly after an interruption.

The workflow is always the same three steps: **seed** the frontier with URLs to
visit, **crawl** them, then **export** what you collected.

## Where it lives

The mirror is a directory. By default it is `$HOME/data/douban`; override it with
`--data` on any mirror command or with the `DOUBAN_DATA` environment variable.

```
data/douban/
  douban.db                 SQLite: the frontier, records, and crawl cursors
  raw/<source>/<type>/<shard>/<id>.<ext>.gz   gzipped raw page bytes
  export/                   JSONL written by `douban export --out`
```

Every URL is stored two ways: the raw bytes are kept verbatim and gzipped so
nothing is lost, and a normalized record is written to SQLite so the catalog is
uniform and queryable. The raw path and a sha256 of the bytes are recorded on
the frontier row.

## Seed

Seeding adds URLs to the frontier. It is idempotent: re-seeding a URL keeps its
status and history. There are four ways to seed.

From the sitemap, optionally banded by entity type so you fetch only the types
you want:

```bash
douban seed sitemap --band subject --limit 5000
douban seed sitemap --band celebrity --band musician
douban seed sitemap --since           # the daily updated feed
```

Known bands: `subject`, `review`, `group-topic`, `event`, `people`, `note`,
`celebrity`, `musician`.

From a contiguous id range, useful for the dense id spaces:

```bash
douban seed ids --type book --from 1084336 --to 1084340
douban seed ids --type celebrity --from 1601851 --to 1601861
```

From explicit URLs or a file:

```bash
douban seed url https://book.douban.com/subject/1084336/
douban seed list urls.txt
```

## Crawl

Crawling drains the pending frontier. For each URL it fetches through the source
that serves it, archives the raw bytes, records the normalized entity, and
enqueues the entity links it discovers. So a crawl expands outward from its
seeds.

```bash
douban crawl --limit 20
douban crawl --type book --source html --concurrency 4
douban crawl --retry-failed
```

The crawl is resumable. An interrupted run leaves its in-flight rows pending, so
the next `crawl` picks up where it left off. `--limit` bounds a single pass;
without it, the crawl drains everything pending.

It never silently caps. URLs it cannot fetch are recorded with an honest status:
`blocked` (a sealed surface or a rejected API key), `skipped` (not found), or
`failed` (a transient error you can retry). Watch progress on stderr, or run
quiet with `-q`.

### Sources and the Frodo key

Two sources feed the mirror. Raw HTML serves books, music, games, drama,
doulists, groups, and most surfaces. The signed Frodo app API serves the
movie, TV, and celebrity detail that the desktop site seals behind a security
challenge. The crawler routes each entity to the right source and paces each
host separately, so the API and the web hosts each keep their own polite delay.

The Frodo key and secret are built in but overridable, so they keep working if
Douban rotates them:

```bash
douban crawl --frodo-key KEY --frodo-secret SECRET
# or set DOUBAN_FRODO_KEY / DOUBAN_FRODO_SECRET
```

## Inspect

`info` reports the data dir, record and frontier counts by type, and disk usage:

```bash
douban info
```

`queue` lists frontier rows, filtered by status and type, so you can see what is
pending or why something blocked:

```bash
douban queue --status failed
douban queue --status blocked --type note -n 50
```

`reset-failed` requeues failed rows for another pass:

```bash
douban reset-failed
douban reset-failed --type book
```

## Export

`export` streams the normalized records as JSONL, one object per line:

```bash
douban export --type book -o jsonl | jq .
douban export --out ./export        # writes ./export/<type>.jsonl
```

Each record carries the entity type and id, the source it came from, the hoisted
common fields (title, year, cover, intro, rating), and the full source data:
the complete Frodo document, or the page's meta, JSON-LD, and `#info` fields.
Nothing the page carried is dropped.

## A note on scale

Douban's full URL space is large, and at a polite crawl delay a complete pass is
not a single afternoon's work. The mirror is built for that reality: it is
banded so you crawl one entity type at a time, resumable so you spread a crawl
across many runs, and rate limited per host so you stay a good citizen. The
runtime is yours to spend. The tool will not pretend a sealed surface worked,
and it will not stop early without telling you.
