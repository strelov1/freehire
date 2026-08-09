## Context

`cmd/embed` (`internal/embed`) already implements exactly the pattern wanted: drain
`semantic_outbox`, embed via TEI, write the vector to `jobs.semantic_embedding`
(Postgres) and upsert into `jobs_semantic` (Meilisearch) in one transaction per batch
— the semantic-index sibling of `search-drain`. `EnqueuePendingSemanticJobs` already
gates on `is_tech IS TRUE` (durable code, not a hack) and `reindex --semantic
--from-pg` already rebuilds `jobs_semantic` cleanly from Postgres-persisted vectors
without re-embedding. None of this needs to be built — it needs to be **run and
scheduled**, which never happened after the incremental worker shipped (PR #522,
2026-07-09 — the steady-state cron was a TODO that was never done).

What's running instead is a leftover: `/opt/freehire/embed-supervisor.sh` +
`freehire-embed-supervisor.service`, a one-off wrapper for a since-abandoned
HuggingFace-GPU-endpoint bulk-embed. It restarted `cmd/embed` every ~16 minutes,
paying `EnsureSemanticIndex`'s `settingsUpdate` tax (~150-217s) on every restart —
the 2026-07-23 livelock. It last completed 2026-07-26 and has been dormant since.

Systemd units for host2 live in a separate repo, `~/Projects/freehire-ops`
(`provision/host2/systemd/`). `freehire-search-drain.service`/`.timer` is the direct
template — same shape of problem (a Postgres-outbox-driven, Meilisearch-writing,
run-until-empty worker on a shared single-task-queue engine), including a scar from
a real incident (2026-08-06: search-drain's CPU/IO weight cut 40→10 after it
coincided with nginx accept-queue starvation). **Correction (caught in code
review):** as of this writing `origin/main`'s `freehire-search-drain.service` does
NOT yet have a guard against the facet reindex — that guard exists only on an
unrelated, unmerged local branch (`ops/web-accept-queue-watchdog`) in the same repo.
This change's `freehire-embed.service` adds an equivalent guard on its own merits
(design.md Decision 4), not by copying an already-shipped convention.

## Goals / Non-Goals

**Goals:**
- Close the current ~380k un-embedded backlog once.
- Stand up a real, boring, recurring cron for `cmd/embed` — no custom supervisor,
  no hang-detection heuristics, no HF-endpoint lifecycle management.
- Publish a `jobs_semantic` index that reflects current reality (no closed jobs).

**Non-Goals:**
- Not enabling hybrid search for end users (`web/src/lib/api.ts`'s `semantic_ratio`
  stays 0) — a separate, later product decision.
- Not changing `cmd/embed`, the `is_tech` gate, or the `--from-pg` path — all
  already correct.
- Not moving off local TEI to a GPU/HF endpoint — local TEI throughput (~33 docs/s
  at batch=2000) is adequate for both the one-time catch-up (hours, not days) and
  hourly steady-state deltas (small).

## Decisions

**1. One-time backfill: run `cmd/embed` directly, not through any supervisor**
The 07-23 livelock was caused by a wrapper restarting the worker every 16 minutes,
not by the worker itself. `cmd/embed`'s own `Run()` loop already drains until
`ClaimSemanticBatch` returns empty (same shape as `search-drain`), so a single
un-supervised run is sufficient — the `settingsUpdate` tax is paid once (~150-217s),
not repeatedly. Launch as a transient `systemd-run` unit (survives an SSH drop,
journal-logged) with `EMBED_BATCH_SIZE=2000` (measured ~33 docs/s vs. ~500's slower
per-batch overhead) and no other overrides — default `EMBED_CONCURRENCY=1` is kept
because raising it was measured not to help (TEI's e5-base is thread-bound
regardless of client concurrency, per prior investigation).

**2. Pause `search-drain`'s timer during the one-time backfill only**
The backfill and `search-drain` write to different Meilisearch indexes
(`jobs_semantic` vs `jobs`) but share ONE task queue engine-wide — confirmed
repeatedly this session (the facet-reindex-vs-search-drain contention). A ~380k-doc
backfill running for hours next to a 2-minute-cadence facet drain would slow both.
Stop `freehire-search-drain.timer` before launching the backfill, restart it after.
This is a manual one-off ops step, not baked into any unit file.

**3. The steady-state hourly timer does NOT pause search-drain**
Unlike the one-time backfill, an hourly steady-state run only has a small delta to
embed (new/changed jobs since the last hour) — brief, infrequent contention with
search-drain's 2-minute cadence is not worth adding cross-unit coordination for.

**4. The steady-state timer guards against the facet reindex, independently justified
(not copying an already-shipped convention — see the Context correction above)**
`freehire-reindexw` is the one worker whose lost progress is expensive (a multi-hour
swap-rebuild, atomic). `cmd/embed` gets a guard: skip this tick entirely if
`freehire-reindexw.service` is running, never contend for it.

