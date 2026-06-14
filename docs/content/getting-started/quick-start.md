---
title: "Quick start"
description: "Run your first douban command."
weight: 30
---

Once `douban` is on your `PATH`:

```bash
douban --help       # see the command tree
douban version      # build info
```

Search across categories, then drill in:

```bash
# Search books, movies, music
douban search python
douban search "流浪地球" --type movie

# Fast autocomplete
douban suggest "三体"

# One book in full
douban book 1084336

# Movie charts
douban top250 --limit 50
douban chart
douban nowplaying --city beijing

# A curated list (豆列)
douban doulist 240962
```

Every command prints a table by default and pipes straight into `jq` with
`-o json` or `-o jsonl`:

```bash
douban top250 -o jsonl | jq .url
```

Or build a local mirror you can query offline. Seed the frontier, crawl it,
then export:

```bash
douban seed ids --type book --from 1084336 --to 1084340
douban crawl --limit 20
douban info
douban export --type book -o jsonl | jq .
```

The crawl is resumable, so you can stop and resume it freely. See the
[mirror guide](../../guides/mirror/) for the full workflow.
