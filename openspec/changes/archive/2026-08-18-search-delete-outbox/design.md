## Context

The facet index has two write paths and one reconciler:

| Path | What it does | Owner |
|---|---|---|
| `search_outbox` → `cmd/search-drain` | pushes new/changed OPEN canonical jobs | `internal/searchdrain` |
| `internal/linkimport` | single-document push for on-demand imports | `SubmitJobs` |
| `cmd/reindex` swap-rebuild | rebuilds from zero out of open canonical jobs, then swaps | reconciler |

Nothing deletes. `ClaimSearchOutboxBatch` filters `closed_at IS NULL AND duplicate_of IS NULL`,
so a job that closes is never claimed again — its document simply persists until a rebuild
replaces the whole index. `search.Client.DeleteJobs` exists, is idempotent, and is called only
by `internal/search/search_integration_test.go`.

Two measurements from prod on 2026-08-18 shape this design.

**Closure rate: 19,827/day, 4,897 per 3-hour cycle.** That is the standing population of dead
vacancies in search.

**An index operation costs ~8s, not the ~150s `internal/searchdrain/AGENTS.md` documents.**
449 drain batches of 200 documents over two hours: mean 7.69s, min 2.32s, max 21.24s. The
90-180s figure in that file dates from 2026-08-05 and drove both the 600s
`SEARCH_DRAIN_CALL_TIMEOUT_SECONDS` default and the `skipOnTimeout` design. It no longer
describes production. This matters because a design that assumed 150s per operation would have
to amortise deletions aggressively; at 8s it does not.

## Goals / Non-Goals

**Goals:**

- A closed job stops matching search within one drain cycle instead of within one rebuild.
- The enqueue cannot diverge from the close — same transaction, no watermark, no reconciling
  scan.
- Free the rebuild's cadence to become a tuning decision rather than a correctness bound.

**Non-Goals:**

- Jobs demoted to duplicates. `duplicate_of` is set by bulk `UPDATE`s inside the dedup passes
  (478,366 rows in one statement on 2026-08-18) which run adjacent to a rebuild anyway. Same
  symptom, different shape; a separate change if it turns out to matter.
- Actually lengthening the reindex cadence. That is an ops change, gated on this shipping and
  on watching the queue behave.
- Backfilling the documents already stale in the index. The next rebuild clears them; this
  worker owns "from now on".
- Correcting the stale cost figures in `internal/searchdrain/AGENTS.md` beyond noting the new
  measurement. Retuning `SEARCH_DRAIN_CALL_TIMEOUT_SECONDS` is its own decision with its own
  incident history.

## Decisions

### Fold into `cmd/search-drain` rather than add a worker

`freehire-reindexw.service` carries `ExecStartPre=+systemctl stop freehire-search-drain.timer`
and restores it in `ExecStopPost`. A separate deletion worker would need its own copy of that
coupling — otherwise it spends a rebuild's whole duration deleting from an index that is about
to be discarded and swapped. That is wasted work and, worse, a second instance of a discipline
that already had to be automated once after being forgotten by hand.

The drain's job is already "keep the facet index in step with the catalogue incrementally".
Deletion is that job with the opposite sign.

*Alternative considered:* `cmd/search-evict` as its own binary and timer. Rejected on the
pause-coupling argument above; the separation buys nothing, because the two queues share an
index, a lifecycle, and a failure mode.

### A second queue, not a flag on the existing one

`search_outbox` is a reference-only queue whose claim means "index this". Overloading it with
an intent column would make every existing query conditional, and the claim's
`closed_at IS NULL` filter — which is what makes the index/delete race safe — would have to
become conditional too.

`search_delete_outbox` mirrors it: `job_id` (unique), `created_at`, `claimed_at`, `attempts`,
`failed_at`. Same lease/retry shape, so `internal/outbox`'s `RunBatch` drives it unchanged.

### No foreign key to `jobs`

`search_outbox` carries `FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE CASCADE`. Mirroring
that here would be a silent data-loss bug.

`cmd/prune` is the only hard-delete path, and `PruneJobs` deletes by an explicit id list with
NO `closed_at` condition — it can remove an open, indexed job outright. So the sequence
"job closes → removal queued → prune deletes the job before the drain runs → cascade deletes
the queued removal" leaves that document in the index permanently, with nothing left in the
database that knows it should not be there.

