## 1. Prove the transition is readable before building on it

- [x] 1.1 Verify `RETURNING old.col, new.col` works on the Postgres the tests and prod run,
      and that `new.` reflects a BEFORE trigger's rewrite rather than the assigned value.
      **Done before writing the design** — checked against `pgvector/pgvector:pg18`: a row
      whose owned column was cleared returned `was=7, now=NULL`, a row whose owned column was
      set returned `was=NULL, now=5`. Prod is 18.4.

## 2. The three batch passes

- [ ] 2.1 `RecomputeRoleDuplicatesForCompanies`: return `old.duplicate_of`,
      `new.duplicate_of` and `COALESCE(posted_at, created_at)`, then two CTEs — removals into
      `search_delete_outbox`, re-indexes into `search_outbox`. `:execrows` → `:one` ending in
      `SELECT count(*)`.
- [ ] 2.2 Same for `SuppressAggregatorDuplicatesForCompanies`.
- [ ] 2.3 Same for `MarkFuzzyDuplicatesForCompany`.
- [ ] 2.4 `MarkJobDuplicateOfRole` (the ingest-time role verdict) — same treatment; it already
      runs inside the ingest transaction, so the queue write lands atomically with the marker.
- [ ] 2.5 `make sqlc`, fix the call sites the `:one` change touches.

## 3. Tests

- [ ] 3.1 Integration: a canonical posting marked duplicate lands on `search_delete_outbox`
      and NOT on `search_outbox`.
- [ ] 3.2 Integration: a duplicate whose last marker is cleared lands on `search_outbox` with
      the right `job_posted_at`, and not on the removal queue.
- [ ] 3.3 Integration: a duplicate re-pointed at a different canon lands on neither queue.
- [ ] 3.4 **Integration: one pass releases while another still holds — the posting lands on
      NEITHER queue.** This is the case Decision 2 exists for and the one an implementation
      that reads its own column gets wrong.
- [ ] 3.5 Integration: a refresh over an unchanged catalogue queues nothing on either queue.
- [ ] 3.6 Extend the ownership tests' fixture where it overlaps rather than duplicating it —
      the passes' matching behaviour is already covered and must not be re-asserted here.

## 4. Ship

- [ ] 4.1 `gofmt -w`, `go vet ./...`, `go test ./...`, `go vet -tags=integration ./...`, then
      the tagged suites for `internal/db`, `cmd/reindex`, `cmd/ingest`, `internal/linkimport`.
- [ ] 4.2 Deploy. No migration, no backfill, no ordering constraint — the change is live for
      every status change after it lands.
- [ ] 4.3 Confirm on prod that the drain reports removals after the next marker refresh, and
      that a posting marked duplicate leaves `/api/v1/jobs/search` within a drain cycle
      instead of at the next rebuild.
- [ ] 4.4 Note the queue depth a week later against the open question in `design.md` — whether
      the removal batch size wants tuning now that duplicates share the queue with closures.
