## 1. Schema

- [x] 1.1 Add migration `migrations/0042_company_info.sql`: columns `industries TEXT[] NOT NULL DEFAULT '{}'`, `year_founded INT`, `employee_count INT`, `hq_country TEXT`, `organization_type TEXT`, `tagline TEXT`, `company_info JSONB NOT NULL DEFAULT '{}'`, `is_reference BOOLEAN NOT NULL DEFAULT false`, `company_info_at TIMESTAMPTZ`
- [x] 1.2 Add `CREATE INDEX IF NOT EXISTS companies_industries_idx ON companies USING GIN (industries)` in the same migration

## 2. DB access (sqlc)

- [x] 2.1 Add `UpsertCompanyInfo` in `internal/db/queries/companies.sql`: `INSERT ... ON CONFLICT (slug)` writing ONLY the company-info columns + `company_info_at = now()` + `name`; on insert set `is_reference = true`; on conflict update only company-info columns (never `job_count`/`collections`/job-derived facets/`is_reference` of an existing row)
- [x] 2.2 Guard `DeleteOrphanCompanies` with `AND NOT c.is_reference`
- [x] 2.3 Run `make sqlc` and commit the regenerated `internal/db`

## 3. Backfill worker

- [x] 3.1 Add `cmd/backfill-company-info/main.go` (run-once, needs `DATABASE_URL`; file path as arg): stream the JSONL, map name→slug via `internal/normalize`, call `UpsertCompanyInfo` per record
- [x] 3.2 Map record fields → params: empty/zero source values → NULL; assemble `company_info` JSONB from homepage + funding/stock/parent/subsidiaries/activities; keep the loader source-agnostic (no origin named in code, comments, or logs)
- [x] 3.3 Log matched-existing vs inserted-reference counts (and skipped/blank-name rows) for the match-rate measurement

## 4. Company detail exposure

- [x] 4.1 Map the new columns into the company-detail response shape (jobview/company detail), rendering company info on the company row; leave list/search/facets unchanged (Phase 2)

## 5. Tests

- [x] 5.1 `internal/db` integration test (build-tagged `integration`): insert-new-as-reference, update-existing-preserves-`job_count`/facets, idempotent re-run
- [x] 5.2 Integration test: `DeleteOrphanCompanies` deletes a jobless non-reference company but skips an `is_reference` one
- [x] 5.3 Unit test for the loader's record→params mapping (empty/zero → NULL, JSONB extras assembled) over fixture JSONL lines

## 6. Verify

- [x] 6.1 `go build ./... && go vet ./... && go test ./...`; run the backfill against the dump, confirm matched/inserted counts and spot-check enriched + reference rows
