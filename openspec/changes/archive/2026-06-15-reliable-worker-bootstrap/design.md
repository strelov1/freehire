## Context

Ten standalone workers under `cmd/` (`ingest`, `enrich`, `liveness`, `tg-ingest`,
`tg-extract`, `reindex`, `reslug`, `backfill-geo`, `backfill-skills`,
`backfill-class`) each open with the same block:

```go
cfg := config.Load()
ctx := context.Background()
pool, err := database.Connect(ctx, cfg.DatabaseURL)
if err != nil { log.Fatalf("database: %v", err) }
defer pool.Close()
```

They then call a package `Runner.Run(ctx, ...)` (or a maintenance loop) that
returns a stats struct (`Enriched/Failed/DeadLettered`, `RunStats`, etc.) plus an
error that is only non-nil on a hard abort (context cancellation, a fatal sweep
error). Per-item failures are counted into stats but never reach the exit code,
so the worker logs the counts and returns `0`. `cmd/server` already does
signal-based graceful shutdown and is the reference for the pattern, but even it
connects the pool on `context.Background()`.

The audit also found `internal/enrich/runner.go:188` discards the error from the
queue's `Fail` call entirely (neither logged nor counted).

## Goals / Non-Goals

**Goals:**
- One shared bootstrap helper: config + pool + signal-bound context + cleanup.
- A uniform, testable exit-code contract: non-zero when a run finished with any
  `Failed`/`DeadLettered` > 0; zero otherwise.
- Graceful cancellation: the root context cancels on SIGINT/SIGTERM and is
  propagated into each worker's run/sweep calls.
- Fix the swallowed `store.Fail` error so bookkeeping failures count.

**Non-Goals:**
- No change to the run-once-and-exit cadence, cron entries, flags, or env vars.
- No change to per-item failure isolation (one bad board/post still must not
  abort the rest).
- No retry/backoff or alerting logic beyond the exit code itself.
- Not touching the domain meaning of enrichment/lifecycle (those specs are
  unchanged); this is purely worker process behavior.

## Decisions

### Decision 1: A small `internal/worker` bootstrap package

Add `internal/worker` with a single entry point, roughly:

```go
// Bootstrap loads config, opens the pool, and returns a context that is
// cancelled on SIGINT/SIGTERM. cleanup closes the pool and stops the signal
// notification; call it with defer.
func Bootstrap(parent context.Context) (ctx context.Context, cfg config.Config, pool *pgxpool.Pool, cleanup func(), err error)
```

Internally it uses `signal.NotifyContext(parent, syscall.SIGINT, syscall.SIGTERM)`
(stdlib, already used conceptually by `cmd/server`) and `database.Connect`.
`cleanup` runs `stop()` then `pool.Close()`.

**Why over alternatives:** a free function returning a cleanup closure matches the
existing thin-`main` style and is trivially used with `defer`. A struct/`Run`-
callback wrapper (passing the whole worker body into the helper) was considered
but would force every `main` into a callback shape and obscure each worker's
distinct post-run logic (ingest's per-provider sweep, enrich's provider setup).
Keeping `main` linear keeps diffs small and per-worker logic visible.

### Decision 2: `run() (stats, error)` + thin `main` for testable exit codes

`os.Exit` is untestable, so the exit-code decision must live in a function a test
can call. Each worker keeps its logic in a helper that returns its stats (already
true — `runner.Run` returns stats); `main` becomes:

```go
func main() {
    code := run()
    os.Exit(code)
}
```

where `run()` does the bootstrap + work and maps the outcome to a code via one
shared helper:

```go
// exitCode returns 1 when a run finished with any failures or dead-letters.
func exitCode(failed, deadLettered int) int
```

This `exitCode` (or a `RunOutcome` with a `Failed()` predicate) lives in
`internal/worker` and is unit-tested directly: clean → 0, any failure → non-zero.
Workers that already `log.Fatalf` on bootstrap errors keep failing fast (exit !=
0), so only the *successful-Run-with-failures* path is new.

**Why over alternatives:** returning a richer error from `Runner.Run` when stats
show failures was considered, but that conflates "the run aborted" with "the run
completed but some items failed" — callers (and the existing logs) treat those
differently. Keeping stats as the signal and mapping to a code in one place is
clearer and leaves `Runner.Run`'s contract intact.

### Decision 3: Fix `runner.go:188` by counting the `Fail` error

The swallowed `Fail` error becomes a counted failure (increment the run's failure
tally and continue the drain — the per-item isolation rule still holds). This
makes the bookkeeping failure visible both in the logged stats and, via Decision
2, in the exit code.

## Risks / Trade-offs

- **[Cron entries that ignored exit codes now surface failures]** → Intended.
  Operators may suddenly see alerts that were previously masked; documented as a
  behavior change in the proposal Impact. No silent regression — a degraded run
  was already degraded.
- **[A worker that legitimately has nonzero `Failed` every run would always
  alert]** → Per-item failures are real failures by the audit's definition; if a
  source is chronically failing that is exactly what should alert. No threshold
  tuning in this change (out of scope; can follow later).
- **[Touching ten `main.go` files at once]** → Mitigated by doing the shared
  helper first with its own tests, then migrating workers one at a time under the
  per-task TDD loop, each verified by `go build ./...` + `go vet`.
- **[Signal context cancels mid-write]** → That is the desired behavior; existing
  DB calls already take a context and pgx propagates cancellation. Maintenance
  backfills use transactions that roll back on a cancelled context.

## Migration Plan

1. Land `internal/worker` (bootstrap + `exitCode`) with unit tests — no behavior
   change yet.
2. Migrate each worker `main.go` to use the helper + `run()/os.Exit` shape, one
   per task, keeping its post-run logic intact.
3. Fix `internal/enrich/runner.go` to count the `Fail` error.
4. No deploy ordering constraints (no schema/API change); ship normally. Rollback
   is a plain revert — no data migration involved.

## Open Questions

- Should maintenance backfills (`reindex`/`reslug`/`backfill-*`) that have no
  natural "Failed" counter also adopt the helper purely for the signal context +
  bootstrap dedup? **Proposed:** yes for bootstrap/signals (they benefit from
  graceful cancellation on long runs), and they exit non-zero only on a hard
  error (their existing `log.Fatalf` paths), since they have no per-item failure
  tally to surface.
