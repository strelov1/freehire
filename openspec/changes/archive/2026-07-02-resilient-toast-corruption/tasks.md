## 1. Resilient page helper (code, TDD)

- [x] 1.1 Add `ListJobIDsAfter` sqlc query (projects `id` only, keyset by `id`), run `make sqlc`, commit generated code
- [x] 1.2 Add `XX001` classifier helper (`errors.As` → `*pgconn.PgError`, `Code == "XX001"`) with unit test
- [x] 1.3 Implement `ResilientPage` in `internal/worker` behind a small reader interface: fast batch path, `XX001` degrade to id-list + per-row `GetJob`, skip+log corrupted ids, advance keyset; unit tests for healthy batch, one corrupted row, non-`XX001` error propagation

## 2. Reindex uses the helper (code, TDD)

- [x] 2.1 Wire full scan (and `--since` incremental scan) through `ResilientPage`; accumulate skipped ids into run stats and log summary
- [x] 2.2 Test: reindex over a reader that yields a corrupted row skips it, indexes the rest, reaches swap

## 3. Enrich fast-fails on corruption (code, TDD)

- [x] 3.1 Classify `XX001` on the claimed-job read as non-retryable; dead-letter the entry immediately with a clear reason
- [x] 3.2 Test: a corrupted claimed job is dead-lettered without retry; other entries continue draining

## 3b. Unblock CI (pre-existing, separate concern)

- [x] 3b.1 Fix stale `internal/location` city test expectations left red by #374 (code correct, assertions not updated) — a red `go test ./...` blocks this PR's CI

## 4. Graceful DB shutdown (ops)

- [ ] 4.1 Set `stop_grace_period: 60s` on the `db` service in `freehire-ops/docker-compose.prod.yml`; note optional OOM-score tweak in the ops README

## 5. Diagnostics, repair, prevention (ops)

- [ ] 5.1 Add a corruption-scan + `pg_amcheck` runbook (find `XX001` row ids) to ops docs
- [ ] 5.2 Add a repair procedure (clear corrupted field so the row is readable; re-populated on next ingest/enrich) to ops docs
- [ ] 5.3 Add a disk-space alert cron (`df` threshold → log/Telegram) to ops

## 6. Rollout & verification

- [ ] 6.1 Deploy the resilient `reindex` first; run reindex on prod and confirm it passes the corrupted row(s), swaps in, and `/api/v1/jobs/facets` returns 200 with the city facet populated
- [ ] 6.2 Repair the remaining corrupted row(s) found by the scan; re-run `pg_amcheck` to confirm the catalogue is clean
- [ ] 6.3 Apply graceful-shutdown + disk-alert ops changes to prod
