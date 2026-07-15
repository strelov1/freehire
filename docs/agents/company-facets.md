# Company facets conventions

## Scope
The denormalized facet columns on `companies` — job-derived `remote_regions` and curated `yc_*` columns — their maintenance, and the separation between the two.

## Always true
- `companies.remote_regions` (text[], values from `enrich.RegionValues`) is job-derived: the distinct union of `jobs.regions` over the company's open jobs with `work_mode = 'remote'`.
- `remote_regions ⊆ regions` always holds.
- `remote_regions` is maintained in the same set-based `RefreshCompanyFacets` pass as the other denormalized facets (part of the `IS DISTINCT FROM` change-guard) — no separate worker.
- A remote job whose geography did not resolve contributes nothing (same "never guess" bias as `regions`); a company with no open remote job has an empty `remote_regions`.
- The `yc_*` columns (`yc_batch`, `yc_status`, `yc_stage`, `yc_flags`) are curated (importer-owned) and exempt from `RefreshCompanyFacets` — the recompute never references them, guarded by a test.
- `cmd/import-yc` matching is by current-name slug OR any `former_names` slug (first existing wins); the upsert never overwrites `name` on conflict.
- Unmatched YC companies are inserted as reference rows (`is_reference=true`); we hold the full directory (~6k).
- `yc_flags` holds `top_company`/`hiring`; all `yc_*` columns are filterable by overlap on `GET /api/v1/companies`.

## How it works
Company facets fall into two distinct ownership models that must never bleed into each other.

**Job-derived facets (`remote_regions`):** computed by `RefreshCompanyFacets`/`cmd/recount-companies` as the distinct union of `jobs.regions` over the company's open jobs where `work_mode = 'remote'`. This is a remote-scoped sibling of the broader `regions` array, so `remote_regions` is always a subset of `regions`. It stays eventually-consistent with jobs — the recompute runs in the same set-based pass as the other denormalized facets, part of the `IS DISTINCT FROM` change-guard, so a change to a job's regions or work_mode reaches the company on the next recompute. Exposed as a `remote_regions` overlap facet on `GET /api/v1/companies` and a "Remote hiring" pill in the companies FilterModal (`web/src/lib/facets.ts` `COMPANY_FACETS`, reusing the shared `REGION` vocabulary).

**Curated facets (`yc_*`):** populated by `cmd/import-yc` from the yc-oss directory (`yc-oss.github.io/api/companies/all.json`). `internal/ycdir.Map` turns each entry into company-info (one_liner→tagline, long_description→`company_info.description`, `industry`+`industries`+`subindustry` leaf+`tags`→industries, team_size→employee_count, launched_at→year_founded, all_locations→hq_country via `internal/location`) plus the four curated facet columns. `UpsertYCCompany` updates matched companies and inserts the rest as reference rows. Because the `yc_*` columns are importer-owned, `RefreshCompanyFacets` must never touch them — the recompute references only job-derived data, and a test guards this boundary. The company page shows YC badges (top-company/hiring/stage) from `company_info`; logos come from logo.dev (not the yc-oss logo). Re-run `cmd/import-yc` to refresh.

## Limitations
- A dictionary change to `internal/location` affects `hq_country` derivation for YC entries only on the next `cmd/import-yc` run, not automatically.
- `remote_regions` lags behind a job's geography change until the next `RefreshCompanyFacets`/`cmd/recount-companies` recompute.
