## 1. Shared worker package (foundation, fully unit-tested)

- [x] 1.1 Add `internal/worker` with `exitCode(failed, deadLettered int) int` (or a `RunOutcome` predicate): RED a test asserting clean→0 and any failure/dead-letter→non-zero, then implement.
- [x] 1.2 Add `worker.Bootstrap(parent) (ctx, cfg, pool, cleanup, err)` using `signal.NotifyContext(SIGINT,SIGTERM)` + `database.Connect`; `cleanup` stops the signal notify and closes the pool. Test: context cancels when the bound signal fires, and `cleanup` releases the notification (no `os.Exit` in the tested path).

## 2. Surface swallowed bookkeeping failures

- [x] 2.1 `internal/enrich/runner.go` `fail()`: the `store.Fail` error is already counted as a failure (it falls through to `stats.Failed++`), so the real gap is observability — its cause is never logged. Add a `log.Printf` for the `store.Fail` error cause so a bookkeeping outage is diagnosable. No TDD test for the log line (consistent with the runner's other `log.Printf` observability calls); verify via `go build`/`go vet` and the existing runner tests staying green.

## 3. Migrate failure-counting workers (exit non-zero on Failed/DeadLettered)

- [x] 3.1 `cmd/enrich`: adopt `worker.Bootstrap`, move logic into `run() int`, map `stats.Failed`/`stats.DeadLettered` through `exitCode`, `main` calls `os.Exit(run())`. Preserve provider setup and existing logs.
- [x] 3.2 `cmd/ingest`: same shape; map `runStats.Total().Failed` (and any sweep error) through `exitCode`; keep the per-provider stale-sweep intact and pass the signal context into `Run`/`CloseUnseenJobs`.
- [x] 3.3 `cmd/tg-extract`: adopt bootstrap + `run()/os.Exit`; surface `stats.Failed` via `exitCode`.
- [x] 3.4 `cmd/tg-ingest`: adopt bootstrap + `run()/os.Exit`; surface `stats.Failed` via `exitCode`; keep the channels.yml load/validate fail-fast.
- [x] 3.5 `cmd/liveness`: adopt bootstrap + `run()/os.Exit`; surface per-probe DB failures as a non-zero outcome (count them) instead of logging-and-continuing-to-exit-0.

## 4. Migrate maintenance workers (bootstrap + signals; exit non-zero only on hard error)

> Note: origin/main (PR#106) consolidated the three `backfill-{geo,skills,class}`
> binaries into a single `cmd/backfill-derive`, so the maintenance set is
> reindex / reslug / backfill-derive.

- [x] 4.1 `cmd/reindex`: adopt `worker.Bootstrap`; keep its existing fatal-on-error paths as non-zero exits; propagate the signal context.
- [x] 4.2 `cmd/reslug`: same.
- [x] 4.3 `cmd/backfill-derive`: same; ensure the keyset-pagination loop honours context cancellation (keep its existing `main_test.go` green).

## 5. Align server + verify

- [x] 5.1 `cmd/server`: server is a long-lived process (not a run-once worker), so it does NOT use `worker.Bootstrap` (which would add a second, conflicting signal handler). Instead unify its signal handling on one `signal.NotifyContext`: the same signal-bound context cancels the pool connect AND triggers shutdown (replacing the separate quit-channel), behavior-identical (wait for signal → `ShutdownWithTimeout`).
- [x] 5.2 Full verification: `go build ./...`, `go vet ./...`, `go test ./...`, and `go test -tags=integration -run XXNONE ./internal/...` to recompile integration-tagged tests; confirm green.
