## Why

The standalone cron workers (`cmd/ingest`, `cmd/enrich`, `cmd/liveness`, the
`tg-*` crawlers, and the `backfill-*`/`reindex`/`reslug` maintenance jobs) report
success to cron even when a run partially or wholly fails: a DB outage, a
fully-failed crawl, or an entire enrichment wave that dead-letters all exit `0`.
A code audit flagged this as the single highest-impact reliability gap — cron
alerting is effectively blind, so a silently-degrading pipeline looks healthy.
Two related defects compound it: the same ~80 lines of bootstrap are copy-pasted
across ten workers, and none of them handle `SIGINT`/`SIGTERM`, so a cron
timeout or redeploy hard-kills a worker mid-transaction.

## What Changes

- Introduce a single shared worker-bootstrap helper that every standalone worker
  uses to: load config, open the pgx pool, and obtain a root `context.Context`
  wired to `signal.NotifyContext(SIGINT, SIGTERM)` for graceful cancellation —
  replacing the duplicated `config.Load → context.Background → database.Connect →
  log.Fatalf → defer pool.Close` block in each `main`.
- Establish a uniform **exit-code contract**: a worker run that finishes with any
  per-item failures or dead-letters exits non-zero (so cron can alert), while a
  fully-clean run exits `0`. Run-once-and-exit shape and per-item failure
  isolation (one bad board/post never aborts the rest) stay intentional.
- Fix the swallowed `store.Fail` error in the enrichment runner
  (`internal/enrich/runner.go:188`) so a bookkeeping failure during the drain is
  surfaced (counted and propagated), not discarded.
- Propagate the signal-aware context through each worker's existing `Run`/sweep
  calls so in-flight DB work unwinds on shutdown instead of being killed.

## Capabilities

### New Capabilities
- `worker-lifecycle`: how standalone run-once-and-exit cron workers bootstrap
  (shared config/pool/context setup), handle termination signals (graceful
  cancellation via a signal-bound context), and report run outcome through the
  process exit code (non-zero on any failure or dead-letter).

### Modified Capabilities
<!-- None. The enrichment/lifecycle domain rules are unchanged; this change only
     governs worker process behavior (bootstrap, signals, exit codes). -->

## Impact

- **Code**: new `internal/worker` (or equivalent) bootstrap package; edits to all
  ten worker `main.go` files plus `cmd/server` (adopts the shared bootstrap where
  it overlaps); one error-propagation fix in `internal/enrich/runner.go`.
- **Operations**: cron jobs now receive non-zero exit codes on degraded runs —
  monitoring/alerting can finally trigger. No change to scheduling, flags, or the
  run-once cadence; existing cron entries keep working.
- **No API, DB schema, or wire-shape changes.** No new dependencies (uses stdlib
  `os/signal`, already used by `cmd/server`).
