## Why

The shipped `aggregator-ats-dedup` pass suppresses an aggregator posting only when its
normalized title EXACTLY equals a first-party ATS posting's. That catches the clean
aggregators (himalayas 4,563 rows, the Danish portals) but misses aggregators that
**mangle the title** during syndication. Measured on prod: gulftalent carries 23,667 open
postings yet only 3,256 are exact-caught; the biggest employers (Marriott ~2,800 Gulf
postings, Apparel Group, Aster) are syndicated from their Oracle/Taleo ATS — which we
already crawl — with the title reformatted:

- an appended location/property suffix: `Assistant Director of Sales - Leisure` (ATS) vs
  `Assistant Director of Sales` (aggregator);
- an undecoded HTML entity: `Assistant F&amp;B Marketing Manager` (aggregator) vs
  `Assistant F&B Marketing Manager` (ATS).

These are the same requisition, but the exact-title key does not match, so the aggregator
copy stays indexed, embedded, and enriched — the wasted-compute / polluted-index problem
`aggregator-ats-dedup` set out to fix, for its highest-volume syndicators.

## What Changes

- Add a second, **better-normalized title key** to the suppression match, applied to both
  the aggregator and the ATS side, as an additional OR match path beside the existing
  exact key (never replacing it):
  - **decode HTML entities** (`&amp;`→`&`, `&#38;`, …) before the alphanumeric collapse,
    so `F&amp;B` and `F&B` normalize identically;
  - **strip a trailing separator suffix** (` - …`, ` | …`, ` — …`) so an ATS title that
    only appends a location/department matches the aggregator's base title.
- An aggregator posting is suppressed when it matches an ATS twin on the exact key **or**
  the normalized key (same company, compatible country as before). All other invariants of
  `aggregator-ats-dedup` are unchanged: aggregator-only suppression, ATS never demoted,
  country gate, idempotent failover, reachable-by-slug, `duplicate_of` reuse.
- Runs in the same reindex-reconcile pass; no schema change, no new extension.

Scope is **deterministic normalization only** (entity-decode + separator-strip). Fuzzy
similarity for reworded/partial titles (`Waiter` vs `Waiter/ Waitress`) — which needs a
trigram index and threshold tuning — is a deliberate, separate follow-up.

## Capabilities

### New Capabilities
- `aggregator-fuzzy-title-dedup`: an additional normalized-title match path in the
  aggregator suppression pass that catches HTML-entity and trailing-separator title
  mangling.

### Modified Capabilities
<!-- none — extends the aggregator-ats-dedup mechanism additively; its requirements are
     unchanged (the exact-key behavior still holds). -->

## Impact

- `internal/db/queries/jobs.sql` — extend `SuppressAggregatorDuplicatesForCompany` with the
  normalized key as an OR match path; regenerate sqlc. (`CompaniesWithAggregatorPostings`
  driver and the `cmd/reindex` wiring are unchanged.)
- No migration, no new column, no Postgres extension; reuses the existing `duplicate_of`
  column and every surface that already filters it.
- **Over-merge risk** rises slightly vs the exact key (a bare aggregator title could match
  the wrong `Base - Suffix` ATS row); bounded by the same safety net (aggregator-only,
  reversible, reachable-by-link, un-suppresses on twin close) and analyzed in design.md.
