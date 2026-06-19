## 1. Schema + DB query layer

- [ ] 1.1 Add migration `migrations/0025_companies_job_count.sql`: `ALTER TABLE companies ADD COLUMN IF NOT EXISTS job_count INT NOT NULL DEFAULT 0;` plus `CREATE INDEX IF NOT EXISTS companies_job_count_idx ON companies (job_count DESC);`
- [ ] 1.2 Edit `internal/db/queries/companies.sql` `ListCompanies`: drop the `LEFT JOIN jobs` / `GROUP BY` / `count()`, select `c.job_count`, and `ORDER BY c.job_count DESC, c.name`. Keep the `q` ILIKE filter and `CountCompanies` unchanged.
- [ ] 1.3 Add a `RecountCompanyJobCounts` query to `companies.sql`: a single set-based `UPDATE companies SET job_count = COALESCE(sub.cnt, 0)` deriving `sub` from `GROUP BY company_slug` over `jobs WHERE closed_at IS NULL`, covering companies with zero open jobs (→ 0).
- [ ] 1.4 Run `make sqlc` (or `go install sqlc` fallback) and confirm `internal/db` regenerates: `db.Company` gains `JobCount`, `ListCompanies` returns it, `RecountCompanyJobCounts` exists. `go build ./...` is green.

## 2. Recount worker

- [ ] 2.1 RED: integration test (`//go:build integration`, testcontainers) asserting `RecountCompanyJobCounts` sets `job_count` to the number of open jobs per company (ignoring `closed_at` rows) and zeroes a company whose jobs all closed.
- [ ] 2.2 RED: integration test asserting `ListCompanies` returns companies ordered by `job_count DESC` (tie-broken by name) and that the `q` filter still matches by name.
- [ ] 2.3 GREEN: add `cmd/recount-companies/main.go` (template: `cmd/backfill-derive` + `internal/worker.Bootstrap`): run `RecountCompanyJobCounts` once, log rows affected, return exit code 0/1. Make tests pass.
- [ ] 2.4 Add `cmd/recount-companies` to the root `Dockerfile` (the `go build` chain and the final-stage `COPY` list). `go vet ./...` + `go build ./...` green.

## 3. Backend facet cheap-win

- [ ] 3.1 In `internal/handler/facets.go`, exclude `company_slug` from the attributes `facetAttributes()` requests distributions for, while leaving it in `search.StringFacets` (so `?company_slug=` filtering is unaffected). Verify the `/jobs/facets` response no longer carries a `company_slug` key but `?company_slug=` filtering on `/jobs` still works.

## 4. Frontend lazy company filter

- [ ] 4.1 Add a `control: 'remote'` option to the `FacetControl` type / `FacetDef` in `web/src/lib/facets.ts`, and switch the `company_slug` FACETS entry from `{ dynamic: true, control: 'select' }` to `{ control: 'remote', ... }`. Keep `companyLabel`/`dynamicLabel`.
- [ ] 4.2 Create `web/src/lib/components/facets/RemoteSearchSelect.svelte`: debounced fetch via `api.listCompanies(q, limit, 0)`, render company `name` + `job_count`, show the popular first page on empty query, call `onToggle(slug)` on select, render selected values as removable chips using a session `slug→name` map with `companyLabel(slug)` fallback.
- [ ] 4.3 Route `control === 'remote'` to `RemoteSearchSelect` in `FacetSection.svelte`, passing the store-keyed `company_slug` state (selected values, exclude) so URL-sync/exclude/chips keep working.
- [ ] 4.4 `cd web && npm run check` (svelte-check) is green; manually confirm (dev or prod-API) that typing "google" returns Google and selecting a company filters the job list.

## 5. Verification + deploy notes

- [ ] 5.1 `go build ./... && go vet ./...` and `go test ./...` green; `go test -tags=integration ./internal/db/` (or the relevant package) green for the new tests.
- [ ] 5.2 Document the deploy steps in the change (full Go image rebuild for the new binary; web image; apply migration 0025 manually via `psql`; run `cmd/recount-companies` once to populate; add the compose worker service + hourly `flock` cron line in `freehire-ops`).
