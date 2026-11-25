## Context

`cmd/reindex/main.go`'s `run()` builds a `db.Queries` handle (`q := db.New(pool)`),
optionally runs several best-effort duplicate-marker recompute passes (none of which
touch `search_outbox` — they're raw `UPDATE jobs SET duplicate_of = ...`), then either
rehydrates the semantic index in place or builds-and-swaps a fresh index via the
`rebuilder` interface (`Prepare` → `Push` per batch → `Promote`). The **facet** rebuild
(`client.NewFacetRebuild()`, selected whenever `!semantic`) is *always* a full,
unscoped scan — `--posted-within` is rejected outright unless `--semantic` is also set
— so every facet reindex run reads the entire `jobs` table's current state.

`search_outbox` (`internal/searchdrain`) is the facet index's own incremental queue,
independent of `semantic_outbox` (the semantic index's queue, owned by
`internal/embed`). A full facet reindex and `cmd/search-drain` are already
mutually-exclusive by design (`freehire-reindexw.service`'s `ExecStartPre` stops
`freehire-search-drain.timer` for the run's duration, per
`internal/searchdrain/AGENTS.md`), but nothing currently drains or trims the queue
itself once the reindex it made redundant finishes.

## Goals / Non-Goals

**Goals:**
- After a successful full facet reindex, remove `search_outbox` rows that are
  provably redundant — their job's content is already in the newly-swapped live index.
- Never remove a row that might still represent real, un-pushed work.

**Non-Goals:**
- Not touching `semantic_outbox` or the semantic reindex path — that queue and index
  are unrelated to this one.
- Not changing `cmd/search-drain`'s own claim/complete/fail logic — the purge is purely
  additive cleanup from the reindex side.
- Not attempting to purge mid-run (e.g., as each batch is scanned) — a single purge
  after `Promote()` succeeds is simpler and the staleness window it leaves (rows queued
  during the run, kept until next cycle) is harmless.

## Decisions

**1. Capture the start timestamp at the very top of `run()`, before anything else**
The safety argument: any `search_outbox` row with `created_at < startedAt` was queued
before this run began, and the reindex's keyset scan reads each job's row at-or-after
`startedAt` — so whatever caused that row to be queued is necessarily already reflected
in the content the scan read. Capturing the timestamp as early as possible (before the
disk guard and the duplicate-marker recompute passes, which can themselves take
significant wall-clock time under load) is *more* conservative than capturing it right
before the scan starts — it only means a slightly smaller set of rows qualifies for
purge this cycle, never an unsafe one. Simplicity favored the earliest sensible point
over shaving that margin.

**1a. `created_at` alone is not sufficient — also require `jobs.updated_at < startedAt`
(found in code review, before merge)**
The first-pass argument above has a gap: `EnqueueSearchOutbox`'s `ON CONFLICT (job_id)
DO NOTHING` stamps `created_at` only on a job's FIRST enqueue since its last drain. A
job whose outbox row is already pending (very plausible — search-drain is paused for
the reindex's whole duration, and the backlog runs 90-113k deep in production) and
that changes AGAIN during the reindex's own scan window keeps its OLD `created_at`,
even though the reindex's keyset scan (ordered by `id`, not by modification recency)
may have already read that job's row before the second change landed. Purging on
`created_at` alone would then delete a genuinely-still-needed entry. `jobs.updated_at`
is stamped in the same transaction as every `EnqueueSearchOutbox` call
(`cmd/ingest/store.go`) and is already the trusted "last real change" signal elsewhere
in this codebase (`ListJobsUpdatedAfter`, `reindex --since`), so the purge query now
requires BOTH `search_outbox.created_at < startedAt` AND the row's `jobs.updated_at <
startedAt` — closing the gap without needing to touch `EnqueueSearchOutbox` itself.
Covered by
`TestDeleteSearchOutboxCreatedBefore_SurvivesRepeatChangeUnderConflictDoNothing`.

**2. Purge only on the facet rebuild path, gated on `!semantic`**
`search_outbox` has no relationship to the semantic index or its `--posted-within`
scoping. Gating on the same `!semantic` boolean `run()` already branches on (the only
condition under which `search_outbox` is even semantically relevant) avoids adding a
new flag or config knob.

**3. Delete, not soft-mark**
`search_outbox` rows have no "already covered by a reindex" state to set — the
existing `DeleteSearchOutboxEntries` query already deletes rows by id after a
successful drain, so a bulk `DELETE ... WHERE created_at < $1` is the same operation
at a coarser grain, not a new pattern.

**4. Best-effort, not fatal**
Mirrors the file's existing style for the duplicate-marker recompute passes (log and
continue rather than fail the run) — but for a different reason: those passes run
*before* the expensive scan/swap and a failure there risks stale markers, while this
purge runs *after* the reindex has already fully succeeded, so a purge failure has
already-safe fallback: the rows just survive one more (harmless) search-drain cycle
before the next reindex tries again.

## Risks / Trade-offs

- **[Risk] Off-by-window race if `created_at` and the reindex's read happen in the same
  instant** → **Mitigation**: none needed — Postgres timestamps have microsecond
  resolution and `time.Now()` is captured in Go before any Postgres round-trip in this
  run, so `startedAt` is provably earlier than the first row the scan reads. Even a
  tied timestamp errs toward NOT purging (strict `<`), which is the safe direction.
- **[Risk] The purge query itself becomes another expensive full-table-ish scan under
  host load** → **Mitigation**: `search_outbox` is a queue table, not the `jobs` table
  — its total row count is the pending backlog (tens of thousands, not millions), and
  `created_at` should use its default/insertion order, so a `WHERE created_at < $1`
  delete is cheap relative to the reindex's own multi-minute precompute passes it
  follows. Not adding an index for this — reassess only if it's measured as a cost.

## Migration Plan

1. Add the sqlc query, regenerate, wire the call into `cmd/reindex/main.go`.
2. Ship as a normal code deploy — no data migration, no reindex required for this fix
   itself (unlike the `proximityPrecision` settings change, this doesn't touch
   Meilisearch index settings at all).
3. Verify: after the next full facet reindex completes on prod, check
   `search_outbox`'s row count and oldest `created_at` before vs. after — the backlog
   predating that reindex run should be gone, and the log line
   (`reindex: purged N stale search_outbox entries...`) should report a count.
4. **Rollback**: revert the purge call (or the whole commit) and redeploy — no data
   loss risk, since a missed purge only means the (harmless, if wasteful) redundant
   entries stay in the queue.

## Open Questions

None.
