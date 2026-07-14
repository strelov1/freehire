# Tasks

## 1. RefreshCompanyFacets — employee_count-authoritative company_sizes (SQL)

- [x] 1.1 **RED** — Add an integration test (`//go:build integration`,
  testcontainers) in `internal/db` seeding: a company with `employee_count = 320`
  plus open jobs whose `enrichment.company_size` is `11-50`/`51-200` (expect
  `company_sizes = {201-500}`, the bucket, not the LLM union); a company with no
  `employee_count` and an enriched job `11-50` (expect `{11-50}`, fallback); a
  company with `employee_count = 5` (expect `{1-10}`); a company with
  `employee_count` but zero open jobs (expect the bucket, not empty). Call
  `RefreshCompanyFacets` and assert. Confirm it fails (current union ignores
  `employee_count`).
- [x] 1.2 **GREEN** — Add the `csize_final` CTE to `RefreshCompanyFacets` in
  `internal/db/queries/companies.sql` (employee_count bucket, else `csize` union),
  point `company_sizes` SET + the `IS DISTINCT FROM` guard at `csize_final.arr`,
  and `LEFT JOIN csize_final`. `make sqlc`.
- [x] 1.3 Confirm the existing `company_sizes` facet tests still pass (the
  no-`employee_count` companies keep the union), and the idempotency guard still
  short-circuits an unchanged company.

## 2. Verify + backfill

- [x] 2.1 `go build ./... && go vet ./...` and
  `go test -tags=integration ./internal/db/` green.
- [x] 2.2 Document the deploy step: deploy (no migration needed), then run
  `cmd/recount-companies` once to rematerialize `company_sizes` from the more
  accurate source.
