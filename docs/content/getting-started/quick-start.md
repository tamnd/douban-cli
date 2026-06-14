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
