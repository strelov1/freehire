# Reindex disk-safety & disposable semantic index

**Date:** 2026-07-24
**Status:** approved (design)
**Scope:** `cmd/reindex`, `internal/search`, `internal/config` (freehire); one systemd timer/script (freehire-ops)

## Problem

host2 (`/dev/sda1`, 301G) recurrently hits 100% disk, taking down the whole app:
Postgres can no longer write (crash-loop, or `SQLSTATE 53100 No space left` on temp
files), the embed worker crash-loops, and background workers stall. This has recurred
repeatedly (2026-06-17, 06-25, 07-03, 07-23/24), each time "fixed" by hand, never
structurally.

**Root cause.** freehire keeps two large Meili indexes resident — `jobs` (facet/keyword,
~50G) and `jobs_semantic` (hybrid vectors, ~45G) — on a disk too small for even one
swap-rebuild. A full reindex uses a fresh-index-plus-atomic-swap strategy
(`internal/search/client.go` `Rebuild`): it builds a complete second copy
(`jobs_rebuild` / `jobs_semantic_rebuild`, ~another 45–50G) then swaps. On a chronically
tight disk the build fills the disk, aborts before the swap, and leaves the orphan
rebuild index behind. `Rebuild.Prepare` only drops a leftover rebuild index on the NEXT
run, so the orphan lingers (13h+ in the 07-23 incident) and eats ~50G until dropped by
hand.

Two facts make a clean structural fix possible:

1. **`jobs_semantic` is a disposable, derived index.** Its vectors are durably stored in
   Postgres (`jobs.semantic_embedding`, migration 0021; backed up by the nightly
   pg_dump). It can be rebuilt from Postgres with no re-embedding (no TEI/GPU cost) —
   `reindex --semantic --from-pg` already does this via the swap path. Losing it
   temporarily is acceptable (it backs `/similar` and CV recommendations, not the main
   search; hybrid main-search is not enabled yet).
2. **The embed worker already writes `jobs_semantic` in place** (upsert, no swap) — so an
   in-place bulk rehydration needs no new atomicity story.

## Goals

- A full reindex can never fill the disk (no more disk-full-from-reindex outages).
- No orphan rebuild index is ever left behind.
- Rebuilding `jobs_semantic` is cheap in BOTH cpu (no re-embed) AND disk (no 2× copy), so
  it can be treated as disposable — dropped to free ~45G and rebuilt on demand.
- Early warning before the disk is critical.

## Non-goals

- Growing the disk / moving other tenants (that is an infra decision, out of scope here).
- Changing the facet index's swap-rebuild strategy (it needs the atomic swap for the live
  search index and has no Postgres shortcut).
- Enabling hybrid search in the SPA (separate, later work).

## Design

### Component 1 — In-place semantic rehydration from Postgres (`cmd/reindex`, `internal/search`)

Make `reindex --semantic --from-pg` rebuild `jobs_semantic` **in place**, without a
rebuild copy or swap:

1. Ensure a FRESH empty `jobs_semantic` — drop it if present, then create it with the
   semantic settings (userProvided embedder). Dropping first means the old copy's space is
   released before the new fill, so peak disk usage is ~1× the index, never 2×.
2. Stream every open, canonical job carrying a persisted vector
   (`jobs.semantic_embedding`) and upsert its document (with vector) directly into the
   live `jobs_semantic`, in batches, reusing `semanticDocsFromPG`. No TEI calls.

Because the index is freshly (re)created, there are no stale documents to delete — closed
and non-canonical jobs are simply never pushed (same `splitJobs` filtering the reindex
feed already applies). During the fill, `/similar` and recommendations return partial or
empty results; acceptable for a disposable index (this is the same in-place,
non-atomic model the embed worker already uses).

**Decision:** `--from-pg` is ALWAYS in-place. The swap-based `NewSemanticRebuildFromPG`
path is removed — a from-PG rebuild via swap has no benefit and is exactly what created
the disk pressure. (`NewSemanticRebuild` — the TEI-embedding swap rebuild — is retained
for a from-scratch re-embed if ever needed, and is covered by the Component 2 guard.)

