## Why

Remote, location-agnostic vacancies are under-discoverable in the filters. Two
gaps: (1) common worldwide markers like `International` are not recognized, so
those roles fall out of the `global` region facet; (2) a bare `Remote` with no
resolvable country/region deliberately gets no geography, so there is no way to
filter for "remote, region not specified" — these jobs are invisible to anyone
browsing by region. Closing both gaps surfaces more remote roles with purely
deterministic dictionary code (no LLM), applied to existing rows via the existing
re-derive + reindex path.

## What Changes

- Map the worldwide marker `International` (and close synonyms) to the existing
  `global` region in the location dictionary.
- Add a new deterministic facet `remote_unspecified`: true when a job is
  `work_mode=remote` AND has no resolved countries AND no resolved regions.
  Stored as a `jobs` column, derived at ingest by `jobderive`, served dict-only,
  indexed as a Meilisearch filterable attribute, filterable via a new
  `remote_unspecified` search param, and shown as a filter pill in the SPA Region
  group (distinct from `Global`).
- Re-derive existing jobs (`cmd/backfill-derive`) and rebuild the search index so
  the new mapping, the new facet, and the already-implemented `Contractor`
  employment-type matcher reach the existing catalogue. **Operational ordering:**
  the new filterable attribute must be added and a full reindex completed before
  the API/UI exposes the filter, or `/jobs` returns 500 on the unknown attribute.

Note: `Contractor` requires no code — `jobfacts.EmploymentType` already resolves
`contractor`/`freelance`/etc. to `employment_type=contract`, with an existing
search filter and UI pill. It is covered by the backfill only.

## Capabilities

### New Capabilities
- `remote-region-filters`: the `remote_unspecified` derived facet — its
  derivation rule, deterministic storage, dict-only serving, search filterability,
  and SPA filter surface.

### Modified Capabilities
- `job-geography`: the location parser additionally resolves `International` (and
  close worldwide synonyms) to the `global` region.

## Impact

- Code: `internal/location/dictionaries.go` (International→global),
  `migrations/` (new `remote_unspecified` column + a new filterable attribute),
  `internal/db/queries/*.sql` + `make sqlc` (write the column on upsert/derive),
  `internal/jobderive` (compute the facet), `internal/jobview` (serve it),
  `internal/search` (filterable attribute + `query_filter` param),
  `web/src/lib/facets.ts` (filter pill).
- Ops: `cmd/backfill-derive` then `make reindex` (reindex BEFORE exposing the new
  filterable attribute to avoid `/jobs` 500s).
- No LLM, no new dependencies.
