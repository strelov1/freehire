## Why

An ingest board is a stateless YAML entry. When a board fails, the failure is only
logged and counted in an ephemeral `stats.Failed` that vanishes when the run exits.
So a board that fails every hour keeps getting hammered every hour, and there is no
way to see *which* boards are unhealthy without grepping logs across runs. This is
the felt operational pain: no per-board error memory, no backoff, no visibility.

This is **Slice-0** of a possible larger ingest-orchestration effort — the cheapest
slice that delivers the actual pain (track errors + cool a failing board down)
without the cost of a scheduler daemon or a job queue. It is deliberately additive
over the existing cron: no new dependency, no schedule change.

## What Changes

- **New `board_health` table**, keyed by `(provider, board)` — a runtime-state
  sidecar recording each board's last outcome: `last_run_at`, `last_success_at`,
  `last_error_at`, `last_error`, `consecutive_failures`, `cooldown_until`,
  `last_ingested_count`. It mirrors the existing `liveness_strikes` idea at board
  granularity.
- **The Runner records each board's outcome** after it crawls: a success resets
  `consecutive_failures` to 0, clears any cooldown, and stamps `last_success_at` +
  `last_ingested_count`; a failure (unknown provider or fetch error) increments
  `consecutive_failures`, stores `last_error`, and sets `cooldown_until = now +
  backoff(consecutive_failures)`.
- **The Runner skips a board whose `cooldown_until` is in the future** — checked
  *before* invoking the adapter — so a persistently-failing board backs off instead
  of crawling every run. It is logged once and counted as cooled. A later success
  self-heals (clears the cooldown); the cooldown is **never permanent**.
- **Backoff is capped and self-healing**: the hourly cron re-run is the natural
  retry for the first couple of failures, so there is no cooldown until a threshold
  of consecutive failures, then an exponential cooldown capped at ~24h (exact policy
  in design). A board always retries after its window.
- **Visibility**: the Runner logs a per-run summary of currently-unhealthy boards
  (in cooldown or with `consecutive_failures > 0`), and the table is directly
  SQL-queryable. A moderator/admin read endpoint is a noted follow-up (**Slice-0.5**),
  not part of this slice.

**Non-goals (explicit):** no scheduler daemon, no job queue, no River; the cron
systemd timers and their cadence stay exactly as they are; boards stay in YAML — git
remains the sole source of truth for the **catalog and the cadence**, while
`board_health` holds **only runtime state**. Scheduling is untouched.

## Capabilities

### New Capabilities

- `ingest-board-health`: Persistent per-board (`provider`, `board`) health state that
  the ingest run records and reads — consecutive-failure tracking, a capped
  self-healing cooldown that skips a failing board, and per-run visibility of
  unhealthy boards. Runtime state only; the YAML catalog is unchanged.

### Modified Capabilities

_None._ The existing `source-ingest` guarantees still hold — per-run failure
isolation is unchanged; board-health is an additive overlay, not a change to how
adapters, the write path, or scheduling behave.

## Impact

- **New migration** adding `board_health`. Note: there is no versioned migration
  runner — it must be applied **manually to prod (PG18) before deploy**, per the
  deploy convention.
- **New sqlc queries**: upsert-on-success / upsert-on-failure, load cooldowns for the
  boards in a run, list unhealthy boards.
- **Wiring** in `internal/pipeline` (Runner records outcome + skips cooled boards via
  an injected health port) and `cmd/ingest` (the adapter over `*db.Queries`).
- **Behavior**: a board in cooldown is skipped for its window — the only observable
  change to a run; everything else (crawl, normalize, upsert, stale sweep) is
  unchanged.
