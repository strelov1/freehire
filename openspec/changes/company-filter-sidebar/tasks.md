## 1. Schema: derived facet columns on companies

- [x] 1.1 Add an additive migration under `migrations/` adding `regions`,
  `countries`, `domains`, `company_types`, `company_sizes` as `TEXT[] NOT NULL
  DEFAULT '{}'` to `companies` (follow the existing migration numbering/style).
- [x] 1.2 Run `make sqlc` and confirm `db.Company` gains the five `[]string`
  fields; `go build ./...` stays green.

## 2. Recompute: aggregate facet arrays alongside job_count

- [x] 2.1 RED — extend the queue/DB integration test (`internal/db`,
  `//go:build integration`) to seed a company with open + closed jobs carrying
  regions/countries and enrichment (`domains`/`company_type`/`company_size`), run
  the recompute, and assert the five arrays are the distinct union over **open**
  jobs only (closed excluded; unenriched contributes no enrichment facets; all-closed
  empties the arrays).
- [x] 2.2 GREEN — extend `RecountCompanyJobCounts` in
  `internal/db/queries/companies.sql` (rename to reflect its wider job, e.g.
  `RefreshCompanyFacets`) to compute the arrays in the same set-based `UPDATE … FROM`
  pass: `array_agg(DISTINCT … ORDER BY …)` over `unnest(regions|countries)` and over
  `jsonb_array_elements_text(enrichment->'domains')`; `array_agg(DISTINCT
  enrichment->>'company_type'|'company_size')` filtered to non-null/non-empty. Keep
  the `IS DISTINCT FROM` guard across job_count **and** every array. `make sqlc`.
- [x] 2.3 Update `cmd/recount-companies` to call the renamed query; confirm its
  shape is otherwise unchanged. Re-run the integration test — green.

## 3. API: facet-filtered company list

- [x] 3.1 RED — handler integration test (`internal/handler`, `//go:build
  integration`) asserting `GET /api/v1/companies` with `regions`, multiple
  `regions`, `collections`, `company_type`, and a `q`+facet combination filters by
  array overlap (OR within a facet, AND across facets, composed with `q`), and that
  `meta.total` reflects the filtered count.
- [x] 3.2 GREEN — extend `ListCompanies`/`CountCompanies` in
  `internal/db/queries/companies.sql` to take optional array params and filter with
  the `&&` overlap operator (empty param ⇒ no constraint), preserving the existing
  `q` short-circuit and ordering. `make sqlc`.
- [x] 3.3 GREEN — parse the repeatable facet params in
  `internal/handler/companies.go` `ListCompanies` and pass them through; unfiltered
  requests behave exactly as before. Re-run tests — green.

## 4. Frontend: companies filter sidebar

- [x] 4.1 Add a `COMPANY_FACETS` registry (subset of the `FacetDef` shape in
  `web/src/lib/facets.ts`) covering collection/region/country/industry/company
  type/company size, reusing the existing option vocabularies; country as a
  searchable select.
- [x] 4.2 Add `CompanyFiltersPanel.svelte` that iterates `COMPANY_FACETS` and
  renders each via the existing `FacetSection`, with a clear-all action.
- [x] 4.3 Wire a `FilterStore` into `CompaniesView.svelte`: render the sidebar
  next to the list (mobile = toggle-opened panel), subscribe the list reload to the
  debounced `applied` snapshot, and keep the existing `?q` search composed with the
  facets.
- [x] 4.4 Extend `listCompanies` in `web/src/lib/api.ts` to pass facet params, and
  forward them from `/companies/+page.server.ts` for SSR/deep-linking.
- [x] 4.5 Verify the web changes with `svelte-check` (no test runner in `web/`) and
  a manual browse: filter by region/collection, reload a filtered URL, back/forward,
  clear-all.

## 5. Verify & ops

- [x] 5.1 `go build ./... && go vet ./... && go test ./...` and the integration
  tests green.
- [x] 5.2 Document the prod rollout in the change/PR: apply the migration manually
  via psql, then run `cmd/recount-companies` once to backfill the arrays (no
  reindex needed).
