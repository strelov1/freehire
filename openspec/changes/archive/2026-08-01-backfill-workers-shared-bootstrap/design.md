## Context

`worker.Main(run func() int)` defers a panic-capture-and-flush and then `os.Exit(run())`.
`worker.Bootstrap(parent)` returns `(ctx, cfg, pool, cleanup, error)` having already called
`observability.Init` and derived a `SIGINT`/`SIGTERM`-cancellable context. `cmd/recount-companies`
is the canonical 39-line consumer.

The two backfills predate none of this — they simply were not converted. Their current shape is:

```
cfg := config.Load()
ctx := context.Background()
pool, err := database.Connect(ctx, cfg.DatabaseURL)
if err != nil { log.Fatalf(...) }
defer pool.Close()
```

which is exactly what `Bootstrap` returns, minus Sentry and minus signal handling.

Measured, not assumed: of the 11 binaries under `cmd/` that do not call `worker.Bootstrap`,
only three open a database pool — these two and `cmd/server`. The other eight are code
generators and file-writing harvest tools. That is what makes the guard test cheap: the
exemption list is one entry, not ten.

## Goals / Non-Goals

**Goals:**

- Bring both workers under the shared bootstrap, so Sentry sees their failures and `SIGTERM`
  cancels their in-flight LLM calls.
- Make the deferred Langfuse flush actually run on the failed run.
- Give `backfill-resume-structured` an exit code that reflects its failure tally.
- Convert "every worker uses the bootstrap" from prose into a test.

**Non-Goals:**

- Merging the two workers' extractor construction. They encode opposite policies (optional vs
  fatal), both documented in place; a shared constructor would have to smuggle a flag to
  reproduce them, which is how one of the two ends up wrong.
- Touching per-user logic, flags, log lines, or idempotence.
- Replacing `backfill-resume-structured`'s hand-written eligibility SQL with sqlc. Real, and a
  separate concern from process lifecycle — mixing it in would hide this diff's point.
- Rewriting the other nine non-bootstrap binaries. They open no pool; the requirement does not
  bind them.

## Decisions

### `log.Fatalf` → `return 1`, not a wrapper

Every `log.Fatalf` becomes a `log.Printf` plus `return 1` from `run`. `log.Fatalf` calls
`os.Exit(1)` internally, which is precisely the defect: it runs no deferred function, so the
pool close and the Langfuse flush are skipped. The mechanical rule is that inside a
`worker.Main` worker, the only exit is a return value.

This matters most in `backfill-experience`, where the flush is registered at
`main.go:76` and the `os.Exit(1)` at `:136` is reached only when `failed > 0` — so the traces
are dropped on exactly the run that needed them.

In `backfill-resume-structured` the picture is subtler and worth stating: four of its nine
`log.Fatalf` calls fire *before* `defer llmFlush()` is registered, so those drop nothing today.
The remaining five do. Converting all nine is still right — it removes the need for anyone to
work out which is which.

### The guard test scans `cmd/`, and its exemption list is the documentation

A test in `internal/worker` walks `../../cmd/*/`, finds packages that reference
`database.Connect` or `pgxpool`, and asserts each also references `worker.Bootstrap`. `server`
is the declared exemption, with the reason in the literal.

**Alternative considered — assert on an explicit list of expected worker names.** Rejected:
that list would need editing whenever a worker is added, which is the failure mode of the thing
it replaces. Deriving the population from "opens a pool" means a new worker is enrolled by
existing.

**Alternative considered — no test, just fix the two.** Rejected because it is what happened
last time. The whole reason this change exists is that three written requirements did not stop
two workers drifting out of compliance.

The test reads source text rather than building an import graph. Coarser, but it depends on no
build tooling and runs in milliseconds. It parses each file and re-prints it without comments
before matching, because matching raw text would let a `// TODO: move this to worker.Bootstrap`
satisfy the check — exactly the comment someone deferring the migration would leave. What it
still cannot see is a pool obtained through a helper package that mentions neither
`database.Connect` nor `pgxpool`; the honest answer there is that the test is a ratchet against
the common case, not a proof.

The same walk also asserts the second half of the rule: a package that uses `worker.Bootstrap`
must contain no `log.Fatal` or `os.Exit`. Bootstrapping is not enough on its own — a worker
that bootstraps and then fatals still skips its deferred pool close and telemetry flush, which
is half the defect being fixed here. That assertion is green across the fleet today, so it
costs nothing and holds the line.

### `worker.ExitCode`, not a hand-written comparison

`backfill-resume-structured` ends on `worker.ExitCode(failed, 0)`. `backfill-experience`
already has the equivalent `if failed > 0` and switches to the same helper, so both read the
same and neither restates the convention.

## Risks / Trade-offs

- **`SIGTERM` now cancels in-flight work where it previously did not.** → That is the intended
  behaviour and matches every other worker. Both workers are idempotent by construction, so a
  cancelled run is safe to re-run; a partially-completed run needs no reconciliation. Each
  per-user loop checks `ctx.Err()` at the top and stops, following
  `cmd/backfill-applications/main.go:84` — without that the loop would run to the end of the
  list turning every remaining user into a logged failure, reporting one cancellation as a
  fleet of them. A cancelled run exits non-zero and says how many targets it left.
- **`backfill-resume-structured` starts exiting non-zero on partial failure**, so a cron entry
  that previously looked green may start alerting. → That is the requirement, and a silent
  partial failure on a worker that spends money is the worse outcome.
- **The guard test could go stale** if a future worker reaches the database through some path
  that mentions neither `database.Connect` nor `pgxpool`. → It would then be out of the test's
  population and silently pass. Accepted: the test is a ratchet against the common case, not a
  proof.
- **Sentry now receives errors from two workers that were silent.** → Intended; it is the
  headline of the change.

## Migration Plan

None. No schema, no data, no deploy ordering. Both binaries keep their flags
(`--user`, `--dry-run`) and their idempotence, so any in-flight operational runbook is
unaffected apart from the new exit code.

## Open Questions

None.
