## 1. Migration: index + estimate function

- [x] 1.1 Add a migration `migrations/00NN_jobs_open_created_index.sql` with a plain
  `CREATE INDEX IF NOT EXISTS jobs_open_created_idx ON jobs (created_at DESC, id
  DESC) WHERE closed_at IS NULL;` and a `CREATE OR REPLACE FUNCTION
  estimate_open_jobs() RETURNS bigint` that returns the planner's `Plan Rows`
  estimate for `SELECT 1 FROM jobs WHERE closed_at IS NULL` (via `EXPLAIN (FORMAT
  json)`). Pick the next free migration number.

## 2. Query + handler

- [x] 2.1 In `internal/db/queries/jobs.sql`, rename `CountJobs` to
  `EstimateOpenJobs` and change its body to `SELECT estimate_open_jobs()::bigint;`.
  Run `make sqlc` (or `sqlc generate`) and commit the regenerated `internal/db`.
- [x] 2.2 Update `internal/handler/jobs.go` `ListJobs` to call
  `a.queries.EstimateOpenJobs(...)` instead of `CountJobs(...)` for the list
  total; the `ListJobs` query itself is unchanged (it now rides the new index).

## 3. Tests

- [x] 3.1 Add/extend an integration test (`//go:build integration`, testcontainers)
  that inserts a mix of open and closed jobs and asserts: `ListJobs` returns only
  open jobs ordered by `created_at` desc then `id` desc with correct
  limit/offset; and `EstimateOpenJobs` executes and returns a non-negative
  `int64`. Run with `go test -tags=integration ./internal/...`.

## 4. Verify

- [x] 4.1 `go build ./...`, `go vet ./...`, `go test ./...` (unit), and the
  integration test above are green. Confirm the migration file is valid SQL and
  the sqlc-generated code compiles.

## 5. Prod apply note (documentation)

- [x] 5.1 Document in the change (and PR body) that on prod the index is applied
  manually and non-locking: `CREATE INDEX CONCURRENTLY jobs_open_created_idx ON
  jobs (created_at DESC, id DESC) WHERE closed_at IS NULL;` (initdb only runs on
  first volume init; `CONCURRENTLY` can't be in the migration file), followed by
  `CREATE OR REPLACE FUNCTION estimate_open_jobs() ...`, then `ANALYZE jobs;` so
  the first `meta.total` estimate is warm (autovacuum would refresh it eventually,
  but an explicit ANALYZE avoids an initially-off total). Note the INVALID-index
  drop-and-retry caveat if the concurrent build is interrupted.
