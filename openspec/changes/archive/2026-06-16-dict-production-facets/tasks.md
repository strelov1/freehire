## 1. FromRow dict-only (core doctrine)

- [x] 1.1 Add failing tests in `internal/jobview/jobview_test.go`: for all six facets (countries, regions, work_mode, skills, seniority, category), the served value is the `jobs` column only — including the case where the LLM has a value but the dictionary column is empty (the served facet must be empty, not the LLM value), and the multi-valued case where the LLM has extra members that must NOT be unioned in.
- [x] 1.2 Change `jobview.FromRow`: serve `countries`/`regions`/`skills` from `j.*` only (drop `mergeSets` with the enrichment values); serve `work_mode` from `j.WorkMode` only; set `e.Seniority = j.Seniority` and `e.Category = j.Category` unconditionally (dictionary wins). Keep clearing the folded enrichment fields so they are not duplicated; leave the stored JSONB untouched.
- [x] 1.3 Remove now-dead merge helpers/branches left by 1.2 (e.g. `mergeSets` if unused), keeping `go build ./...` and `go vet ./...` clean.
- [x] 1.4 Run `go test ./internal/jobview/...` green; confirm no other package's tests regressed.

## 2. Unified backfill query (sqlc)

- [x] 2.1 Add a combined `UpdateJobFacets` query in `internal/db/queries/*.sql` that sets `countries`, `regions`, `work_mode`, `skills`, `seniority`, `category` for one job id (work_mode written as given by the caller, which preserves a set value).
- [x] 2.2 Run `make sqlc` (or `sqlc generate`) and commit the regenerated `internal/db`.

## 3. cmd/backfill-derive (unified pass)

- [x] 3.1 Add failing tests for the backfill runner against a fake store: one pass rewrites all six facet columns from `jobderive.Derive`; it is idempotent; a set `work_mode` is preserved when the location yields no hint; slugs are not written.
- [x] 3.2 Implement `cmd/backfill-derive/main.go`: iterate existing jobs (including closed), call `jobderive.Derive` on each job's raw fields, fill `work_mode` from the derived value only when the row's `work_mode` is empty, and persist the six columns via `UpdateJobFacets`. Mirror the existing backfill command's pagination/connection pattern.
- [x] 3.3 Run the backfill-derive tests green; `go build ./cmd/backfill-derive`.

## 4. Remove the three per-facet backfill commands

- [x] 4.1 Delete `cmd/backfill-geo`, `cmd/backfill-skills`, `cmd/backfill-class` and their per-column sqlc queries; run `make sqlc` and commit the regenerated `internal/db`.
- [x] 4.2 Confirm no remaining references (`grep -r backfill-geo\|backfill-skills\|backfill-class`); `go build ./...` and `go vet ./...` clean.

## 5. Ops wiring

- [x] 5.1 Update the `Dockerfile`: remove the three deleted binaries from the build/COPY list and add `backfill-derive`.
- [x] 5.2 Update the `freehire-ops` cron/compose: collapse the three backfill cron entries into one `backfill-derive` run (followed by a single reindex).

## 6. Verify and migrate

- [x] 6.1 `go build ./... && go vet ./... && go test ./...` all green; `gofmt -l` clean on changed files.
- [x] 6.2 Deploy tail (documented for the operator): rebuild/deploy the app image from origin/main, run `cmd/backfill-derive` once, then a single `reindex`; spot-check that a previously LLM-only skill/region no longer appears in the served facet.
