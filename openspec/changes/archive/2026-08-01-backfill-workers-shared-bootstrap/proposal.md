## Why

`internal/worker` is one of this repo's genuinely successful shared seams: 33 of the 34
production cron binaries call `worker.Main` + `worker.Bootstrap`, and per-main boilerplate is
about eight lines. The exceptions are not the harmless ones. Of every binary under `cmd/` that
opens a database pool, exactly three sit outside the shared bootstrap — `cmd/server`, which is
a long-lived daemon with its own lifecycle, and the two backfills that **spend LLM money on
stored user CVs**:

- `cmd/backfill-experience`
- `cmd/backfill-resume-structured`

Neither calls `observability.Init`, so a panic in either is invisible to Sentry — the exact gap
`worker.Bootstrap` exists to close, and one that `error-tracking`'s "Worker error capture with
guaranteed delivery" already requires them to close. (Run-ending *errors* are still only logged
here as everywhere else: no worker in the fleet calls `sentry.CaptureException`. This change
brings these two to parity with the other 34, it does not raise the fleet's bar.)

The consequences are not theoretical:

- `cmd/backfill-experience/main.go:136` calls `os.Exit(1)` with `defer pool.Close()` and the
  Langfuse `defer flush()` still pending. `os.Exit` runs no deferred function, so the trace
  flush is skipped **precisely on the partially-failed run** — the only run whose traces would
  explain what happened.
- `cmd/backfill-resume-structured/main.go:161` logs its failure tally and returns normally, so
  the process exits `0` even when every user failed. `worker-lifecycle`'s "Run outcome reported
  through exit code" says it must not.
- Both derive their root context from `context.Background()`, so a redeploy's or cron timeout's
  `SIGTERM` hard-kills an in-flight LLM call instead of cancelling it.

Three existing requirements across two specs already forbid all of this. They were held by
prose, and prose did not hold: the two workers drifted out of compliance without anything
failing. So this change also makes the rule checkable.

## What Changes

- Both mains become `worker.Main(run)` + `run() int` built on `worker.Bootstrap`, replacing the
  hand-rolled `config.Load` / `context.Background` / `database.Connect` / `defer pool.Close()`.
- Every `os.Exit(1)` and `log.Fatalf` in the two workers becomes `return 1` from `run`, so the
  deferred pool close and the Langfuse flush actually run — including on the failed run.
- `cmd/backfill-resume-structured` reports its outcome through `worker.ExitCode(failed, 0)`
  instead of always exiting `0`.
- A guard test makes the bootstrap rule mechanical: every binary under `cmd/` that opens a
  database pool must reach it through `worker.Bootstrap`, with `cmd/server` the single declared
  exception. The list of exceptions becomes readable data instead of an unwritten assumption.

Explicitly NOT changed: the two workers' résumé-extractor construction chains, which look
duplicated and are not. `backfill-experience` treats every piece as optional (missing S3 or LLM
degrades to the free pass over users with a current structure); `backfill-resume-structured`
treats every piece as fatal (a run without the PII detector would be a no-op, so it refuses to
start). Those are opposite policies, both documented in place, and merging them would break one.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `worker-lifecycle`: the "Shared worker bootstrap" requirement gains an explicit scope (which
  binaries it binds and which are exempt) and the requirement that compliance is enforced
  mechanically rather than by convention.

`error-tracking`'s "Worker error capture with guaranteed delivery" and `worker-lifecycle`'s
"Run outcome reported through exit code" need no text change — the two workers simply start
complying with what they already say.

## Impact

- `cmd/backfill-experience/main.go`, `cmd/backfill-resume-structured/main.go` — rewired; their
  per-user logic, flags, idempotence and log lines are untouched.
- `internal/worker/` — gains the guard test. No production code change in the package.
- No schema, API, index or wire change. No migration. Both workers stay idempotent, so a
  partially-completed pre-change run needs no reconciliation.
