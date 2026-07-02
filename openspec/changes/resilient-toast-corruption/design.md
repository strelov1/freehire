## Context

`reindex` reads jobs in keyset batches of 2000 via `db.Queries.ListJobsByIDAfter` (`cmd/reindex/main.go`, `pageFetcher`). The `SELECT` detoasts every column server-side, so one row with a broken TOAST pointer fails the whole batch query with `pgconn.PgError.Code == "XX001"`, aborting the run before the index swap. `enrich` instead reads one job at a time via `db.Queries.GetJob` on each claimed outbox entry (`cmd/enrich/store.go`), and already routes failures to fail→retry→dead-letter — so it is per-row isolated but wastes retries on a permanently unreadable row. `ingest` writes and never full-scans+detoasts, so it is unaffected (`backfill-derive` completed for the same reason — it does not touch the corrupted field). The DB runs as `postgres:16-alpine` in `freehire-ops/docker-compose.prod.yml` with the default docker stop grace (10s), so a slow shutdown under load gets SIGKILLed — a likely origin of the corruption.

## Goals / Non-Goals

**Goals:**
- One corrupted (`XX001`) row cannot abort a full-scan worker; it is skipped and logged, the scan completes.
- `reindex` reaches its swap with corrupted rows present (this also closes the live `/facets` 500 once deployed).
- `enrich` dead-letters a corrupted row immediately instead of burning retries.
- Postgres shuts down cleanly to stop producing new corruption.
- Operators can find and repair corrupted rows.

**Non-Goals:**
- Recovering the corrupted field's data (it is unrecoverable; re-ingest/re-enrich repopulates).
- Enabling Postgres data checksums (requires a cluster rebuild via dump/restore — tracked separately).
- Changing the batch/keyset design or any API/schema.

## Decisions

**1. Resilient page helper (shared).** Add `resilientPage(ctx, r ResilientReader, afterID, batchSize) (rows []db.Job, lastID int64, skipped []int64, err error)`:
1. Fast path: `ListJobsByIDAfter(afterID, batchSize)`.
2. On error classified as `XX001` (via `errors.As` → `*pgconn.PgError`, `Code == "XX001"`): call a new `ListJobIDsAfter(afterID, batchSize)` (projects `id` only — no toasted columns, so it never faults), then `GetJob(id)` per id; readable rows collected, `XX001` ids appended to `skipped` and logged, keyset advanced to the last listed id.
3. Any non-`XX001` error returned unchanged.
Place it where both the query set and Job type are in scope; `internal/worker` (already imported by the workers) with `db.Queries` behind a small interface for testability. One new sqlc query `ListJobIDsAfter`.

**2. `reindex` wires the helper.** `fullScan`/`incrementalScan` return via `resilientPage`; the run accumulates `skipped` into the existing stats/log summary. Incremental scan (`--since`) needs the same id-only fallback keyed on `updated_at`; if that adds too much surface, the fallback can list ids with the same predicate — decided during implementation, but the full-scan path is the one that crashed and is mandatory.

**3. `enrich` classifies corruption.** In the claim/process path, wrap the `GetJob` read; if the error is `XX001`, mark the entry dead-lettered (skip retry) with a clear reason. Reuses the existing dead-letter path, only short-circuiting the retry.

**4. Graceful shutdown (ops).** Set `stop_grace_period: 60s` on the `db` service in `docker-compose.prod.yml`. Postgres receives SIGINT/SIGTERM (fast shutdown) from docker and gets 60s to checkpoint before SIGKILL. Optionally set the container's OOM score so the kernel prefers killing a worker over Postgres — noted, not required.

**5. Diagnostics & repair (ops runbook).** A `DO` scan that reads every row and collects `XX001` ids (already used live to find the current bad rows); `pg_amcheck` for an authoritative heap+TOAST check; repair by `UPDATE jobs SET <corrupted field> = '' WHERE id = <bad>` (rewrites the TOAST pointer, row readable again; value returns on next ingest/enrich). A disk-space alert cron (`df` threshold → log/Telegram), since disk-full is a known prior corruption trigger.

## Risks / Trade-offs

- **Row-by-row fallback is slow for a batch that contains a bad row** (up to `batchSize` single-row fetches). Acceptable: corruption is rare, so the penalty applies only to the few batches that contain a bad row, not the whole scan.
- **A corrupted row is silently absent from the index** until repaired+re-ingested. Mitigated by logging skipped ids and the repair runbook — silent only in the index, never in the logs.
- **Repair clears a field**, losing one job's description until refresh. Acceptable — the alternative is an unreadable, unindexable row.
- **`stop_grace_period` lengthens worst-case deploy stop** by up to 60s if Postgres is slow to checkpoint. Acceptable trade for avoiding corruption.