### Component 2 — Pre-flight disk guard + defer-cleanup (`cmd/reindex`, `internal/search`, `internal/config`)

For the FACET reindex (which must swap — no Postgres shortcut, needs atomicity for the
live search index) and any TEI swap rebuild:

- **Guard.** Before `Rebuild.Prepare`, measure free disk via `syscall.Statfs` on the Meili
  data dir and abort with a clear log + non-zero exit if free is below a configured floor.
  - New config: `MEILI_DATA_DIR` (default `/var/lib/freehire/meili`), `REINDEX_MIN_FREE_GB`
    (default `70`).
  - The check is a single syscall at startup — zero cost on the indexing hot path.
  - We deliberately do NOT size the threshold from Meili's reported index size:
    `/indexes/<uid>/stats` `rawDocumentDbSize` under-reports on-disk footprint ~8× (e.g.
    6G reported vs ~51G on disk for `jobs_semantic`), so an adaptive check would be
    unreliable. A statfs floor is simple, honest, and operator-tunable.
  - On trip: `log.Printf("reindex: refusing — free %dG < required %dG (REINDEX_MIN_FREE_GB); skipping to avoid disk-full")` and `return 1`. Search stays as-is; incremental ingest keeps the facet index fresh (the full reindex is a reconciler, not the freshness path).
- **Defer-cleanup.** Add `Cleanup(ctx)` to the `rebuilder` interface (best-effort drop of
  the rebuild index, idempotent). In `reindexFull`, `defer` it, running only when a
  successful `Promote` was NOT reached (tracked by a `promoted` flag). Best-effort: log on
  failure, never mask the original error. Caveat: under a genuine 100%-full disk the
  cleanup's Meili `indexDeletion` may not schedule immediately (serial task queue) — this
  is a safety net for non-disk aborts; the guard is what prevents disk-full.

### Component 3 — Ops disk alert (freehire-ops)

A `freehire-disk-alert.service` (oneshot, `/opt/freehire/bin/disk-alert.sh`) + `.timer`
(every 15 min), mirroring the `freehire-pg-backup` pattern under
`provision/host2/systemd/`. The script checks `df /` and, at ≥85% used, emits an alert.
**Open detail:** delivery channel — reuse the project's Telegram bot (token/chat from
`/opt/freehire/.env`) if available, else email via the existing SES path. Finalized when
the ops side is written.

## Operational sequencing (not code)

1. Ship Components 1 & 2, deploy.
2. Verify in-place `--from-pg` rebuilds `jobs_semantic` correctly and disk-cheaply.
3. THEN (optional, when disk headroom is wanted): drop `jobs_semantic` on prod to free
   ~45G (82%→~67%); rebuild on demand via the proven in-place path before hybrid/similar
   is needed. Not urgent — disk is currently ~82% after the 07-24 orphan cleanup.

## Testing

- **Guard (unit):** inject the free-space probe; assert abort when free < floor and
  proceed when free ≥ floor. Assert the abort happens before any index is created.
- **Defer-cleanup (unit):** a rebuilder stub — assert `Cleanup` is called when the build
  errors before Promote, and NOT called after a successful Promote.
- **In-place from-PG (unit):** assert the from-PG path ensures a fresh index and pushes
  only open-job vectors via `semanticDocsFromPG`, with no swap/rebuild index created.
- **Ops:** smoke-test `disk-alert.sh` on host2 (dry-run threshold).

## Risks

- Guard may refuse the facet reindex until the disk grows (free ~53G vs ~50G index +
  overhead). This is intended — refusing is safe; filling the disk is an outage. Incremental
  indexing keeps the facet index fresh meanwhile.
- In-place semantic rebuild degrades `/similar` and recommendations during the fill (index
  incomplete). Accepted — the index is disposable and the rebuild is fast from PG.
