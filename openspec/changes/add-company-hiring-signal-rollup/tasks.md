## 1. Schema

- [x] 1.1 Add migration `migrations/0029_insights_company_stats.sql` creating `insights_company_stats(company_slug text NOT NULL, day date NOT NULL, added int NOT NULL DEFAULT 0, removed int NOT NULL DEFAULT 0, open int NOT NULL DEFAULT 0, PRIMARY KEY (company_slug, day))` plus an index on `(day)` for as-of/cross-company scans, with a header comment matching the `insights_*` rollup convention (pure function of `jobs`, atomic rebuild, initdb-only + manual-on-prod note).

## 2. Rebuild query (TDD)

- [x] 2.1 RED: add a failing integration test (`//go:build integration`) in `internal/db` that seeds `jobs` covering the spec scenarios — a canonical open job, a job closed on a later day, a `duplicate_of` repost copy, and an empty-`company_slug` job — runs the rebuild, and asserts `added`/`removed`/running `open` rows per company/day (including that duplicates and company-less rows contribute nothing, and `open` is the running `cumAdded − cumRemoved`).
- [x] 2.2 GREEN: add `DeleteAllInsightsCompanyStats` and `RebuildInsightsCompanyStats` to `internal/db/queries/insights.sql` (per-company aggregate of `created_at::date` as added and `closed_at::date` as removed, filtered to `company_slug <> '' AND duplicate_of IS NULL`, with a window `SUM(...) OVER (PARTITION BY company_slug ORDER BY day)` for running `open`); regenerate sqlc (`make sqlc`); make the test pass.
- [x] 2.3 REFACTOR + simplify the query and test under green.

## 3. Rollup worker

- [x] 3.1 Add `cmd/rollup-company/main.go` mirroring `cmd/rollup-stats`: `worker.Main`/`worker.Bootstrap`, one transaction, `SET LOCAL work_mem = '256MB'`, `DeleteAllInsightsCompanyStats` then `RebuildInsightsCompanyStats`, atomic commit, non-zero exit + log on failure, with a package doc comment describing it as a run-once-and-exit rollup.
- [x] 3.2 `go build ./... && go vet ./...`; run the integration test (`go test -tags=integration ./internal/db/`) green.

## 4. Docs

- [x] 4.1 Update `CLAUDE.md` (layout tree + Commands section) to list `cmd/rollup-company` alongside `cmd/rollup-stats`/`cmd/rollup-facets`, noting it needs only `DATABASE_URL` and is cron-scheduled (~daily).
