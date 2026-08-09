## Why

`cmd/embed` (the incremental semantic-embedding worker, `internal/embed`) shipped in
PR #522 (2026-07-09) — it drains `semantic_outbox` and, per job, embeds via TEI and
writes the vector to both Postgres (`jobs.semantic_embedding`) and the live
`jobs_semantic` Meilisearch index in one transaction, exactly mirroring how
`search-drain` drains `search_outbox` into the facet index. But the steady-state cron
for it was a TODO that was never completed. What exists instead is an ad-hoc
one-off bulk-backfill wrapper (`/opt/freehire/embed-supervisor.sh` +
`freehire-embed-supervisor.service`) built for an abandoned HuggingFace-GPU-endpoint
approach, which livelocked in July and has been dormant since it last completed on
2026-07-26. Nothing has embedded a job since then.

Measured live on host2 (2026-08-09): 439,054 jobs are eligible (open, non-duplicate,
`is_tech=TRUE`), only 59,061 carry a current vector — a ~380k backlog. The live
`jobs_semantic` index (729,078 docs) is stale: a 40-id sample found 70% no longer
exist in the live facet index. TEI is alive, idle, with real host headroom.

## What Changes

- One-time manual backfill run of `cmd/embed` to close the ~380k gap (not via the old
  supervisor script).
- New `freehire-embed.service` + `freehire-embed.timer` in `freehire-ops`
  (`provision/host2/systemd/`), modeled on `freehire-search-drain`'s pair, hourly
  cadence (`OnUnitActiveSec=1h` — embedding is heavier per-doc than a facet push, and
  hourly freshness is acceptable for now).
- Delete the dormant `/opt/freehire/embed-supervisor.sh` +
  `freehire-embed-supervisor.service` on host2 (not tracked in `freehire-ops` — a
  hand-placed file, so this is a host2-only cleanup, no repo change for the deletion
  itself).
- After the backfill, run `reindex --semantic --from-pg` (already-shipped) once to
  publish a clean `jobs_semantic` built fresh from Postgres-persisted vectors —
  drops the stale/closed entries the dormant worker left behind.
- **Explicitly out of scope**: `web/src/lib/api.ts`'s hardcoded `semantic_ratio=0`
  stays untouched. Enabling hybrid search for end users is a separate future decision.
- **Scope revised mid-flight (design.md Decision 8)**: the backfill itself surfaced a
  real `ClaimSemanticBatch` performance bug (an O(claimable-set) join+sort instead of
  O(batch-size) — 109s per claim call, measured live via `EXPLAIN ANALYZE`, not fixed
  by batch-size tuning alone). This now includes ONE Go/SQL code change: a new
  `semantic_outbox.job_posted_at` column (denormalized freshness key) + a migration +
  updated `EnqueuePendingSemanticJobs`/`ClaimSemanticBatch` queries. Everything else
  (`cmd/embed` itself, the `is_tech` gate, `--from-pg` rehydration) is still unchanged,
  already-shipped code.

## Capabilities

### New Capabilities
(none)

### Modified Capabilities
(none — no application-code requirement changes; this restores an already-designed
worker to its intended steady-state operation and cleans up its live index.)

## Impact

- `freehire-ops` repo: new `provision/host2/systemd/freehire-embed.service` +
  `freehire-embed.timer`.
- host2: delete `/opt/freehire/embed-supervisor.sh` +
  `freehire-embed-supervisor.service` (+ its `.log` if present); install and enable
  the new timer.
- Data: `jobs.semantic_embedding` gains ~380k more current vectors; `jobs_semantic`
  Meilisearch index gets rebuilt clean via `--from-pg`.
- `hire` repo: new migration (`semantic_outbox.job_posted_at` + index),
  `internal/db/queries/semantic.sql` (`EnqueuePendingSemanticJobs`,
  `ClaimSemanticBatch`), regenerated `internal/db` — see design.md Decision 8.
- Note: `freehire-ops`'s working tree currently carries OTHER unrelated uncommitted
  changes (`.env.example`, `README.md`, a modified `freehire-search-drain.service`,
  new `freehire-site-alert.*` files) — pre-existing work from elsewhere. This change
  must not touch, revert, or bundle those; only the new embed unit files are staged.
