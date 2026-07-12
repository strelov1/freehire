## Context

`internal/sources/tbank.go` crawls T-Bank's `getVacancies` list endpoint with an
empty `filters` map. Live probing (2026-07) shows the `publisher` source treats an
empty `filters.category` as `["tcareer_work_with_clients"]` alone (970 postings),
silently excluding `tcareer_it` (264) and `tcareer_back_office` (457). The adapter's
comment asserting the source "covers all roles" is therefore false. Everything else
in the adapter — offset pagination, per-vacancy detail fan-out, block assembly,
`urlSlug` identity — is correct and stays.

## Goals / Non-Goals

**Goals:**
- Crawl all three top-level categories so IT and back-office vacancies are ingested.
- Keep the change surgical: only the list request's filter and the stale comment.

**Non-Goals:**
- No prod DB surgery (existing client rows stay; their category is still crawled).
- No dynamic category discovery, no per-role config, no new endpoint.

## Decisions

**One filtered request with all categories, not a per-category loop.**
Live verification: passing all three categories in a single `filters.category` array
returns the full union (1691) under normal `publisher.offset` pagination — the server
merges them server-side. So the existing single pagination loop is unchanged; only
the request body gains the filter. Alternative (loop the adapter once per category
and concatenate) was rejected: it triples the request count and re-implements
pagination bookkeeping for no benefit, since the server already unions them.

**Curated constant slice, not runtime discovery.**
The category set is a small, stable, explicit `tbankCategories` constant — the same
"curated dictionary, never guesses" pattern the project uses for `location`/
`skilltag`/`classify`. The API exposes no category-enumeration endpoint (probed:
`getFilters`/`getCategories` → 404; list response carries no facet metadata), and
the three top-level segments are discoverable only from the careers-site config, so
hardcoding the vetted set is both necessary and idiomatic. A future category is a
one-line addition with a documented seam.

## Risks / Trade-offs

- [T-Bank renames or adds a top-level category] → The crawl silently misses it, the
  same failure mode as today but narrower. Mitigation: the constant is documented as
  the seam; a coverage drop surfaces via the ingest count. Acceptable for a curated
  single-source adapter.
- [Non-IT roles remain in the catalogue] → By explicit product decision (crawl all
  categories); the enrich non-tech gate already withholds LLM budget from them, so
  the cost is storage/search noise only, not spend.

## Migration Plan

Pure code change; no schema/config/migration. On merge, the next scheduled
`cmd/ingest sources/custom.yml` run begins upserting IT and back-office vacancies.
Rollback is reverting the one-line filter. Existing rows are untouched either way.
