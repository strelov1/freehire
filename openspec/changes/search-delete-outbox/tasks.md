## 1. The queue

- [x] 1.1 Add a migration creating `search_delete_outbox` (`job_id` unique, `created_at`,
      `claimed_at`, `attempts`, `failed_at`) with the index the claim order needs. Deliberately
      NO foreign key to `jobs`: a queued removal must outlive the row it refers to, or
      `cmd/prune` silently strands that document in the index. Say so in the migration comment.
- [x] 1.2 Add `internal/db/queries/search_delete_outbox.sql` — claim a lease-expired batch,
      complete by id, and reap COMPLETED/dead-lettered entries. The reap must not key on the
      job being gone; for this queue that is valid work. Run `make sqlc`.

## 2. Enqueue at the close

- [x] 2.1 Rewrite `CloseUnseenJobs` as a CTE that closes and enqueues in one statement, and
      cover it with an integration test asserting a bulk close of N jobs leaves N queue rows.
- [x] 2.2 Do the same for the remaining four: `CloseUnseenJobByID`,
      `CloseUnseenJobsBySource`, `CloseJobBySourceExternalID`, `CloseJobByID`.
- [x] 2.3 Add a test that enumerates the closing family and fails if any member closes a job
      without enqueuing it — so a sixth closing query added later cannot silently skip the
      queue. This is the task that keeps the change honest; do not fold it into 2.2.
- [x] 2.4 Assert a rolled-back close leaves no queue row.
- [x] 2.5 Make `PruneJobs` enqueue the ids it hard-deletes, in its existing `DELETE ...
      RETURNING` CTE. Test that pruning an OPEN indexed job queues its removal, and that the
      queue row survives the job row being gone — the case a mirrored `ON DELETE CASCADE`
      would have silently eaten.

## 3. Draining

- [x] 3.1 Extend `internal/searchdrain` with a deletion wave: claim, call the deleter,
      complete. Unit-test against fakes the way the indexing runner is tested — including that
      a delete for an unindexed job completes rather than retrying forever.
- [x] 3.2 Inherit the existing failure policy: batch failure falls back to per-item, a
      call-context timeout skips the wave without fallback (`skipOnTimeout`). Test both.
- [x] 3.3 Wire `cmd/search-drain` to the new store and to `search.Client.DeleteJobs`, and
      assert the drain runs both waves in one pass.

## 4. Documentation

- [x] 4.1 Document the second queue in `internal/searchdrain/AGENTS.md`: what it holds, why the
      enqueue lives in the closing statement, and why the index/delete race is already closed
      by the indexing claim's `closed_at IS NULL` filter.
- [x] 4.2 Record the measured index-operation cost (mean 7.69s over 449 batches of 200 on
      2026-08-18) beside the existing 90-180s claim, dated — without retuning
      `SEARCH_DRAIN_CALL_TIMEOUT_SECONDS`, which is a separate decision with its own incident
      history.
- [x] 4.3 Note in `internal/searchdrain/AGENTS.md` that duplicates are still rebuild-only, so
      whoever lengthens the reindex cadence sees what that still costs.

## 5. Verification

- [x] 5.1 Run `gofmt -l .` (must print nothing), `go vet ./...`, `go test ./...`,
      `go vet -tags=integration ./...`, and the tagged DB suite
      `go test -tags=integration ./internal/db/`.
