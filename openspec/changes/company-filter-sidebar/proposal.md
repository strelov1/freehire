## Why

The `/companies` catalog page is a flat, search-only list — the only way to narrow
it is a name substring. Users who want to browse *kinds* of companies ("YC fintech
startups hiring in Europe") have no way to filter. Jobs already have a rich facet
sidebar; companies should get a parallel one so the catalog is browsable, not just
searchable.

The obstacle is data: a company row today carries only `slug`, `name`,
`collections`, and `job_count`. It has no geography, industry, type, or size of its
own — those facts live on its jobs. So this change is primarily about deriving and
denormalizing those facets onto the company from its open jobs, then filtering on
them.

## What Changes

- Add derived facet arrays to the `companies` table: `regions`, `countries`,
  `domains`, `company_types`, `company_sizes` (all `TEXT[]`), each the distinct
  union of the corresponding value across the company's **open** jobs.
- Extend the periodic company recompute (`cmd/recount-companies`) to refresh these
  arrays in the same set-based pass that maintains `job_count`, so they stay
  eventually consistent with `jobs`.
- Extend `GET /api/v1/companies` to accept repeatable facet query parameters
  (`collections`, `regions`, `countries`, `domains`, `company_type`,
  `company_size`), filtering by array overlap: OR within a facet, AND across
  facets, composable with the existing `q` name search.
- Add a filter sidebar to the `/companies` web page, reusing the jobs filter
  machinery (URL-synced filter state + reusable facet controls), with facets for
  collection, region, country, industry, company type, and company size.

No breaking changes: the new query parameters and columns are additive; an
unfiltered request behaves exactly as before.

## Capabilities

### New Capabilities
<!-- none: this extends existing capabilities -->

### Modified Capabilities
- `companies`: the `companies` table gains derived facet arrays (`regions`,
  `countries`, `domains`, `company_types`, `company_sizes`) maintained by the
  periodic recompute; `GET /api/v1/companies` gains facet-filter query parameters
  with array-overlap semantics.
- `web-frontend`: the companies catalog page gains a filter sidebar (collection /
  region / country / industry / company type / company size), URL-synced like the
  jobs filters.

## Impact

- **Schema**: new additive migration adding five `TEXT[]` columns to `companies`
  (default `'{}'`). Applied on prod manually via psql (per project ops); after
  deploy, one `cmd/recount-companies` run backfills the arrays. No reindex — the
  companies list is plain SQL, not Meilisearch.
- **DB/queries**: `internal/db/queries/companies.sql` — the recompute query is
  extended to aggregate the facet arrays; `ListCompanies`/`CountCompanies` gain
  optional array-overlap filters.
- **Backend**: `internal/handler/companies.go` parses the new facet params;
  `cmd/recount-companies` unchanged in shape (still one query call).
- **Frontend**: `web/src/lib/components/CompaniesView.svelte` gains a sidebar; new
  `COMPANY_FACETS` registry and a `CompanyFiltersPanel`, reusing `FilterStore` and
  `FacetSection`; `web/src/lib/api.ts` `listCompanies` passes facet params;
  `/companies/+page.server.ts` forwards them for SSR.
- **Data quality note**: `regions`/`countries` are dense from ingest; `domains`/
  `company_types`/`company_sizes` are sparse until a job is LLM-enriched, and
  `company_size` is an LLM estimate — acceptable for a browse filter.
