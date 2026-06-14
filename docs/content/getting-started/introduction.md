---
title: "Introduction"
description: "What douban is and how it is put together."
weight: 10
---

Crawl Douban (豆瓣) into structured JSON, or mirror it offline

douban is a single binary. It reads Douban's public pages over plain HTTPS,
shapes the responses into clean records, and gets out of your way. There is
nothing to sign up for and nothing to run alongside it.

It works two ways. The **lookup** commands answer one question and print the
result. The **mirror** subsystem crawls the catalog into a local store so you
can reconstruct Douban offline, keeping the raw page bytes and a normalized
record per subject. See the [mirror guide](/guides/mirror/).

The lookup commands are defined once and exposed over three surfaces: the CLI,
an HTTP API (`douban serve`), and an MCP server (`douban mcp`). A script, a
service, and an AI agent all reach the same records. See the
[CLI reference](/reference/cli/).

## How it is built

- A **library package** (`douban`) holds the HTTP client, the signed Frodo
  app-API client, and the typed data models. It paces requests, sets an honest
  User-Agent, and retries the transient failures any public site throws under
  load.
- A **command tree** (`cli`) declares each lookup once as an operation on the
  [any-cli/kit](https://github.com/tamnd/any-cli) framework, so the CLI, the
  HTTP API, and the MCP server all derive from one registry with shared output
  formats and flags.
- A **mirror subsystem** (`mirror`) adds the crawler: a SQLite-backed frontier
  and record store, sitemap enumeration, a URL classifier, and a resumable
  crawl engine with per-host rate limiting.
- One **`cmd/douban`** entry point ties them together.

## Scope

The lookup commands are a read-only client over data Douban already serves
publicly. The mirror is the same posture at scale: it crawls that public data
into a local copy, keeping the raw bytes verbatim so nothing is lost.

Douban gates its surfaces unevenly. Book subject pages and the search,
suggest, chart, now-playing and doulist surfaces serve fully over anonymous
HTTPS. Movie subject and celebrity detail pages redirect to a security
challenge, so the lookup commands reach movie detail through `suggest` and the
list commands, and the mirror reaches it through the signed Frodo API.

Next: [install it](/getting-started/installation/), then take the
[quick start](/getting-started/quick-start/).
