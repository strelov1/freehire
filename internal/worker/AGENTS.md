# Worker conventions

## Scope
The shared bootstrap and run-outcome plumbing for the run-once-and-exit cron workers under
`cmd/` (~40+ binaries): config + pool + signal-bound context setup, Sentry panic capture,
the exit-code convention, a progress heartbeat, and a corruption-tolerant keyset scanner.

## Always true
- **`Main(run func() int)` wraps every worker's run, and Bootstrap's cleanup is deferred
  INSIDE run** (main.go:14-26). On the normal path Main exits with run's status after the
  deferred cleanup has flushed Sentry. On a panic, the deferred `capturePanic` reports it,
  flushes (bounded at 2s), and re-panics so the process still crashes with the original
  stack and a non-zero exit. A panic BEFORE Bootstrap (e.g. bad config) is not captured —
  those paths already log and exit non-zero.
- **`Bootstrap(parent)` is the one setup path** (bootstrap.go:29): Sentry init (a malformed
  DSN is fatal — broken observability must not boot silently), `config.Load()`, a
  SIGINT/SIGTERM-cancellable root context (a cron timeout or redeploy delivers SIGTERM), and
  the pgx pool. The returned cleanup stops the signal notification, closes the pool, and
  flushes Sentry; a failed step releases anything already acquired.
- **`ExitCode(failed, deadLettered)` is the convention** (exit.go:11): 0 on a clean run, 1
  when the run finished with ANY per-item failure or dead-lettered item, so cron alerts on
  a partially-failed run that would otherwise look successful. Deliberate divergence:
  `internal/applyform`'s `RunStats.Degraded` rejects this rule (hundreds of thousands of
  requests to other companies' APIs make a handful of transient failures the healthy
  shape) — see internal/applyform/AGENTS.md.
- **`Heartbeat(interval, report)` logs progress so a stall is visible** (heartbeat.go:11).
  `report` runs on a background goroutine CONCURRENTLY with the work — every counter it
  reads must be safe for concurrent access (atomic). The returned stop halts the goroutine;
  defer it.
- **`ResilientPage` degrades on TOAST corruption, never on anything else** (resilient.go:48-
  99). One row's damaged TOAST fails the whole `SELECT *` (Postgres XX001); the scanner
  then re-lists the same window as bare ids (never detoasts, so it cannot fault) and fetches
  rows individually, collecting the readable ones and logging-skipping the corrupted.
  Non-corruption errors always propagate unchanged. The keyset cursor advances past a
  skipped row, so the scan never loops on it; a row that vanished mid-window (`ErrNoRows`)
  is skipped too, staying symmetric with the fast path.
- **One reader constructor** (resilient.go:16-46): `NewFullScanReader` keysets over the
  whole `jobs` table via its own narrow `FullScanQueries` interface, so a worker with its
  own store satisfies it without declaring methods the reader never calls. A second
  constructor, `NewPostedSinceReader` (a freshness-windowed keyset over open jobs), existed
  only to feed `reindex --semantic --posted-within` and was removed alongside it
  (openspec/changes/drop-hybrid-search-pgvector-similar) — re-add the pattern if a future
  worker genuinely needs a scoped scan, rather than resurrecting the old one blind.

## Prometheus metrics

A run-once worker has no listener for Prometheus to scrape, so metrics go out as
node_exporter textfile-collector files in `PROM_TEXTFILE_DIR` (unset disables it). Two
kinds are published, and they answer different questions.

**Per run, automatically** — `Main` calls `writeRunMetrics` for every worker
(metrics.go:30), so `freehire_worker_last_run_{timestamp_seconds,duration_seconds,success}`
exist without any worker opting in. The `job` label is the binary name and `instance` is a
trailing board path if there is one, so ~140 ingest boards land in distinct series. This
answers *is the worker alive*: a hung run never stamps a completion, so last-run age
covers both "hung" and "timer stopped".

**Per pipeline, by `cmd/queue-metrics`** — `freehire_queue_{depth,dead_letters,oldest_age_seconds}`
labelled by `queue`, `freehire_boards_total` labelled by `state`, and
`freehire_catalogue_newest_job_timestamp_seconds`. This answers *is the worker keeping up*.
These names are a contract with the dashboard and alert rules in `freehire-ops`, which
cannot be compiled against this repo — `cmd/queue-metrics/render_test.go` pins the exact
exposition text so a rename is a visible edit rather than a silent breakage.

**One worker, one filename — and the run file is written LAST.** `Main` writes the run
outcome to `<binary>.prom` *after* `run()` returns (main.go:29-30). A worker that also
publishes its own textfile must therefore own a DIFFERENT name, or every run will emit
its payload and then overwrite it, leaving the collector with only the run outcome while
the setup still looks correct. `RunMetricsFilename()` exposes the name to compare
against; `cmd/queue-metrics` publishes as `freehire-pipeline.prom` and asserts the two
cannot converge.

**The `exported_job` trap.** node_exporter's textfile collector does not set
`honor_labels`, so Prometheus keeps its own `job="host2-node"` and renames a worker's
`job` label to `exported_job`. Any query against the per-run metrics must use
`exported_job` and `exported_instance`; querying `job` returns an empty vector with no
error, which reads exactly like a healthy silence. The queue metrics use `queue` and
`state` labels specifically to stay clear of the collision.

**Zero and absent differ.** A drained queue publishes `0`; an empty catalogue publishes
nothing. A missing series is how an alert rule recognizes a dead exporter, so a real
measurement of zero must never be omitted — while a zero *timestamp* would read as 1970,
i.e. an infinitely stale catalogue, which a fresh install is not.

## Usage sketch
Every `cmd/<worker>/main.go` is a variation on:

```go
func main() { worker.Main(run) }

func run() int {
    ctx, cfg, pool, cleanup, err := worker.Bootstrap(context.Background())
    if err != nil { log.Print(err); return 1 }
    defer cleanup()
    // ... work, then:
    return worker.ExitCode(failed, deadLettered)
}
```
