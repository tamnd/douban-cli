# douban

Search Douban books, movies, and music (豆瓣)

`douban` is a single pure-Go binary. It reads Douban's public search pages over
plain HTTPS, extracts structured results from the embedded JSON, and pipes into
the rest of your tools. No API key, nothing to run alongside it.

## Install

```bash
go install github.com/tamnd/douban-cli/cmd/douban@latest
```

Or grab a prebuilt binary from the [releases](https://github.com/tamnd/douban-cli/releases), or run
the container image:

```bash
docker run --rm ghcr.io/tamnd/douban:latest --help
```

## Usage

```bash
douban --help
douban version

# Search books (default)
douban search python
douban search "机器学习"

# Search movies
douban search "流浪地球" --type movie

# Search music
douban search "周杰伦" --type music --limit 5

# Output formats
douban search python -o json
douban search python -o jsonl | jq .url
douban search python -o csv
```

## Output columns

| Column   | Description                        |
|----------|------------------------------------|
| rank     | Result rank (1-based)              |
| title    | Book/movie/music title             |
| rating   | Score and review count, e.g. 9.1 (707) |
| abstract | Author, publisher, year, price     |
| url      | Douban subject URL                 |

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
git tag v0.1.0
git push --tags
```

The Homebrew and Scoop steps self-disable until their tokens exist, so the first
release works with no extra secrets.

## License

Apache-2.0. See [LICENSE](LICENSE).
