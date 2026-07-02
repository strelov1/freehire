## Why

A single row in `jobs` with corrupted TOAST storage (`missing chunk number 0 for toast value ... SQLSTATE XX001`) crashed the entire facet `reindex` mid-run, so a new deploy's search index never swapped in and `/api/v1/jobs/facets` stayed 500. One unreadable row out of ~2.5M open jobs must not be able to take down a full-catalogue worker. The corruption itself most likely came from Postgres being SIGKILLed (OOM / short docker stop grace) mid-write, so we also close that root cause.

## What Changes

- **Resilient full-scan reads**: a shared helper wraps the keyset batch read so that when a batch fails with `XX001` (data corruption), it degrades to id-only listing + per-row fetch, skips and logs the unreadable row(s), and continues — instead of aborting the whole scan.
- **`reindex` uses the helper**: the facet/semantic reindex completes to the index swap even with corrupted rows present; skipped rows are counted and logged.
- **`enrich` fast-fails on corruption**: the enrichment worker (already per-row) recognises `XX001` and dead-letters the entry immediately instead of burning its retry budget.
- **Graceful DB shutdown (ops)**: raise the Postgres container `stop_grace_period` so `docker stop` lets Postgres finish a clean fast-shutdown before SIGKILL — removing the likely corruption trigger.
- **Diagnostics & repair (ops)**: a corrupted-row scan + `pg_amcheck` procedure to find bad rows, a repair step that clears the corrupted field so the row is readable again (re-populated on next ingest/enrich), and a disk-space alert (disk-full is a known prior trigger).

## Capabilities

### New Capabilities
- `corruption-resilience`: full-scan workers survive individual corrupted (`XX001`) rows by skipping+logging them rather than aborting; operators can detect and repair corrupted rows; the DB shuts down cleanly to prevent new corruption.

### Modified Capabilities
<!-- none: worker read behaviour is new resilience, not a change to an existing spec's requirements -->

## Impact

- **Code (`hire`)**: new `internal/db` (or `internal/worker`) resilient-page helper + `ListJobIDsAfter` query; `cmd/reindex` streaming loop; `internal/enrich` failure classification.
- **Ops (`freehire-ops`)**: `docker-compose.prod.yml` `db` service `stop_grace_period`; diagnostics/repair runbook; disk-space alert cron.
- **Data**: repairing a corrupted row clears one job's corrupted field (e.g. `description`) until its next ingest/enrich refresh.
- **No API/schema breaking changes.**
