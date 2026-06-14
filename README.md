# douban

Crawl Douban (豆瓣) into structured JSON, or mirror it offline

`douban` is a single pure-Go binary. It reads Douban's public pages over plain
HTTPS, extracts structured records from the embedded JSON and page markup, and
pipes into the rest of your tools. No API key, nothing to run alongside it.

It works two ways. The seven **lookup** commands answer one question and print
the result. The **mirror** subsystem crawls the catalog into a local store so
you can reconstruct Douban offline: every entity, the raw page bytes, and a
normalized record per subject, all resumable and rate limited.

## Install

```bash
go install github.com/tamnd/douban-cli/cmd/douban@latest
```

Or grab a prebuilt binary from the [releases](https://github.com/tamnd/douban-cli/releases), or run
the container image:

```bash
docker run --rm ghcr.io/tamnd/douban:latest --help
```

## Lookup commands

| Command      | What it returns                                             |
|--------------|------------------------------------------------------------|
| `search`     | Cross-category search results (books, movies, music, more) |
| `suggest`    | Fast autocomplete suggestions for a book or movie query    |
| `book`       | The full record for one book subject                       |
| `top250`     | The movie Top 250 chart                                    |
| `chart`      | The weekly new-movies chart                                |
| `nowplaying` | Movies now in theaters (and, with `--coming`, upcoming)    |
| `doulist`    | The subjects in a curated list (豆列), in list order        |

## Mirror commands

| Command        | What it does                                                  |
|----------------|--------------------------------------------------------------|
| `seed`         | Add URLs to the crawl frontier (sitemap, id range, url, file) |
| `crawl`        | Drain pending frontier URLs into the mirror; resumable        |
| `export`       | Stream normalized records as JSONL                            |
| `info`         | Show the data dir, record/frontier counts, and disk usage     |
| `queue`        | Inspect frontier rows by status and type                      |
| `reset-failed` | Requeue failed rows for another crawl                         |

## Usage

```bash
douban --help
douban version

# Search across categories
douban search python
douban search "流浪地球" --type movie
douban search "周杰伦" --type music --limit 5

# Autocomplete suggestions
douban suggest "三体"
douban suggest "盗梦空间" --type movie

# One book in full (bare id or pasted URL)
douban book 1084336
douban book https://book.douban.com/subject/1084336/

# Movie charts
douban top250 --limit 50
douban chart
douban nowplaying --city beijing
douban nowplaying --coming

# A curated list
douban doulist 240962 --limit 100

# Output formats
douban search python -o json
douban top250 -o jsonl | jq .url
douban book 1084336 -o csv
```

## Mirror

The mirror builds a local copy of the catalog you can query offline. Seed the
frontier, crawl it, then export. The crawl is resumable, so you stop and resume
it freely, and it never silently caps: it logs what it blocks or skips.

```bash
# Seed an id range, then crawl it
douban seed ids --type book --from 1084336 --to 1084340
douban crawl --limit 20

# Seed from the sitemap, banded by entity type
douban seed sitemap --band subject --limit 5000
douban crawl --type book --concurrency 4

# Seed explicit URLs or a file of URLs
douban seed url https://book.douban.com/subject/1084336/
douban seed list urls.txt

# Inspect and export
douban info
douban queue --status failed
douban export --type book -o jsonl | jq .
douban export --out ./export
```

Each crawled URL is stored two ways: the **raw page bytes** gzipped under
`raw/<source>/<type>/<shard>/<id>.<ext>.gz` for faithfulness, and a
**normalized record** in SQLite for query and export. The store lives at
`$HOME/data/douban` by default; override it with `--data` or `DOUBAN_DATA`.

Two sources feed the mirror. The signed Frodo app API serves the
movie/TV/celebrity detail the desktop seals behind a security challenge; raw
HTML serves books, music, games, drama, doulists, groups, and the rest. The
crawler routes each entity to the source that serves it and paces each host
separately. Surfaces behind a JS challenge (notes, some profiles) are recorded
as `blocked` with a reason rather than dropped.

The Frodo key and secret are built in but overridable, so they rotate without a
rebuild:

```bash
douban crawl --frodo-key KEY --frodo-secret SECRET
# or DOUBAN_FRODO_KEY / DOUBAN_FRODO_SECRET in the environment
```

## What serves anonymously

Douban gates its surfaces unevenly. Book subject pages and the search,
suggest, chart, now-playing and doulist surfaces serve fully over anonymous
HTTPS. Movie *subject* and *celebrity* detail pages redirect to a security
challenge, so the lookup commands reach movie detail through `suggest` and the
list commands rather than a per-subject fetch. The mirror reaches the same
sealed surfaces through the signed Frodo API instead.

## Output

Every command renders through the same engine: a table by default, or
`-o json|jsonl|csv|tsv|url`. Select columns with `--fields`, drop the header
with `--no-header`, or supply a Go `--template`.

## Exit codes

| Code | Meaning                          |
|------|----------------------------------|
| 0    | success                          |
| 1    | error (network, parse, HTTP 5xx) |
| 2    | usage error (bad flags/args)     |
| 3    | no results / subject not found   |

## Development

```
cmd/douban/     thin main, wires cli.Root into fang
cli/            the cobra command tree (lookup + mirror commands)
douban/         the library: HTTP client, Frodo client, data models
mirror/         classifier + normalizer
mirror/store/   SQLite frontier + records + raw artifact writer
mirror/sitemap/ sitemap index/child enumeration, banded by entity type
mirror/crawl/   the crawl engine: worker pool, per-host limiter, link expansion
pkg/render/     table/json/jsonl/csv/tsv/url renderer
docs/           documentation site
```

```bash
make build      # ./bin/douban
make test       # go test ./...
make vet        # go vet ./...
```

## Releasing

Push a version tag and GitHub Actions runs GoReleaser, which builds the
archives, Linux packages, the multi-arch GHCR image, checksums, SBOMs, and a
cosign signature:

```bash
git tag v0.2.0
git push --tags
```

The Homebrew and Scoop steps self-disable until their tokens exist, so the first
release works with no extra secrets.

## License

Apache-2.0. See [LICENSE](LICENSE).
