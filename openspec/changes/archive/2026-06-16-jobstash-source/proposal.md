## Why

The catalogue has no crypto/Web3 coverage. JobStash (`middleware.jobstash.xyz`)
is a Web3 job aggregator exposing a public, no-auth JSON API of ~3000 open
postings with structured fields (organization, location type, salary, seniority,
tags). It fits the existing `Source` adapter shape, so a single adapter adds a
whole vertical the direct ATS crawls miss (small Web3 shops off the mainstream
ATS platforms).

## What Changes

- **Add a `jobstash` source adapter** that paginates JobStash's
  `GET /jobs/list?page=&limit=` by the response `total` and yields every posting
  as the normalized job shape. The posting's `url` is passed through as-is — for
  a `public` posting it is the real downstream ATS apply link, for a `protected`
  posting it is the JobStash detail page (apply with the real link when present,
  else the best link available). Company comes from each posting's
  `organization.name`, not from configuration. Registered in `sources.All` with
  one entry in `sources/jobstash.yml`.
- **Decouple `boardless` from "single company".** Today the `boardless` marker
  means both "needs no board id" *and* "is one company, so redundant with the
  company filter and excluded from the source facet". JobStash is boardless (one
  API, no per-tenant board) but aggregates many companies, so it must stay a
  selectable value in the source facet. A new opt-in `aggregator` marker keeps
  the source facet listing such a provider; the existing single-company boardless
  adapters stay excluded and untouched. The already-merged `tecla` marketplace
  (PR #130) is the same case — a boardless aggregator currently (wrongly) absent
  from the source facet — so it adopts the new marker too.
- **Regenerate the stale source-facet contract.** The generated `SOURCE_VALUES`
  had drifted: `globalpayments` and `tecla` landed without a contract regen, so
  neither was selectable. Regenerating restores both alongside the new `jobstash`
  (and adds proper UI labels for `JobStash`/`Global Payments`).
- **Dedup overlap is accepted.** JobStash republishes postings that the direct
  greenhouse/lever/ashby crawls already ingest, under a separate `source`
  (`jobstash`) and `external_id`, so the same vacancy may appear twice. This is
  acceptable for the crypto-vertical coverage gain; no cross-source dedup is in
  scope.

## Capabilities

### New Capabilities

<!-- none -->

### Modified Capabilities

- `source-ingest`: a boardless provider may aggregate many companies (not only
  one), and such an aggregator stays a source-facet value; the `jobstash`
  aggregator adapter is registered.

## Impact

- `internal/sources/jobstash.go` (new adapter) + `jobstash_test.go` + a captured
  JSON fixture.
- `internal/sources/source.go`: new `aggregator` marker; `FilterableProviders`
  excludes `boardless && !aggregator`; register `NewJobStash` in `All`.
- `internal/sources/tecla.go`: adopts the `aggregator()` marker (one line) so the
  existing marketplace becomes source-filterable.
- `sources/jobstash.yml` (new board file, one boardless entry).
- `web/src/lib/generated/contracts.ts` regenerated (`SOURCE_VALUES` gains
  `globalpayments`, `jobstash`, `tecla`); `web/src/lib/facets.ts` gains the
  `JobStash`/`Global Payments` label overrides.
- No schema, migration, or API-surface change. New jobs ingest under
  `source = "jobstash"`.
