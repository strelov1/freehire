## Context

`pipeline.Runner` crawls each configured board concurrently (`ingestBoard`), returning
per-board `(ingested, failed, skipped)` counts that fold into an ephemeral `RunStats`.
A board failure (unknown provider / fetch error) is logged and counted, then forgotten
when the run exits. There is no persistent per-board state. The codebase already has
the shape we need in three places — `liveness_strikes` (a consecutive counter that
drives an action), the enrichment retry/lease, and `telegram_posts` attempts — so this
is a fourth instance of an established pattern, not a new one.

This is **Slice-0**: the cheapest slice that delivers per-board error memory + cooldown
over the existing cron, deferring the scheduler-daemon / queue (a validated but not-yet-
needed bet) to a future slice.

## Goals / Non-Goals

**Goals:**
- Persist per-board outcome so a failing board is remembered across runs.
- Back a repeatedly-failing board off (skip it) with a capped, self-healing cooldown.
- Make unhealthy boards visible (run summary + a queryable table).

**Non-Goals:**
- No daemon, no queue, no River — cron and its cadence are untouched.
- No catalog move — boards + cadence stay in YAML/git; the table is runtime state only.
- No change to how a board crawls, normalizes, upserts, or to the stale sweep.
- No admin/API surface in this slice (a read endpoint is Slice-0.5).

## Decisions

### D1. `board_health` is a runtime-state sidecar keyed by `(provider, board)`

```
board_health(
  provider text, board text,           -- PK; boardless/aggregator entries use board=''
  consecutive_failures int not null default 0,
  cooldown_until timestamptz,           -- null = eligible
  last_error text, last_error_at timestamptz,
  last_success_at timestamptz, last_ingested_count int,
  last_run_at timestamptz,
  PRIMARY KEY (provider, board)
)
```

The identity mirrors the crawl unit (`CompanyEntry.Provider` + `Board`), the same key
`jobIdentity` derives from. No catalog columns (company, cadence, enabled) — those live
in YAML. A row is created lazily on first crawl and is inert if its board later leaves
the YAML (the run only ever reads rows for boards it is about to crawl).

*Alternative considered:* reuse a generic `worker_runs` table for all workers — rejected
as premature; this slice is board-scoped, and a unified table is part of the deferred
orchestration slice, not now.

### D2. The Runner owns the decision; an injected port owns persistence

`pipeline.Runner` gains an optional `BoardHealth` port (nil = disabled, so unit tests
and non-DB callers are unaffected, mirroring the optional `closer`):

```
type BoardHealth interface {
    Cooldown(ctx, provider, board string) (time.Time, bool, error)   // is it cooled, until when
    RecordSuccess(ctx, provider, board string, ingested int) error
    RecordFailure(ctx, provider, board, errMsg string) error         // computes next cooldown
}
```

`ingestBoard` calls `Cooldown` **before** the adapter lookup/fetch; if cooled, it logs
once, counts the board as *cooled* (a new `RunStats` field, distinct from `Failed`), and
returns without crawling. Otherwise it crawls and calls `RecordSuccess` / `RecordFailure`.
The `QueriesBoardHealth` adapter over `*db.Queries` lives in `cmd/ingest` (like `dbStore`).

*Alternative:* have `cmd/ingest` wrap each board — rejected; the skip decision must gate
the adapter call inside the Runner's per-board loop, so the port belongs to the Runner.

### D3. Backoff: threshold 3, exponential from 6h, capped at 24h

The hourly cron is the natural retry, so the first two failures apply **no** cooldown.
From the third consecutive failure:

```
consecutive_failures < 3   → no cooldown (eligible next run)
consecutive_failures >= 3  → cooldown_until = now + min(24h, 6h * 2^(f-3))
      f=3 → 6h    f=4 → 12h    f=5 → 24h    f>=6 → 24h (capped)
```

A success resets `f` to 0 and clears `cooldown_until` (self-heal). Cooldown is never
permanent — even a chronically dead board retries once every 24h, so a fixed board
recovers on its own. The policy is a small pure function, unit-tested against the table
above (no DB needed).

*Alternative:* dead-letter (disable) after N failures — rejected for Slice-0; auto-disable
needs a re-enable surface (Slice-0.5's endpoint). A capped 24h cooldown is self-healing
without any human action, which is the right default before there is an admin UI.

### D4. Failure classification: only board-level fetch/registry failures count

`consecutive_failures` counts an **unknown provider** or a **Fetch error** — a whole-board
failure. It does NOT count per-job save skips (those are already `stats.Skipped` and are
usually transient/partial), nor a streaming board's mid-crawl error that still saved some
jobs (partial progress is a success signal, not a board outage). This keeps cooldown a
signal of "this board is unreachable/misconfigured", not "one posting failed to save".

## Risks / Trade-offs

- **[A board wrongly cooled by a transient outage]** → Mitigated by the threshold (3
  consecutive, i.e. ~3h of hourly failures before any cooldown) and self-heal on the next
  success. A one-off blip never cools a board.
- **[Concurrent runs / concurrent boards racing the upsert]** → Rows are per-board and
  independent; the upsert is `ON CONFLICT (provider, board) DO UPDATE`, so concurrent
  board crawls touch disjoint rows and a rare overlapping run is idempotent.
- **[Migration not applied before deploy]** → New queries reference a missing table →
  ingest errors. Mitigated by the deploy convention: apply the migration to prod PG18
  manually BEFORE the binary ships (documented in tasks).
- **[Stale rows for removed boards]** → Inert (never read for a non-catalog board); an
  optional prune is out of scope.

## Migration Plan

1. Add the `board_health` migration + sqlc queries; regenerate `internal/db`.
2. Add the `BoardHealth` port + backoff function in `internal/pipeline` with unit tests
   (nil port = today's behavior, proven by the existing Runner tests staying green).
3. Wire cooldown-skip + outcome recording into `ingestBoard` (and the streaming path's
   board-level outcome); add the `Cooled` stat.
4. Implement `QueriesBoardHealth` in `cmd/ingest` + the per-run unhealthy summary log.
5. Apply the migration to prod PG18 manually, then deploy.

**Rollback:** the port is optional — reverting the `cmd/ingest` wiring (nil port) restores
exact prior behavior while leaving the harmless table in place.

## Open Questions

- Threshold/cap tuning (3 / 6h / 24h) is a first guess — revisit once real failure data
  from the `board_health` table shows the actual distribution of flaky vs dead boards.
- Should the streaming path (`ingestStream`) treat a board-level `FetchStream` error that
  saved zero jobs as a failure for cooldown? Leaning yes (zero-progress = outage);
  confirm against `ingestStream` during task 3.
