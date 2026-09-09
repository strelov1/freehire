## Why

`companies.tagline`/`company_info` are today only ever filled by two curated,
name/slug-matched external datasets (see `openspec/specs/company-info/spec.md`
and the sibling `company-info-wikipedia-backfill` change, which adds a
third, best-effort name-matched source). But several ATS platforms already
publish a company's own "about us" text as part of the same public API our
adapters crawl for postings — a first-party source with zero name-matching
ambiguity, at zero extra cost beyond a request our crawl is already making to
that host. A quick check of `boards-api.greenhouse.io/v1/boards/{board}`
(distinct from the `/jobs` endpoint adapters already call) confirmed real
employer-authored text is present for a meaningful share of boards (Coinbase:
1078 chars, Figma: 486, Asana: 309 — though many boards leave it blank and
use a fully custom career page instead, so coverage is partial, not
universal). This is strictly higher-confidence than any name-matched
external source and should take priority when both are available, and it
keeps working for every newly-crawled company on a supporting platform
without needing a separate backfill run.

## What Changes

- Extend the `Source` adapter contract with an optional company-level
  description the adapter may populate when its platform exposes one
  separately from posting bodies.
- Implement it for the Greenhouse adapter first (confirmed to expose a
  board-level `content` field via a distinct endpoint from the one used for
  postings) — the only platform in the crawled set confirmed both to have
  such a field and to have it populated often enough to be worth the extra
  request per board.
- Wire the ingest pipeline to write a returned company description into
  `companies.tagline`/`company_info` using the same fill-gap, never-overwrite
  semantics already specified for the two curated backfills — an ingest-time
  writer follows the identical rule, not a new one.
- Leave every other adapter unchanged for now; Workable exposes a
  similarly-shaped `description` field on its account endpoint but it was
  empty in every board checked during the spike, so implementing it is
  deferred until a larger sample shows it's worth the extra request per
  board (tracked as a fast-follow, not blocking this change).

## Capabilities

### New Capabilities
(none)

### Modified Capabilities
- `source-ingest`: the adapter output contract gains an optional
  company-level description an adapter may supply once per board, alongside
  the per-posting fields it already returns.
- `company-info`: adds ingest-time crawling as a third source of
  `tagline`/`company_info`, under the same fill-gap rule as the two curated
  backfills and the Wikipedia backfill.

## Impact

- `internal/ingest/sources/source.go`: the `Job`-adjacent adapter output (or
  a new per-board return value alongside `[]Job` — decided in `design.md`)
  gains an optional company description.
- `internal/ingest/sources/greenhouse.go`: one additional request per board
  (the board-metadata endpoint), fetched once per crawl, not per posting.
- `internal/ingest/pipeline`: the company upsert step gains a fill-gap write
  for `tagline`/`company_info`, reusing the merge semantics already
  implemented for `UpsertYCCompany` (`internal/platform/db/queries/companies.sql`).
- No schema migration: writes into the existing `companies.tagline` /
  `company_info` / `company_info_at` columns.
- No reindex needed: `tagline`/`company_info` are not part of `content_hash`
  or any Meilisearch facet, matching the existing two backfills' behavior.
