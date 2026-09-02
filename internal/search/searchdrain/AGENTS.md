# Facet-search drain conventions

## Scope
Incremental facet-search indexing via the `search_outbox` queue, mirroring `internal/ai/embed`/`semantic_outbox`; the full `reindex` swap-rebuild remains the reconciler.

## Always true
- Work flows through `search_outbox` — a reference-only queue (`job_id` + lease/retry, not a copy of the job or a target-version/target-model column: the facet index has no staleness key beyond `content_hash`, which `cmd/ingest`'s cheap-write gate already checks before enqueuing).
- `cmd/ingest` enqueues (`EnqueueSearchOutbox`, `ON CONFLICT (job_id) DO NOTHING`) inside the SAME transaction as the job's upsert and the enrichment enqueue — atomic with the write, not best-effort after it.
- The enqueue is unconditional: it does not check `MEILI_MASTER_KEY`. `cmd/search-drain` is the sole gate on whether indexing is actually configured; an unconfigured deployment just never drains the queue.
- `ClaimSearchOutboxBatch` joins `jobs` and filters `closed_at IS NULL AND duplicate_of IS NULL` at claim time (not just enqueue time) — a job that closed or became a non-canonical repost between queueing and draining is simply never claimed. That entry no longer sits in the table forever: `cmd/reindex` purges stale rows after a successful facet swap (`DeleteSearchOutboxCreatedBefore`, gated on `o.created_at < startedAt AND j.updated_at < startedAt` so an entry whose job changed again during the run survives for the next drain).
- One wave (`SEARCH_DRAIN_BATCH_SIZE`, default 500) is built and pushed as ONE `IndexJobs` call (awaited — unlike `internal/ingest/linkimport`'s `SubmitJobs`, a silently-dropped push here would leave the outbox entry deleted with nothing actually indexed). On a batch-level failure the runner falls back to per-item processing so one poison/corrupted/deleted row can't sink the wave (mirrors `internal/ai/embed`) — **except when the failure is the call context timing out** (`Runner.skipOnTimeout`): that case is skipped for the whole wave, not fallen back, and the wave stays claimed so its lease expiry retries it fresh next run. See the incident note below for why.
- **The 90-180s per-push figure below is from 2026-08-05 and no longer matches production.** Re-measured 2026-08-18 over 449 drain batches of 200 documents in two hours: **mean 7.69s, min 2.32s, max 21.24s**. The index had shrunk to ~1.47M documents by then. Treat the paragraph below as the worst case it was written from rather than the current cost — but **do not retune `SEARCH_DRAIN_CALL_TIMEOUT_SECONDS` on this measurement alone**: the 600s default and `skipOnTimeout` exist because misclassifying a slow-but-successful push as failed produced a real outage, and a generous timeout costs nothing when pushes are fast.
- **`SEARCH_DRAIN_CALL_TIMEOUT_SECONDS` must stay generous (600s default) and must never drop back near the observed per-push cost.** A push to this index costs 90-180s+, dominated by Meilisearch's whole-index re-merge, almost independent of batch size — a single document costs about as much as a 500-document batch. If the timeout is too tight, a normal successful-but-slow batch gets misclassified as failed; falling back to per-item in that case would turn ONE slow-but-fine batch into up to `BatchSize` equally slow individual pushes, all competing for the same disk IO that starves `freehire-web`'s `accept()` queue — this is not hypothetical, it produced a real ~8-minute outage plus a second unattended multi-hour recurrence on 2026-08-05, the first day this worker ran in prod. `Runner.skipOnTimeout` is the fix: it distinguishes "the call context expired" (skip the wave, no fallback) from "Meilisearch reported a real per-document error" (fall back, since that assumption — isolate one poison row from N-1 healthy ones — still holds for a genuine content defect).
- The document is built the same way the old inline ingest push did: `search.FromJob(row)` + the job-reality signal (`jobview.ClassifyReality`) + widening the canon's geography with its role cluster's (`RoleClusterCount`/`RoleClusterGeo`/`MergeClusterGeography`) — lives in `cmd/search-drain`'s `searchIndexer`, deliberately NOT shared code with `cmd/embed`'s near-identical semantic-index version (each is one small adapter file over a different index; not worth a shared abstraction across two call sites).
- This exists because Meilisearch re-merges its inverted index/facet structures across the WHOLE live index on every push, regardless of batch size — routing every write-path push through one drained queue collapses many small, expensive pushes (formerly one per board per crawl, across ~169 independent `cmd/ingest` processes) into few, fat pushes. "Fat" does not mean "cheap": each push is still 90-180s+ regardless, so the win is fewer total pushes, not fast ones. Before re-enabling the timer after any period it was off, confirm host disk has real headroom (`df -h /`) — a full `reindex` swap-rebuild needs ~2x the live index size free and refuses below `REINDEX_MIN_FREE_GB` (a logged refusal and exit 1, not a crash — and it fails open when the data dir can't be measured, e.g. off the prod host or a misconfigured `MEILI_DATA_DIR`), so a disk squeeze quietly disables the reconciler that would otherwise paper over a drain worker's mistakes.

## The removal queue

`search_delete_outbox` is the mirror of `search_outbox`: that one says "index this job", this
one says "drop this job's document". `cmd/search-drain` drains both in one pass — indexing
first, then removals.

It exists because nothing removed documents incrementally. `ClaimSearchOutboxBatch` filters
`closed_at IS NULL`, so a job that closes is never claimed again and its document simply
persists until the next full swap-rebuild replaces the whole index.
`search.Client.DeleteJobs` was written for exactly this and never wired up — its only callers
were integration tests. Measured on prod 2026-08-18: **19,827 jobs close per day, 4,897 per
3-hour reindex cycle**, and every one of them stayed searchable until the next rebuild.

- **The enqueue rides the closing statement**, as a CTE over the `UPDATE`'s `RETURNING`. All
  five closing queries and `PruneJobs` carry it. A sweep closes a whole provider's stale
  postings in one round trip, so a per-row enqueue would undo that; riding the statement also
  makes the enqueue atomic (a rolled-back close queues nothing) and exact (only rows that
  actually closed are queued).
- **Those queries are `:one`, not `:execrows`** — the CTE moves the row count out of the
  command tag, so they end in `SELECT count(*) FROM closed`. That is the same `int64` the
  callers already received, so no call site changed.
- **`cmd/prune` enqueues too.** It is the only hard-delete path and deletes by id list with no
  `closed_at` condition, so it can remove an OPEN, indexed job outright. After that statement
  the row is gone and nothing downstream could work out it was ever indexed.
- **NO foreign key to `jobs`, deliberately** — the one place this table must not mirror
  `search_outbox`, which carries `ON DELETE CASCADE`. With a cascading key the sequence "job
  closes → removal queued → prune deletes the job before the drain runs" would delete the
  queued removal too, stranding that document in the index permanently. The asymmetry is real:
  `search_outbox` NEEDS the row, because the drain reads it to build a document; a removal
  needs only the primary key, and the row being gone is the ordinary case.
- **A consequence worth knowing before writing tests:** `TRUNCATE ... CASCADE` only reaches
  tables that reference `jobs`, so this one must be named explicitly. `internal/platform/db`'s
  `truncate` helper does. Leave it out and rows leak between tests, which reads as a passing
  assertion about code that was never touched — it happened while building this.
- **The claim has no `EXISTS (SELECT 1 FROM jobs ...)` guard**, unlike the indexing claim.
  There a closed or demoted job is nothing to index; here it is precisely the work. Adding the
  guard would make the queue skip every entry it was created for.
- **No `Reap` either.** `Store.Reap` exists because the indexing claim strands entries it can
  never take. This claim skips nothing, so nothing strands.
- **The index/delete race is already closed on the other side.** A job queued for indexing and
  then closed is never claimed by that entry, because `ClaimSearchOutboxBatch` filters
  `closed_at IS NULL`. The two queues cannot fight over the same job, and `DeleteJobs` is
  idempotent, so overlapping or retried removals cost nothing.
- **Duplicates are still rebuild-only.** `duplicate_of` is set by bulk `UPDATE`s inside the
  dedup passes — 478,366 rows in one statement on 2026-08-18 — which is a different shape of
  problem and runs adjacent to a rebuild anyway. **Whoever lengthens the reindex cadence
  should know that a demoted repost still lingers for a full cycle even after this shipped.**

## How it works
The facet index (`jobs`, plain keyword/facet, no embedder) can otherwise only be refreshed by a full `reindex` — a swap-rebuild from zero on a multi-hour schedule — or by `internal/ingest/linkimport`'s single-document `SubmitJobs` push for its own on-demand imports. `cmd/search-drain` fills the gap `cmd/embed` fills for the semantic index: work flows through `search_outbox`, claimed in waves, indexed in one batch, completed (rows deleted) in one call. The runner lives in `internal/search/searchdrain` behind `Store` + `Indexer` ports (unit-tested with fakes, mirroring `internal/ai/embed/runner_test.go`); `cmd/search-drain` wires the concrete Postgres + Meilisearch adapters. Tuning via `SEARCH_DRAIN_BATCH_SIZE`/`SEARCH_DRAIN_LEASE_SECONDS`/`SEARCH_DRAIN_MAX_ATTEMPTS`/`SEARCH_DRAIN_CALL_TIMEOUT_SECONDS` (`config.LoadSearchDrain`).

## Limitations
- The `freehire-search-drain.timer`/`.service` unit files are NOT tracked in this repo — they live only on host2 (`/etc/systemd/system/`), unlike the application code. Since 2026-08-06, `freehire-reindexw.service` (also host2-only) carries `ExecStartPre=+systemctl stop freehire-search-drain.timer` and `ExecStopPost=+systemctl start freehire-search-drain.timer`, so a full facet reindex automatically pauses the drain for its duration and resumes it on exit — success or failure alike — rather than relying on a human remembering the "never stack" discipline above. Added after an incident where the drain was stopped manually during a stuck reindex investigation and never turned back on.
- **The pause stops the SERVICE, not only the timer** (since 2026-09-02; `freehire-reindexw` and `freehire-reindex-dedup` both do it). Stopping only the timer was right while a drain run meant one batch. It stops being true once the queue carries a backlog, because then a run is a loop over the whole queue: on 2026-09-02 the queue stood at 464k and the drain had run for 6h08m when a rebuild started, so the two shared Meilisearch's one serial task queue for three hours — the rebuild at 6.8k docs/min and each 200-document push at 2m13s, against 26k docs/min and 19s once they were separated. It compounds, too: a slower drain grows the backlog, which lengthens the run, which overlaps more of the next rebuild. **Killing a push mid-flight is safe** — the worker's context is SIGTERM-cancelled and an outbox entry is deleted only AFTER its push succeeded, so a cancelled wave stays claimed and its lease expires for the next run to redo.
- **A successful swap is what collapses a backlog**, not the drain: `DeleteSearchOutboxCreatedBefore` purges every entry whose job did not change during the run. The same 2026-09-02 rebuild took the queue from 464k to 158k in one step. A drain that is starving a rebuild is therefore starving the only thing that can catch it up.
