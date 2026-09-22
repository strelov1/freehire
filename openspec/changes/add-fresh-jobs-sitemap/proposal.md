# Add a freshest-first job sub-sitemap

## Why

`web-ssr-seo` has required since it was written that the job sub-sitemap list the
freshest open jobs **newest first** (spec.md:172, and the scenario at :191). The
implementation does not do that and never has: `search.Client.ListSitemapPage`
pages the Meilisearch **`/documents`** endpoint, which returns documents in internal
storage order with no sorting available — sorting exists only on `/search`. Every
test passes because they assert the XML is a valid `<urlset>` of open jobs, which it
is. Nothing asserts the order, so a requirement stated in prose has gone unmet
without a single red check.

The cost is measured, not theoretical. Over 2026-09-09..19 the catalogue gained
153,030 jobs that belong in the index (open, canonical, public, `is_tech`).
OAI-SearchBot — the crawler behind ChatGPT's search, which sends more live visitors
to this site than every other source combined — first fetched **27,447 of them,
17.9%**. Its budget is not the constraint: it spends ~54k requests a day, 42k of
them on `/jobs`, against a flow of ~11k eligible new postings a day. It re-reads
what it already has because a 70-chunk sitemap in arbitrary order gives it no way to
tell what is new. Postings that go uncrawled are worth nothing: a job's value to a
searcher expires, and the median time from creation to first crawl is already 29
hours for the postings that do get fetched.

## What Changes

- Add `GET /api/v1/jobs/sitemap/fresh?offset=` — one page of job sitemap entries
  ordered `created_at` descending, drawn through Meilisearch `/search` (where sorting
  exists) under the same `jobSitemapFilter` the existing sub-sitemap uses.
- Add `GET /sitemap-jobs-fresh.xml?offset=` on the SPA, mirroring the existing
  `sitemap-jobs.xml` route.
- List four fresh chunks of 10,000 (40,000 URLs, ~3.5 days of flow) **before** the
  existing job chunks in the sitemap index.
- Bound the fresh offset at 30,000. Past it the response is an empty `<urlset>`, not
  an error — the rule the existing sub-sitemap already follows for a stale index.
- Leave the existing `sitemap-jobs.xml` chunks, their order, their tiling and their
  URLs untouched. They keep providing full catalogue coverage; the fresh file
  provides recency. Neither replaces the other.
- No Meilisearch settings change: `created_at` is already in the jobs index's
  `sortableAttributes`, verified against the live index.

## Capabilities

### New Capabilities

_None._ This restores behaviour an existing capability already requires.

### Modified Capabilities

- `web-ssr-seo`: the "robots.txt and sitemap" requirement currently states that the
  job sub-sitemap lists the freshest open jobs newest first *instead of* the full
  catalogue. Both halves are now out of date — the catalogue IS fully enumerated
  across 70 chunks, and those chunks are NOT ordered. The requirement splits in two:
  the paged job sub-sitemaps enumerate the whole findable catalogue in unspecified
  order, and a separate freshest-first sub-sitemap carries recency with an explicit
  ordering guarantee and a scenario that tests the order rather than the shape.

## Impact

- `internal/search/search/sitemap.go` — a second reader alongside `sitemapPage`,
  using `/search` with a sort instead of `/documents`.
- `internal/api/handler/sitemap.go` — one route, one handler, one offset bound.
- `web/src/routes/sitemap-jobs-fresh.xml/+server.ts` — new route.
- `web/src/lib/sitemap.ts` — the index gains four entries ahead of the job chunks.
- No database work, no migration, no Meilisearch settings patch, no reindex.
- Rollback is removing the four index entries: the new routes then serve nobody, and
  the existing sitemap is byte-identical to today's.