**Bug found and fixed before shipping**: the first version used
`ExecStartPre=/bin/sh -c '! systemctl is-active --quiet freehire-reindexw.service'`.
`systemctl is-active --quiet` reports success (exit 0) ONLY for the exact `active`
state — a `Type=oneshot` unit mid-`ExecStart` (`freehire-reindexw`'s state for its
*entire* multi-hour run, confirmed live on host2 this session) reports `activating`,
which `is-active --quiet` treats as NOT active. The naive guard would have silently
no-op'd during the one window it exists to catch — verified empirically on host2
with a synthetic oneshot "blocker" unit before this shipped: the naive guard let a
guarded test unit start while the blocker was still `activating`. Fixed by matching
on the known-clear states instead of the busy ones (`case ... in inactive|failed)
exit 0;; *) exit 1;; esac` — fails safe/blocks on any state it doesn't recognize).

**Also verified empirically**: a code-review concern was whether a failing
`ExecStartPre` could desync the timer's `OnUnitActiveSec` scheduling into a rapid
retry storm (risking `start-limit-hit` and a silently-stuck, permanently-failed
unit). Observed on host2 over several real cycles with the blocker held active: the
timer rescheduled cleanly on its normal interval every time, regardless of whether
`ExecStartPre` passed or failed — no rapid retries, no `start-limit-hit`. No
`StartLimitIntervalSec`/`StartLimitBurst` override needed.

**5. Conservative CPU/IO weight from day one**
`search-drain` shipped at `CPUWeight=40`/`IOWeight=40` and only got cut to 10/10
after causing nginx accept-queue starvation. `cmd/embed`'s per-doc cost (a TEI call
+ a Postgres write + a Meili push) is heavier than search-drain's (Meili push only),
and it has zero steady-state track record. Ship the new unit at `CPUWeight=10` /
`IOWeight=10` from the start rather than repeat the same discovery-by-incident.

**6. Hourly cadence (`OnUnitActiveSec=1h`)**
User's explicit choice. Slower than search-drain's 2min (embedding costs more per
doc than a facet push) but still far more current than the status quo (nothing,
indefinitely). Revisit if product ever turns hybrid search on for users and 1h
staleness proves noticeable.

**7. Delete the old supervisor outright, not disable-in-place**
It is not tracked in `freehire-ops` (a hand-placed file/unit on host2 only) and its
entire purpose (HF-GPU-endpoint lifecycle + hang-detection for a flaky restart loop)
is obsolete once a plain recurring timer exists. Nothing depends on it.

