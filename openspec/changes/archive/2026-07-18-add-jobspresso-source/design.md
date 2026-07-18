## Context

Jobspresso (jobspresso.co) runs on WordPress with the WP Job Manager plugin. Live probing
established:

- `https://jobspresso.co/feed/?post_type=job_listing` returns a well-formed RSS 2.0 document
  with ~20 `<item>` entries (the recent window; `?paged=N` does not page further reliably).
- Each `<item>` carries the posting inline: `<title>` is the role, `<link>` is the canonical
  detail URL, `<pubDate>` is RFC1123Z, `<guid isPermaLink="false">` is a URL carrying the numeric
  post id as its `p=` query param (e.g. `…?post_type=job_listing&p=163363`), `<dc:creator>` is a
  CDATA string of the form `Company<br>⚲&nbsp;Location`, `<description>` is a truncated HTML
  summary, and `<content:encoded>` is the full HTML body.
- The board lists only remote work.

This is the exact weworkremotely shape (`internal/sources/weworkremotely.go`): a boardless
aggregator over one RSS feed, company taken per posting, body inline, remote-only. The only real
differences are where company/location and the id live.

## Goals / Non-Goals

**Goals:**
- A keyless, boardless, aggregator `jobspresso` adapter that maps the RSS feed to `Job`s.
- Reuse the existing `XMLGetter` + `sanitizeHTML` machinery; no new shared helper.

**Non-Goals:**
- A generalized WordPress Job Manager adapter. The `dc:creator`/`⚲` conventions are jobspresso's;
  generalize only when a second live WPJM board appears (noted as a seam, not built now).
- Backlog pagination. The feed is a recent window; incremental crawls keep the catalogue fresh
  and the standard ingest sweep soft-closes anything that ages out.
- A detail fetch or liveness probe. The feed carries the full body inline.

## Decisions

- **`ExternalID` = the `p=` numeric post id from `<guid>`, falling back to the `<link>` slug.**
  The guid's `p=` is the stable native WordPress post id; a `url.Parse(...).Query().Get("p")`
  reads it order-independently. If a future item ever lacks it, the last path segment of `<link>`
  (the post slug) is a stable fallback. An item with neither is dropped. *Alternative rejected:*
  using the slug as the primary id — the numeric post id is shorter and immune to slug edits.

- **Company/location split from `<dc:creator>`.** The CDATA text is `Company<br>⚲&nbsp;Location`.
  Split on the literal `<br>`: the left side is the company, the right side is the location after
  stripping the `⚲` (U+26B2) marker and unescaping `&nbsp;`. A creator with no `<br>` is treated
  as company-only with an empty location. An item whose company resolves empty is dropped (an
  empty company breaks the public slug), mirroring weworkremotely.

- **Body from `<content:encoded>`, falling back to `<description>`.** `content:encoded` is the
  full posting; `description` is a truncated summary. Decode both by their local element names
  (`encoded`, `description`) — Go's `encoding/xml` matches the local name regardless of the
  `content:` / prefix — and sanitize with `sanitizeHTML(html.UnescapeString(...))` as
  weworkremotely does. Prefer `content:encoded` when non-empty.

- **Remote-only.** Jobspresso lists only remote roles, so every mapped `Job` gets `Remote: true`
  and work mode `remote`, exactly as weworkremotely hard-codes it.

- **Boardless aggregator, keyless.** Add the `boardless()` and `aggregator()` markers; the config
  entry is a single boardless line with `company:` present only as a label. One line in
  `sources.All`. No proxy — a direct datacenter fetch of the public RSS works.

## Risks / Trade-offs

- **Recent-window only (~20 items/crawl).** → Accepted; the same as weworkremotely and the other
  RSS aggregators. Frequent crawls keep coverage current; aged-out postings are soft-closed by the
  standard sweep.
- **`dc:creator` format could drift** (a board could stop emitting the `⚲` marker). → The split is
  defensive: no `<br>` yields company-only, a missing `⚲` just leaves the raw remainder as
  location; neither aborts the crawl. Verified against a saved real fixture in the RED test.

## Migration Plan

- No DB migration, no API change, no new dependency.
- Deploy: add a `cmd/ingest sources/jobspresso.yml` cron schedule in freehire-ops.

## Open Questions

- None. The feed is keyless, public, and fetched directly; the mapping is pinned to a real
  fixture.
