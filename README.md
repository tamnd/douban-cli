# douban

Crawl Douban (豆瓣) books and movies into structured JSON

`douban` is a single pure-Go binary. It reads Douban's public pages over plain
HTTPS, extracts structured records from the embedded JSON and page markup, and
pipes into the rest of your tools. No API key, nothing to run alongside it.

## Install

```bash
go install github.com/tamnd/douban-cli/cmd/douban@latest
```

Or grab a prebuilt binary from the [releases](https://github.com/tamnd/douban-cli/releases), or run
the container image:

```bash
docker run --rm ghcr.io/tamnd/douban:latest --help
```

## Commands

| Command      | What it returns                                             |
|--------------|------------------------------------------------------------|
| `search`     | Cross-category search results (books, movies, music, more) |
| `suggest`    | Fast autocomplete suggestions for a book or movie query    |
| `book`       | The full record for one book subject                       |
| `top250`     | The movie Top 250 chart                                    |
| `chart`      | The weekly new-movies chart                                |
| `nowplaying` | Movies now in theaters (and, with `--coming`, upcoming)    |
| `doulist`    | The subjects in a curated list (豆列), in list order        |

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

## What serves anonymously

Douban gates its surfaces unevenly. Book subject pages and the search,
suggest, chart, now-playing and doulist surfaces serve fully over anonymous
HTTPS. Movie *subject* and *celebrity* detail pages redirect to a security
challenge, so movie detail is reached through `suggest` and the list commands
rather than a per-subject fetch. The commands above only cover surfaces that
serve cleanly.

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
cmd/douban/   thin main, wires cli.Root into fang
cli/          the cobra command tree
douban/       the library: HTTP client and data models
pkg/render/   table/json/jsonl/csv/tsv/url renderer
docs/         documentation site
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
