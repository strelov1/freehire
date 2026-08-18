## Why

The full `reindex` swap-rebuild is currently the ONLY thing that removes a closed job's
document from the facet index. `search.Client.DeleteJobs` exists and its doc comment claims
it is "Used by reindex to drop closed jobs", but its only callers are integration tests — in
production nothing deletes incrementally. `ClaimSearchOutboxBatch` filters
`closed_at IS NULL`, so the drain never even sees a closed job; it simply stops being
touched and stays searchable.

Measured on prod 2026-08-18: **19,827 jobs close per day, 4,897 per 3-hour reindex cycle**.
That is how many dead vacancies sit in search at the worst point of each cycle.

This makes the rebuild's cadence load-bearing for garbage collection, and that cadence is
now hurting: a rebuild takes ~2.5h against a 3h timer, and `freehire-reindexw` stops
`freehire-search-drain.timer` for its whole duration. The incremental path is therefore off
roughly 83% of the time — the reconciler is crowding out the thing it was meant to reconcile.

## What Changes

- A new `search_delete_outbox` queue records jobs that need their document removed from the
  facet index. Rows are written in the SAME statement that closes the job, via a CTE over the
  closing `UPDATE`, so the enqueue is atomic with the close and survives bulk closes.
- `cmd/prune` — the only hard-delete path — enqueues the same way. It deletes by id list with
  no `closed_at` condition, so it can remove an open, indexed job outright.
- The queue holds bare job ids with NO foreign key to `jobs`, unlike `search_outbox`. That
  queue needs the row to build a document; this one needs only the primary key, and the row
  being gone is the normal case rather than an error. A mirrored
  `ON DELETE CASCADE` would delete a pending removal the moment `cmd/prune` removed its job,
  stranding that document in the index permanently.
- `cmd/search-drain` drains that queue alongside the existing index queue, calling
  `search.Client.DeleteJobs` — putting an existing, tested, idempotent method into production
  use for the first time.
- The batch reindex stays the reconciler, but stops being the only garbage collector. Its
  cadence becomes a tuning decision rather than a correctness requirement.
- Not in scope: jobs demoted to duplicates (`duplicate_of`). Those are marked by bulk
  `UPDATE`s inside the dedup passes — 478,366 rows in one statement on 2026-08-18 — which is
  a different shape of problem and still followed by a rebuild. They keep riding the rebuild.

## Capabilities

### New Capabilities

None. This adds to and corrects an existing capability.

### Modified Capabilities

- `job-search`: one requirement gains an incremental removal path, and the requirement that
  currently assigns closed-job removal exclusively to the batch reindex is updated to reflect
  that it is no longer the only route.

## Impact

- `migrations/` — one new migration creating `search_delete_outbox`.
- `internal/db/queries/jobs.sql` — the five closing queries gain a CTE that enqueues.
- `internal/db/queries/pruning.sql` — `PruneJobs` enqueues the ids it hard-deletes.
- `internal/db/queries/search_delete_outbox.sql` — new: claim, complete, reap.
- `internal/searchdrain/` — the runner gains a deletion wave.
- `cmd/search-drain/` — wires the new store and the existing `DeleteJobs` adapter.
- `internal/report/repository.go` — the one closing call site outside `cmd/ingest`.
- `internal/searchdrain/AGENTS.md` — documents the second queue.
- No API change, no user-visible shape change.
- Deployment note: this must ship BEFORE any lengthening of the reindex cadence, since that
  cadence is currently what bounds how long a dead vacancy stays searchable.
