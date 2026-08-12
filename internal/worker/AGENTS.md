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
- **`ResilientPage` degrades on TOAST corruption, never on anything else** (resilient.go:95-
  135). One row's damaged TOAST fails the whole `SELECT *` (Postgres XX001); the scanner
  then re-lists the same window as bare ids (never detoasts, so it cannot fault) and fetches
  rows individually, collecting the readable ones and logging-skipping the corrupted.
  Non-corruption errors always propagate unchanged. The keyset cursor advances past a
  skipped row, so the scan never loops on it; a row that vanished mid-window (`ErrNoRows`)
  is skipped too, staying symmetric with the fast path.
- **Two reader constructors, two query shapes** (resilient.go:21-42): `NewFullScanReader`
  keysets over the whole `jobs` table; `NewPostedSinceReader` keysets over OPEN jobs in a
  freshness window (`COALESCE(posted_at, created_at) >= since`, the
  `reindex --semantic --posted-within` shape). Each takes its own narrow query interface so
  a worker with its own store satisfies one without declaring methods the reader never
  calls.

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
