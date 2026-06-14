---
title: "douban"
description: "Crawl Douban (豆瓣) into structured JSON, or mirror it offline"
heroTitle: "douban, from the command line"
heroLead: "Crawl Douban (豆瓣) into structured JSON, or mirror the whole catalog offline. One pure-Go binary, no API key, output that pipes into the rest of your tools."
heroPrimaryURL: "/getting-started/quick-start/"
heroPrimaryText: "Get started"
---

Crawl Douban (豆瓣) into structured JSON, or mirror it offline

```bash
douban search python              # cross-category search
douban book 1084336               # one book in full
douban top250 --limit 50          # the movie Top 250
douban seed ids --type book --from 1084336 --to 1084340
douban crawl --limit 20           # build a local mirror
```

Seven read-only lookup commands over Douban's public pages (`search`,
`suggest`, `book`, `top250`, `chart`, `nowplaying`, `doulist`), plus a mirror
subsystem that crawls the catalog into a local store you can query offline. The
[CLI reference](/reference/cli/) is the full surface; the [mirror
guide](/guides/mirror/) is the crawl workflow.

## Where to go next

- New here? Read the [introduction](/getting-started/introduction/), then the
  [quick start](/getting-started/quick-start/).
- Installing? See [installation](/getting-started/installation/).
- Need every flag? The [CLI reference](/reference/cli/) is the full surface.
