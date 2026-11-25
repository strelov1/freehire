## 1. Add the purge query

- [x] 1.1 Add a failing integration test `internal/db/search_outbox_integration_test.go`
      (`//go:build integration`, testcontainers, mirrors
      `internal/db/jobs_reindex_integration_test.go`'s style) for
      `DeleteSearchOutboxCreatedBefore`: seed rows with `created_at` before and at/after
      a cutoff timestamp, call the query, assert only the before-cutoff rows are gone
      and the at/after ones survive.
      `search_outbox.created_at DEFAULT now()` — backdated via a plain
      `UPDATE search_outbox SET created_at = $1 WHERE job_id = $2` after
      `EnqueueSearchOutbox`, same pattern `jobs_reindex_integration_test.go` uses for
      `jobs.updated_at`.
- [x] 1.2 Add `DeleteSearchOutboxCreatedBefore` (`:execrows`) to
      `internal/db/queries/search_outbox.sql`: `DELETE FROM search_outbox WHERE
      created_at < sqlc.arg(before)`. Run `make sqlc` to regenerate `internal/db`.
      Confirm the test from 1.1 passes.
      `docker run sqlc/sqlc` timed out pulling the image; used the pinned local
      `sqlc v1.31.1` binary instead (`$(go env GOPATH)/bin/sqlc generate`), matching
      the version header already in the generated files.

## 2. Wire the purge into cmd/reindex

- [x] 2.1 In `cmd/reindex/main.go`'s `run()`, capture `startedAt := time.Now()` at the
      very top (before the disk guard and duplicate-marker recompute passes — see
      design.md Decision 1 for why earliest-is-safest).
- [x] 2.2 After the facet-swap branch's `reindexFull(...)` call returns successfully
      (guarded on `!semantic` — search_outbox has no relationship to the semantic
      index), call `q.DeleteSearchOutboxCreatedBefore(ctx, startedAt)`. Best-effort:
      log and continue on error (mirrors the file's existing style for the
      duplicate-marker passes), log the count when `n > 0`.
- [x] 2.3 Considered a dedicated test for the `!semantic` purge gate and skipped it:
      the gate is a one-line conditional identical in shape to the untested `if
      semantic { b = client.NewSemanticRebuild() }` two lines above it in the same
      function — `run()` is not structured for unit testing without a refactor this
      change has no other reason to make, and the query's actual behavior (the part
      that can be silently wrong) is already covered by the real-Postgres integration
      test in 1.1. Verified by reading, not a new test.

## 2a. Code review fix

- [x] 2a.1 Code review (dispatched subagent) found the purge's safety argument broke
      under `EnqueueSearchOutbox`'s `ON CONFLICT (job_id) DO NOTHING`: a job re-changed
      while its outbox row was still pending keeps its OLD `created_at`, so a purge on
      `created_at` alone could delete a row that still represents real, un-pushed work.
      Reproduced with a new RED test
      (`TestDeleteSearchOutboxCreatedBefore_SurvivesRepeatChangeUnderConflictDoNothing`),
      fixed by also requiring `jobs.updated_at < before` in the query (GREEN), and
      updated the original `TestDeleteSearchOutboxCreatedBefore` to pin the purged row's
      `jobs.updated_at` too (it was relying on the now-closed gap without realizing it).
      design.md Decision 1a documents the gap and fix.

## 3. Verify

- [x] 3.1 `go build ./... && go vet ./...`
- [x] 3.2 `go test ./...`
- [x] 3.3 `go vet -tags=integration ./...`
- [x] 3.4 If Docker is available, `go test -tags=integration ./internal/db/... ./cmd/reindex/...`
      All green, including the new `TestDeleteSearchOutboxCreatedBefore` and the
      existing `cmd/reindex` integration suite (`TestReindexFull_*`).
