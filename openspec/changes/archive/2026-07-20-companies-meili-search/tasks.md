## 1. Company document & mapper

- [x] 1.1 Add `CompanyDocument` type and `FromCompany(db.Company) CompanyDocument` in a new `internal/search/company.go`, mapping `slug` (primary key), `name`, `tagline`, `job_count`, the facet arrays (`collections`, `regions`, `countries`, `domains`, `company_types`, `company_sizes`, `remote_regions`, `yc_batch`, `yc_status`, `yc_stage`, `yc_flags`) and scalars (`maturity`, `subindustry`)
- [x] 1.2 Unit-test `FromCompany`: full mapping, empty/`nil` arrays serialize to empty, `NULL` `tagline`/`maturity`/`subindustry` map to zero-values, `job_count` carried through

## 2. Meili companies index (parallel to jobs, no jobs-code edits)

- [x] 2.1 Add `companyIndexUID = "companies"` / `companyRebuildUID = "companies_rebuild"` constants and bind a `companies` index manager on `*Client` without touching the `facet`/`semantic` jobs fields
- [x] 2.2 Add `companySettings()`: searchable `[name, slug, tagline]`, filterable = the 14 facet attrs, sortable `[job_count]`, ranking rules default + `job_count:desc`
- [x] 2.3 Add `SearchCompanies(ctx, CompanySearchParams) (CompanyResult, error)`: builds the Meili filter from `q` + facets (array overlap OR-within/AND-across, scalar membership, `NULL`-matches-none), returns docs + `estimatedTotalHits`
- [x] 2.4 Unit-test the facet→Meili-filter translation for each facet family (array overlap, scalar membership, `job_count > 0` scope, compose with `q`)
- [x] 2.5 Add `RebuildCompanies(ctx, docs)` reusing the atomic swap approach (`companies_rebuild` → `swap-indexes` → `companies`); assert it never references the jobs UIDs

## 3. Reindex worker

- [x] 3.1 Add `cmd/reindex-companies/main.go`: bootstrap config + DB + search client, keyset-scan `companies WHERE job_count > 0`, map via `FromCompany`, push to `companies_rebuild`, promote (atomic swap)
- [x] 3.2 Add a DB query (if none fits) to keyset-page companies for reindex scoped to `job_count > 0`
- [x] 3.3 Verify the worker builds and, against a local Meili + seeded rows, produces a searchable `companies` index

## 4. Handler reroute with Postgres fallback

- [x] 4.1 Reroute `ListCompanies` (`internal/handler/companies.go`): when search is enabled and (`q` or a facet is present) → `SearchCompanies`; on any Meili error or when search is disabled → existing Postgres path. Preserve the `{data, meta}` shape and `job_count > 0` scope; `meta.total` from `estimatedTotalHits` on the Meili path
- [x] 4.2 Handler test: `q` routes to Meili and ranks the exact-name match first (the "arb" case)
- [x] 4.3 Handler test: Meili error falls back to Postgres and still returns HTTP 200 with the substring result
- [x] 4.4 Handler test: empty `q` and no facets returns the catalog ordered `job_count DESC, name` (unchanged), and search-disabled config uses Postgres
- [x] 4.5 Handler test: each facet family filters correctly through the Meili path (regions overlap, YC facets, scalar `maturity`/`subindustries` membership) matching the ported spec scenarios

## 5. Wiring, docs & ops

- [x] 5.1 Confirm no company-search surface bypasses `GET /api/v1/companies` (job-filter typeahead, referral `CompanyPicker`, `HeaderSearch`, catalog) — grep the frontend; no code change expected
- [x] 5.2 Update `internal/search/AGENTS.md` (and `CLAUDE.md` command list) to document the companies index + `cmd/reindex-companies`
- [x] 5.3 Add the `reindex-companies` cron entry (freehire-ops: systemd timer 05:15 UTC + release.sh worker; installed & enabled on host-2 2026-07-20) in `../freehire-ops` (own flock, not stacked with the jobs reindex) — note in the change if ops lives outside this repo

## 6. Verify

- [x] 6.1 `go build ./... && go vet ./... && go test ./...` green
- [x] 6.2 End-to-end: build the index locally, hit `GET /api/v1/companies?q=<exact>` and confirm the exact match ranks first and a typo query resolves; confirm fallback by pointing at a dead Meili
