# internal/job/ycdir — YC Directory Enrichment

Maps yc-oss directory entries to company-info fields, consumed by `cmd/import-yc`.

## Enrichment Map

`internal/job/ycdir.Map` turns each yc-oss entry into:
- `one_liner` → tagline
- `long_description` → `company_info.description`
- `industry`+`industries`+`subindustry` leaf+`tags` → industries
- `team_size` → employee_count
- `launched_at` → year_founded
- `all_locations` → hq_country via `internal/dict/location`
- Four curated facet columns: `yc_batch`/`yc_status`/`yc_stage`/`yc_flags` (text[], filterable by overlap on `GET /api/v1/companies` + FilterModal; `yc_flags` holds `top_company`/`hiring`)

## Upsert Logic

- `UpsertYCCompany` updates matched companies and inserts unmatched as reference rows (`is_reference=true`), holding the full YC directory (~6k).
- **Matching by current-name slug OR any `former_names` slug** (first existing wins) — renamed companies enriched in place, not duplicated. Upsert never overwrites `name` on conflict.
- **Both slugs are `normalize.CompanySlug`**, the key the catalogue stores companies under — never `normalize.Slug`. A miss here is silent by construction: an unmatched entry is not an error, it becomes a reference row and `loadStats.inserted` counts it, so a directory spelling the catalogue can never hold ("Stripe, Inc.") reads as a company we do not have. It was `normalize.Slug` until 2026-09-06, which cost 76 current and 369 former names on the live directory. `TestMappedSlugsAreCompanySlugStable` pins every produced slug as a fixed point of the rule.
- The `yc_*` columns are **curated (importer-owned) and exempt from `RefreshCompanyFacets`** — recompute never references them, guarded by a test.

## Company Page

Shows YC badges (top-company/hiring/stage) from `company_info`; logos from logo.dev (not yc-oss).

## Convention

- Re-run `cmd/import-yc` to refresh the directory. The map logic is pure — no state, no LLM.
