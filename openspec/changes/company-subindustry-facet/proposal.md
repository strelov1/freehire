## Why

The company "Industry" filter is today the job-derived `domains` facet (14 coarse buckets from
`enrichment.domains`), which throws away the rich YC industry taxonomy carried in
`companies.industries`. Users cannot filter companies by their actual industry vertical
(e.g. "Payments", "Diagnostics"). We deliberately do not trust the flattened `industries`
array for classification — it mixes YC's clean top-level industry and subindustry with an
unbounded, noisy tag cloud. But the **structured subindustry** (`e.subindustry` leaf, ~100
bounded YC-defined values) is clean and trustworthy, and is the natural rich industry axis.

## What Changes

- Store each YC company's clean **subindustry** (the leaf of YC's `subindustry` path) in a new
  scalar `companies.subindustry` column, sourced from `cmd/import-yc`. The existing
  `companies.industries` bag (including noisy tags) is untouched and keeps serving the company
  display chips.
- Add a **scalar** company-list facet `subindustries` that filters `companies.subindustry` by
  membership (`= ANY`), mirroring the existing `maturity` facet.
- Serve the facet's option vocabulary from a new dynamic endpoint
  `GET /api/v1/companies/subindustries` returning the distinct subindustries with company
  counts (a searchable ~100-item list, self-maintaining).
- Frontend: a new searchable "Industry" facet driven by that endpoint; the existing
  `domains` facet is relabelled from "Industry" to "Domain" so the two distinct axes
  (job-derived domain vs YC industry) don't collide.

Coverage is YC companies only — a company with no YC subindustry (`NULL`) is not matched by the
facet (never guessed).

## Capabilities

### New Capabilities

<!-- none -->

### Modified Capabilities

- `companies`: adds a clean scalar `subindustry` classification column (distinct from the noisy
  `industries` display bag), a `subindustries` membership facet on the company list, and a
  dynamic subindustry facet-vocabulary endpoint.

## Impact

- Migration: new `companies.subindustry` column (manual apply, per the migrations convention).
- `internal/ycdir` (new `Record.Subindustry`), `internal/db/queries/companies.sql`
  (`UpsertYCCompany` write, `ListCompanies`/`CountCompanies` filter, new facet-values query) +
  regenerated `internal/db/*.go`, `internal/handler/companies.go` (param + new endpoint).
- Frontend `web/src/lib/facets.ts` (new facet + `domains` relabel) and the filter modal's
  option loading.
- Ops: apply migration, then re-run `cmd/import-yc` to populate. No reindex.
