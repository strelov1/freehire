# internal/ycdir — YC Directory Enrichment

Maps yc-oss directory entries to company-info fields, consumed by `cmd/import-yc`.

## Enrichment Map

`internal/ycdir.Map` turns each yc-oss entry into:
- `one_liner` → tagline
- `long_description` → `company_info.description`
- `industry`+`industries`+`subindustry` leaf+`tags` → industries
- `team_size` → employee_count
- `launched_at` → year_founded
- `all_locations` → hq_country via `internal/location`
- Four curated facet columns: `yc_batch`/`yc_status`/`yc_stage`/`yc_flags` (text[], filterable by overlap on `GET /api/v1/companies` + FilterModal; `yc_flags` holds `top_company`/`hiring`)

## Upsert Logic

- `UpsertYCCompany` updates matched companies and inserts unmatched as reference rows (`is_reference=true`), holding the full YC directory (~6k).
- **Matching by current-name slug OR any `former_names` slug** (first existing wins) — renamed companies enriched in place, not duplicated. Upsert never overwrites `name` on conflict.
- The `yc_*` columns are **curated (importer-owned) and exempt from `RefreshCompanyFacets`** — recompute never references them, guarded by a test.

## Company Page

Shows YC badges (top-company/hiring/stage) from `company_info`; logos from logo.dev (not yc-oss).

## Convention

- Re-run `cmd/import-yc` to refresh the directory. The map logic is pure — no state, no LLM.
