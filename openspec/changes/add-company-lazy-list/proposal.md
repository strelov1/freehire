## Why

The sidebar "Company" filter is backed by a Meilisearch facet distribution,
which Meili caps at `maxValuesPerFacet=300` and returns in **alphabetical**
order, not by popularity. Over a ~1.54M-job catalogue the list is therefore
dominated by junk `0-`/`1-`-prefixed company slugs and excludes popular
employers entirely — typing "google" finds nothing. The filter is effectively
unusable. The fix is to source the company list from the database, sorted by job
count, and to make that sort cheap with a denormalized counter.

## What Changes

- Add a denormalized `companies.job_count` column (open jobs only), maintained by
  a new periodic recompute worker `cmd/recount-companies` (cron, hourly).
- `GET /api/v1/companies` reads `job_count` from the column instead of a
  query-time `count()` join, and orders by `job_count DESC, name` so the most
  active companies surface first. The `q` name-search and pagination are
  unchanged. **BREAKING** (internal): the list is no longer name-ordered.
- Replace the sidebar Company filter's broken dynamic Meili facet with a lazy,
  server-backed typeahead that queries the (now count-sorted) companies endpoint,
  shows real company names and global open-job counts, and on an empty query
  shows the most popular companies. The filter still applies via the existing
  `company_slug` search param, so URL state, exclusion, and chips are unchanged.
- The `/companies` page needs no frontend change — it already renders the job
  count and search; it simply inherits the count-sorted ordering.
- Cheap win: stop requesting the unused `company_slug` facet distribution in the
  analytics facets endpoint, while keeping `company_slug` filterable.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `companies`: the company list gains a denormalized `job_count` maintained by a
  periodic recompute, and is ordered by job count descending; the prior
  requirement that no denormalized counter is required is superseded.
- `web-frontend`: the Company filter facet becomes a lazy, server-backed
  typeahead sourced from the companies endpoint (sorted by job count), replacing
  the capped Meilisearch facet distribution.

## Impact

- **Schema**: migration `0025_companies_job_count.sql` (new column + index).
  Applied manually on prod (initdb runs only on first volume init).
- **Backend**: `internal/db/queries/companies.sql` (ListCompanies, new
  RecountCompanyJobCounts), regenerated `internal/db`; new `cmd/recount-companies`
  worker; `internal/handler/facets.go` (drop company_slug from requested facets).
- **Build/ops**: root `Dockerfile` (new binary build + COPY); a new Go image
  (full rebuild, not sources-only); `freehire-ops/docker-compose.prod.yml` worker
  service + hourly host cron line; one manual `recount-companies` run post-deploy
  to populate the column.
- **Frontend**: new `web/src/lib/components/facets/RemoteSearchSelect.svelte`, a
  `control: 'remote'` facet type, and the `company_slug` FACETS entry switched to
  it; no `/companies` page change.
- **No reindex** required (this does not change the Meili document shape).
