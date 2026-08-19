## 1. Prove the transition is readable before building on it

- [x] 1.1 Verify the transition is readable atomically. **Done before writing the design**,
      first with Postgres 18's `RETURNING old.col, new.col` against `pgvector/pgvector:pg18`:
      a row whose owned column was cleared returned `was=7, now=NULL`, one whose column was
      set returned `was=NULL, now=5`, and `new.` reflected the BEFORE trigger rather than the
      assigned value. **sqlc 1.31.1 then rejected that syntax** — its analyzer predates PG18 —
      so the implementation reads the old value from the statement's own snapshot instead
      (Decision 1). Portable, no larger, and the verification still earned its keep: it
      established the property before anything was built on it.

## 2. The three batch passes

- [x] 2.1 `RecomputeRoleDuplicatesForCompanies`: carry the pre-update `duplicate_of` through
      the existing `target` CTE, return it with the post-update value and
      `COALESCE(posted_at, created_at)`, then two CTEs — removals into `search_delete_outbox`,
      re-indexes into `search_outbox`. `:execrows` → `:one` ending in `SELECT count(*)`.
- [x] 2.2 Same for `SuppressAggregatorDuplicatesForCompanies` — its `target` CTE gains a join
      back to `jobs` for the pre-update value, since it aggregates over match candidates.
- [x] 2.3 `MarkFuzzyDuplicatesForCompany` drives off an `unnest` rather than a scan, so it
      gains a small `before` CTE reading only the rows it is about to touch.
- [x] 2.4 `MarkJobDuplicateOfRole` (the ingest-time role verdict) — same treatment; it already
      runs inside the ingest transaction, so the queue write lands atomically with the marker.
      Carries BOTH branches even though its two callers only ever set a canon and the clearing
      branch is therefore unreachable today: the argument is nullable, the other three writers
      are symmetric, and a future caller clearing the marker here would otherwise leave a
      now-canonical posting out of search until the next rebuild, silently.
- [x] 2.5 `make sqlc`, fix the call sites the `:one` change touches. None needed changing —
      the callers already took `int64`.

## 3. Tests

- [x] 3.1 Integration: a canonical posting marked duplicate lands on `search_delete_outbox`
      and NOT on `search_outbox`.
- [x] 3.2 Integration: a duplicate whose last marker is cleared lands on `search_outbox` with
      the right `job_posted_at`, and not on the removal queue.
- [x] 3.3 Integration: a duplicate re-pointed at a different canon lands on neither queue.
- [x] 3.4 **Integration: one pass releases while another still holds — the posting lands on
      NEITHER queue.** This is the case Decision 2 exists for and the one an implementation
      that reads its own column gets wrong.
- [x] 3.5 Integration: a refresh over an unchanged catalogue queues nothing on either queue.
- [x] 3.6 Extend the ownership tests' fixture where it overlaps rather than duplicating it —
      the passes' matching behaviour is already covered and must not be re-asserted here.

## 4. Ship

- [x] 4.1 `gofmt -l .` clean, `go vet ./...`, `go test ./...`, `go vet -tags=integration ./...`,
      then the tagged suites for `internal/db`, `cmd/reindex`, `cmd/ingest` and
      `internal/linkimport` — all pass.
- [ ] 4.2 Deploy. No migration, no backfill, no ordering constraint — the change is live for
      every status change after it lands.
- [ ] 4.3 Confirm on prod that the drain reports removals after the next marker refresh, and
      that a posting marked duplicate leaves `/api/v1/jobs/search` within a drain cycle
      instead of at the next rebuild.
- [ ] 4.4 Note the queue depth a week later against the open question in `design.md` — whether
      the removal batch size wants tuning now that duplicates share the queue with closures.
