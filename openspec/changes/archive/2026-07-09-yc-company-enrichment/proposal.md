## Why

We already fetch the yc-oss directory (`yc-oss.github.io/api/companies/all.json`,
6,024 YC companies) but read only each entry's `name` — to tag `collections=yc`.
The same payload carries a short description, a full description, industry/tags,
team size, founding year, website, HQ location, YC batch, and status. That data
enriches our company pages and unlocks YC-specific filters, at no new source cost.
The company-info storage already exists (`UpsertCompanyInfo` writes the company-info
columns + `company_info` JSONB, and inserts reference rows for companies we don't
ingest), so this is mostly a mapping + a couple of curated facets.

## What Changes

- Add a **`cmd/import-yc`** run-once worker: fetch yc-oss, map each entry, and
  upsert company-info by `normalize.Slug(name)` — updating existing companies and
  **inserting the rest as reference rows** (`is_reference = true`), so we hold the
  full YC directory (~6k companies, thousands of them job-less cards).
- **Map yc-oss → existing company-info**: `one_liner → tagline`,
  `long_description → company_info.description`, `industry`+`tags → industries`,
  `team_size → employee_count`, `launched_at → year_founded`,
  `all_locations → hq_country` (via `internal/location`), plus `website`, `stage`,
  `top_company`, `isHiring`, YC url, logo into `company_info`.
- Add two **curated filterable facets**: `companies.yc_batch text[]` (e.g.
  `Winter 2012`) and `companies.yc_status text[]` (`Active`/`Acquired`/`Public`/
  `Inactive`), set by the importer and **not touched by the facet recompute**.
  Expose them as `yc_batch`/`yc_status` overlap facets on `GET /api/v1/companies`
  and as filters in the companies FilterModal (batch = searchable select, status =
  pills).

## Capabilities

### New Capabilities

- `yc-company-enrichment`: the yc-oss field mapping, the `cmd/import-yc` directory
  importer (existing-update + reference-insert), and the curated `yc_batch`/
  `yc_status` facet columns (importer-owned, recompute-exempt).

### Modified Capabilities

- `companies`: the list endpoint gains `yc_batch` and `yc_status` facet query
  parameters.

## Impact

- **Schema**: migration adding `companies.yc_batch text[]` + `yc_status text[]`.
- **DB access**: new `UpsertYCCompany` query (company-info columns + yc facets +
  reference-insert); `ListCompanies`/`CountCompanies` gain two facet params.
- **New code**: `cmd/import-yc`, yc-oss mapping (extend `internal/collections` or a
  new `internal/ycdir`), reuse of `internal/location` for HQ.
- **Web**: two new company filters.
- **Data/prod**: importer inserts up to ~6k YC companies as reference rows; the
  recompute leaves the new curated columns alone. Run `cmd/import-yc` once, then a
  `make reindex` is not needed (companies list is Postgres-served).
- **Seam**: matching is slug-only (name-normalized), same as `import-collections`.
