---
title: "Introduction"
description: "What douban is and how it is put together."
weight: 10
---

Crawl Douban (豆瓣) books and movies into structured JSON

douban is a single binary. It reads Douban's public pages over plain HTTPS,
shapes the responses into clean records, and gets out of your way. There is
nothing to sign up for and nothing to run alongside it.

## How it is built

- A **library package** (`douban-cli`) holds the HTTP client and the typed
  data models. It paces requests, sets an honest User-Agent, and retries the
  transient failures any public site throws under load.
- A **command tree** (`cli`) wraps the library in subcommands with shared
  output formats and flags.
- One **`cmd/douban`** entry point ties them together.

## Scope

douban is a read-only client over data Douban already serves publicly. It
reads that data and shapes it for you. That narrow scope keeps it a single
small binary with no database, no daemon, and no setup.

Douban gates its surfaces unevenly. Book subject pages and the search,
suggest, chart, now-playing and doulist surfaces serve fully over anonymous
HTTPS. Movie subject and celebrity detail pages redirect to a security
challenge, so movie detail is reached through `suggest` and the list commands
rather than a per-subject fetch.

Next: [install it](/getting-started/installation/), then take the
[quick start](/getting-started/quick-start/).