**8. Denormalize the freshness sort key into `semantic_outbox` — `ClaimSemanticBatch`
was doing an O(claimable-set) join+sort on every call, not an O(batch-size) one
(found live during the backfill, code change, scope revised from the original
"no Go code changes" premise)**
The one-time backfill measured `EMBED_BATCH_SIZE=2000` at only ~6.2 docs/s
(not the ~33-49 docs/s the Risk mitigation below originally assumed), and bumping
to 5000 only reached ~14 docs/s — a real fix, not just a bigger lever on the same
one. `EXPLAIN (ANALYZE, BUFFERS)` on the live query (~906k claimable rows at the
time) showed why: `ClaimSemanticBatch`'s CTE does `ORDER BY
COALESCE(j.posted_at, j.created_at) DESC, j.id DESC` via a join to `jobs` — since
the sort key lives on the JOINED table, Postgres cannot push the `LIMIT` down
before the join; it nested-loop-joins ALL ~906k claimable rows against `jobs`
(906,305 individual index lookups, ~96.6s of I/O alone) THEN sorts the whole
result (external merge, spilling 47MB to disk) THEN takes the batch. Total: 109s
for a single claim, regardless of `batch_size` — the fixed cost search-drain's own
`EMBED_BATCH_SIZE` mitigation *was* correctly amortizing on `cmd/embed`'s TEI/PG
per-doc costs, but not on this one, which scales with the CLAIMABLE SET size, not
the requested batch size.

Fix: add `semantic_outbox.job_posted_at timestamptz` (copy of
`COALESCE(jobs.posted_at, jobs.created_at)`, written once at enqueue time by
`EnqueuePendingSemanticJobs`) plus a partial index
`(job_posted_at DESC, job_id DESC) WHERE failed_at IS NULL`. `ClaimSemanticBatch`'s
CTE then orders by `o.job_posted_at` directly — no join needed for the ordering,
so Postgres can walk the index in exactly the required order and stop at `LIMIT`,
the same shape `search_outbox_claimable_idx` already gives `search-drain` (though
that index isn't freshness-ordered — `ClaimSearchOutboxBatch` has the textually
identical join-for-ordering pattern and is presumably exposed to the same cost at
a large enough backlog; NOT fixed here, out of scope, flagged for awareness only).
**Correction (caught in code review): `jobs.posted_at` is NOT immutable
post-ingest.** `UpsertJob`'s `ON CONFLICT DO UPDATE` overwrites it unconditionally
on every re-ingest (`internal/db/queries/jobs.sql`), and a moderator edit can
change it too. Combined with `semantic_outbox`'s `ON CONFLICT (job_id,
target_model) DO NOTHING`, an already-queued row's `job_posted_at` can go stale
relative to a later `posted_at` change on the same job — a real behavior change
from the old live-join query, which always read current `posted_at`. Accepted:
this is the identical staleness class the outbox's own `created_at` column
already carries under the same `ON CONFLICT DO NOTHING` (the fresh-first ordering
comment on `EnqueuePendingSemanticJobs` never claimed otherwise for that column),
it self-heals the moment the row is claimed and would be re-derived fresh on the
job's next real change, and it affects only claim ORDER (which job embeds first
under backlog) — never correctness (which jobs get embedded, or duplicated).

Column left nullable (no `SET NOT NULL`) to avoid an extra full-table-scan-under-
lock on a 900k+ row table mid-migration — the app always populates it going
forward and the migration backfills existing rows; `ORDER BY ... NULLS LAST` is
defensive insurance, not load-bearing (`jobs.created_at`, the COALESCE fallback,
is itself `NOT NULL DEFAULT now()`, so a NULL can only arise from a row this
migration's own backfill missed).

**Also corrected in review: the index needs `CONCURRENTLY`, not a plain
`CREATE INDEX`.** `semantic_outbox` is under continuous prod write traffic
(`cmd/ingest`'s enqueue, `cmd/embed`'s claim/update/failure paths); a plain
`CREATE INDEX` holds a `SHARE` lock blocking writes for the whole build. This
repo has an established, recent precedent for exactly this
(`migrations/0078_jobs_source_id_idx.sql`, applied to prod the same day this
change was authored): `-- migrate: no-transaction` plus
`CREATE INDEX CONCURRENTLY IF NOT EXISTS`. The migration now follows that
pattern.

## Risks / Trade-offs

- **[Risk] The one-time backfill hits the same TEI/Postgres contention pattern that
  made the July HF bulk-embed slow (~19 docs/s effective vs. GPU's raw throughput,
  dominated by `ClaimSemanticBatch`'s sort + full job-row loads)** → **Mitigation,
  REVISED**: `EMBED_BATCH_SIZE=2000`/`5000` alone was NOT sufficient (measured
  ~6.2 then ~14 docs/s, not 33-49) — the actual fix is Decision 8's denormalized
  sort key, a real code change. Batch size still matters for the OTHER per-wave
  costs (TEI call batching, Meili push) but was masking, not causing, the
  dominant cost here.
- **[Risk] Backfill runs unattended for hours; TEI or the process could die
  mid-run** → **Mitigation**: `cmd/embed`'s claim/lease design (mirrors
  `search-drain`/`enrich`) means a dead worker just leaves its leased batch to
  expire and get reclaimed by the next run — safe to simply restart the same
  `systemd-run` command if it dies, no cleanup needed.
- **[Risk] Publishing via `reindex --semantic --from-pg` while the hourly timer might
  also fire** → **Mitigation**: run the one-time publish manually, watching it
  complete, before considering the change "done" — don't rely on the timer's first
  automatic fire to also happen to catch a clean state. No code-level mutual
  exclusion exists between them, but this is a one-time launch step, not a repeating
  hazard (both `cmd/embed` steady-state and `reindex --semantic --from-pg` are
  idempotent — an accidental overlap wastes some duplicate work, doesn't corrupt
  anything).

## Migration Plan

1. Author `freehire-embed.service` + `freehire-embed.timer` in `freehire-ops`
   (`provision/host2/systemd/`), PR + merge that repo (separately from `hire` — no
   `hire` code changes in this proposal).
2. On host2: stop `freehire-search-drain.timer`.
3. Launch the one-time backfill as a transient `systemd-run` unit,
   `EMBED_BATCH_SIZE=2000`, monitor to completion (~380k / ~33/s ≈ 3-4h, best-effort
   estimate — could be worse under load, matching this session's facet-reindex
   experience).
4. Restart `freehire-search-drain.timer`.
5. Run `reindex --semantic --from-pg` once, monitor to completion, verify
   `jobs_semantic`'s doc count and spot-check that previously-stale (closed) ids are
   gone.
6. Delete `/opt/freehire/embed-supervisor.sh`, `/opt/freehire/embed-supervisor.log`,
   and disable+delete `freehire-embed-supervisor.service`.
7. Install + enable the new `freehire-embed.timer`; verify one live tick completes
   cleanly (small delta, fast).
8. **Rollback**: `systemctl disable --now freehire-embed.timer` — no data loss risk,
   the worker is a pure incremental drain; stopping it just means semantic freshness
   reverts to "stale until next manual run," the same state this change started from.

## Open Questions

None.
