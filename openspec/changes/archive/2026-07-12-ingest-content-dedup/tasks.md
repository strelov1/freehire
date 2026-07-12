## 1. Schema

- [x] 1.1 Migration: `jobs.duplicate_of bigint NULL REFERENCES jobs(id)` + index for
  the list/enqueue filters. Comment "apply to prod manually before deploy".
- [x] 1.2 Add `RecomputeRoleDuplicates` to `internal/db/queries/jobs.sql` (set
  `duplicate_of` = the `min(id)` open canon for each open `(company_slug,
  role_fingerprint)` cluster with >1 open rows; null out canon/singletons/empty-fp).
  Regenerate `internal/db` via sqlc; `go build ./...` clean.

## 2. Recompute correctness

- [x] 2.1 RED→GREEN (integration, `-tags=integration ./internal/db/`): seed a cluster
  of identical-fingerprint open jobs; run `RecomputeRoleDuplicates`; assert one canon
  (`min(id)`, `duplicate_of` null) and the rest reference it. Assert an empty-fp row
  and a singleton stay canonical, and that closing the canon promotes the next `min(id)`
  on re-run (failover + stability).

## 3. Hide reposts from list + enrichment

- [x] 3.1 RED→GREEN: the jobs-list query and the enrichment-enqueue query exclude
  `duplicate_of IS NOT NULL`. Tests: a repost is absent from the list; a repost is not
  enqueued; the canonical row appears/enqueues normally.

## 4. Hide reposts from search

- [x] 4.1 RED→GREEN: the reindex and incremental-push paths skip rows with a non-null
  `duplicate_of`. Tests: a repost is not indexed; the canonical row is.

## 5. Openings count + wire-up

- [x] 5.1 RED→GREEN: surface the canonical job's cluster open count as its openings
  count in `internal/jobview` (reuse the reality `mass_count`). Test the projection.
- [x] 5.2 Wire `RecomputeRoleDuplicates` into the reindex path (and expose it for a
  cron recompute, mirroring the role-cluster recount). `go test ./...` and the
  integration tests green.