The two queues differ in a way that made the mirror instinct wrong. `search_outbox` NEEDS the
job row: the drain reads it to build a document. This queue needs only the primary key, and the
row being gone is the ordinary case rather than a corruption to reap. So it holds bare ids and
takes no referential dependency on the table it is helping to garbage-collect.

That also means `cmd/prune` becomes a sixth enqueue site: a hard-deleted job must leave the
index just as a closed one does. `PruneJobs` already deletes inside a CTE with `RETURNING`, so
the enqueue lands the same way it does on the closing statements.

*Consequence for reaping:* the reap must NOT drop entries whose job no longer exists — for this
queue that is valid work, not garbage. It drops completed and dead-lettered entries instead.

### Enqueue inside the closing statement, via CTE

The five closing queries are bulk `:execrows` statements — `CloseUnseenJobs` closes every
posting a crawl no longer saw in one round trip. A per-row enqueue call would undo that.

```sql
WITH closed AS (
    UPDATE jobs SET closed_at = now(), ... WHERE ... RETURNING id
)
INSERT INTO search_delete_outbox (job_id)
SELECT id FROM closed
ON CONFLICT (job_id) DO NOTHING;
```

One statement, one transaction. A rolled-back close enqueues nothing, and a bulk close of ten
thousand rows enqueues ten thousand entries without ten thousand round trips.

*Alternative considered:* a watermark scan (`closed_at > last_seen`) in the worker, which would
need no changes at the five call sites. Rejected: `closed_at = now()` is stamped at statement
time but becomes visible at commit time, so a transaction that stamps 09:00 and commits at
09:02 is invisible to a worker that advanced its watermark to 09:01 — the row is skipped
forever, silently. Fixable with a lag window, but that trades a guarantee for a guess, and the
call sites turned out to be three files, not the eight the raw query count suggested.

### Idempotency removes the need for coordination

`DeleteJobs` on an id that is not in the index is a no-op. So a retry, an overlap with a
rebuild, or the same job arriving twice all cost nothing and need no locking against the
indexing path.

The index/delete race is already closed on the other side: `ClaimSearchOutboxBatch` filters
`closed_at IS NULL`, so a job closed after being queued for indexing is never claimed by that
entry. The two queues cannot fight over the same job.

### Failure handling follows the existing drain

Batch-level failure falls back to per-item, except when the call context timed out — the
`Runner.skipOnTimeout` distinction, which exists because misclassifying a slow-but-successful
push as failed once turned one slow batch into 500 slow ones and produced a real outage. A
deletion wave inherits this rather than inventing its own policy.

## Risks / Trade-offs

- **The drain's per-run cost roughly doubles** → two index operations per run instead of one.
  At the measured ~8s that is negligible; at the documented 150s it would not be. The
  measurement is two hours old and from one index size, so this should be watched rather than
  assumed. Mitigation: the deletion wave is cheap to disable independently if it turns out to
  cost more than measured.
- **~20k deletions/day is a new steady write load on Meilisearch** → each delete triggers the
  same index re-merge a push does. This replaces work the rebuild was doing anyway, but it
  redistributes it from six big events to a continuous trickle. That is the intent; it is also
  a change in the load shape the host sees.
- **Five closing queries must all be updated** → miss one and jobs closed by that path stay
  searchable indefinitely, silently. Mitigation: a test that asserts every query in the closing
  family enqueues, rather than testing them one at a time.
- **This does not remove duplicates** → the rebuild is still the only thing that drops a job
  demoted to `duplicate_of`. Lengthening the reindex cadence therefore still lengthens how long
  a repost lingers, even after this ships. Stated so the cadence decision is made with it in
  view.

## Migration Plan

One additive migration creating `search_delete_outbox`; no backfill, no data rewrite, nothing
to undo. Before the worker ships the table simply stays empty.

Rollback is a redeploy of the previous binary: the closing queries would revert to not
enqueuing, and any rows left in the table become inert rather than harmful.

Deploy order matters in one direction only: this must ship before the reindex cadence is
lengthened, because that cadence is currently what bounds how long a closed job stays
searchable.

## Open Questions

None. The two decisions that were genuinely open — whether to build a separate worker, and
whether to drive removal from a queue or a watermark — were settled with the user during
brainstorming and are recorded above with their rejected alternatives.
