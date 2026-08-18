# Worker pause switch: a Redis kill switch for the cron fleet

Date: 2026-08-18
Status: approved, not implemented

## Why

host-2 is disk-bound, not CPU-bound. Measured on 2026-08-18 at 04:15 UTC while a full
Meilisearch rebuild was running:

```
io  full avg60 = 28%
cpu full avg60 =  0%
```

Read attribution over a 20-second window put the ingest backends at ~56 MB/s of ~212 MB/s
total — roughly a quarter of all disk reads, and more than three times what the nightly
`pg_dump` was taking. When a catalogue-wide backfill, a reindex, and the ingest fleet
overlap, the only lever an operator has today is to stop ~140 systemd timers by hand and
remember to start them again.

That lever is bad in the specific way that matters: it is slow to pull under pressure, and
it leaves state on the host that no one can see from the dashboard. A stopped timer looks
exactly like a healthy timer that has not fired yet.

This change adds one switch that sheds load from the whole cron fleet in a single command,
carries its own expiry, and is visible in Prometheus while it is held.

## What already exists

Do not rebuild any of this.

| Component | Location |
|---|---|
| Single worker entrypoint | `internal/worker/main.go` — `Main(run func() int)` wraps every one of the ~40 `cmd/` binaries |
| Shared setup | `internal/worker/bootstrap.go` — `Bootstrap` does Sentry init, `config.Load()`, signal context, pgx pool |
| Redis client and URL | `github.com/redis/go-redis/v9`; `config.Settings.RedisURL`, defaulting to `redis://localhost:6379/0` (`internal/config/config.go:325`) |
| Per-run metrics | `internal/worker/metrics.go` — `writeRunMetrics` publishes `freehire_worker_last_run_{timestamp_seconds,duration_seconds,success}` into `PROM_TEXTFILE_DIR` |
| Worker identity | `filepath.Base(os.Args[0])` for the job label; `runInstance(os.Args[1:])` for the per-board instance label |
| Atomic textfile publish | `worker.WriteTextfile` — write-then-rename |
| Ingest concurrency cap | `/opt/freehire/bin/ingest-slot.sh`, a counting `flock` semaphore, `INGEST_SLOTS=10` |

The ingest semaphore already bounds how many boards crawl at once. It cannot express "stop",
and it only governs ingest. This change is the orthogonal control: not *how many*, but
*whether at all*, across every worker.

## Scope

In scope: refusing to *start* a worker run.

Out of scope: stopping a run already in flight. A heavy ingest board runs up to ~50 minutes,
so the switch does not reclaim the host instantly — it stops the bleeding and lets the
current runs drain. Cancelling live work would mean trusting ~40 workers to unwind a
mid-run context correctly, which is a much larger claim to make and to test. The seam is
noted, not built: `Bootstrap` already derives a SIGINT/SIGTERM-cancellable context, so a
future poll-and-cancel has an obvious place to attach.

## Design

### Where the check lives

In `Main`, before `run()` is called — not in `Bootstrap`.

```go
func Main(run func() int) {
    defer capturePanic()
    if paused(workerJob()) {
        writePausedMetrics()
        os.Exit(0)
    }
    start := time.Now()
    code := run()
    writeRunMetrics(time.Since(start), code)
    os.Exit(code)
}
```

`Bootstrap` is the wrong place because it reports failure by returning an error, and every
worker answers an error with `return 1`. A pause is not a failure; surfacing it as one would
make every held switch page someone.

Putting it in `Main` also means a paused run costs exactly one Redis `GET`. Sentry is never
initialized and the pgx pool is never opened.

### Keys

Two keys are consulted, and either one holds the worker:

```
freehire:pause:all         # every worker
freehire:pause:<job>       # one binary, e.g. freehire:pause:ingest
```

`<job>` is the binary name — the same string already used as the `job` metric label. The
per-board `instance` label is deliberately *not* part of the key: pausing a single ingest
board is a job for that board's timer, not for an incident switch.

Presence is the signal; the value is ignored. Callers are expected to set a TTL:

```bash
# shed the whole fleet for three hours
redis-cli SET freehire:pause:all 1 EX 10800

# or just the loudest tenant
redis-cli SET freehire:pause:ingest 1 EX 10800

# lift early
redis-cli DEL freehire:pause:all
```

The TTL is a convention, not an enforcement. `SET` without `EX` is accepted — refusing it
would mean the switch could fail at the moment it is most needed. The forgotten-switch case
is handled by making the pause visible (below) rather than by making it un-settable.

### Manual override

`FREEHIRE_IGNORE_PAUSE=1` in the environment skips the check entirely.

systemd timer units do not carry it, so the override only admits what a human started by
hand. This is the operating mode the switch exists for: hold the fleet, then run one thing
under a quiet host.

```bash
redis-cli SET freehire:pause:all 1 EX 10800
FREEHIRE_IGNORE_PAUSE=1 /opt/freehire/src/hire-current/backfill-derive
```

### Redis unreachable means run

The check fails open. A Redis error, a timeout, or an unparseable URL logs one line and the
worker proceeds with its normal run.

The inverse would convert an optional convenience into a single point of failure for the
entire catalogue: one Redis blip and all ~40 workers stop, silently, with the dashboard
reporting clean exits. `cmd/server` may depend on Redis hard — there it backs rate limiting,
without which the API must not serve — but nothing about shedding load justifies that
coupling.

The check takes a short timeout (250ms) so an unhealthy Redis delays a worker by a bounded
amount rather than by its dial default.

### Metric

One file per worker, as today. `RunMetricsFilename()` is unchanged, so no second textfile is
introduced and the "one worker, one filename" rule in `internal/worker/AGENTS.md` still
holds.

| Run | File contents |
|---|---|
| Normal | the `last_run_*` triple, plus `freehire_worker_paused{...} 0` |
| Paused | `freehire_worker_paused{...} 1` alone |

While a pause is held, `freehire_worker_last_run_timestamp_seconds` stops advancing and ages.
Any age-based rule eventually fires — and that is the intended safety net for a switch
someone forgot to lift. `freehire_worker_paused` sitting at 1 beside it is what tells the
person paged that the silence was deliberate.

This deliberately rejects the alternative of exiting 0 and stamping a successful run.
`freehire-reindexw` already demonstrated that failure mode on this host: a skipped cycle
recorded as a success kept the dashboard green while the index went stale for days.

Both gauges carry the existing `job` and `instance` labels, and both are therefore subject
to the documented `exported_job` rename by node_exporter's textfile collector. Any query
must use `exported_job`, not `job`.

### Out-of-repo work

`freehire_worker_paused` becomes part of the metric-name contract with the Grafana dashboard
and alert rules in `freehire-ops`, which cannot be compiled against this repository. Adding
the gauge here does not make it visible there. The ops-side change is:

1. Surface `freehire_worker_paused` on the worker panel.
2. Add the paused state to the existing worker-staleness rule's annotation, so a page for a
   stale worker says whether a switch is held.

Treat this as part of shipping the feature, not as a follow-up.

## Components

| Unit | Responsibility | Depends on |
|---|---|---|
| `internal/worker/pause.go` | Decide whether this process may run: read the two keys, honour the override, fail open | `config.Settings.RedisURL`, `go-redis` |
| `internal/worker/metrics.go` | Publish `freehire_worker_paused` alongside the existing triple | `WriteTextfile` |
| `internal/worker/main.go` | Wire the decision in before `run()` | the two above |

The pause decision is a pure function of (job name, override env, Redis contents), so it can
be tested against `miniredis` — already a dependency — without a live Redis or a worker.

## Testing

Unit, against `miniredis`:

- No keys set: the worker runs.
- `freehire:pause:all` set: the worker is held.
- `freehire:pause:ingest` set: `ingest` is held, `search-drain` runs.
- `FREEHIRE_IGNORE_PAUSE=1` with `freehire:pause:all` set: the worker runs.
- Redis unreachable: the worker runs, and the failure is logged.
- Redis slow past the timeout: the worker runs.

Metrics, against a temp directory:

- A paused run writes `freehire_worker_paused 1` and no `last_run_*` series.
- A normal run writes the `last_run_*` triple and `freehire_worker_paused 0`.
- Both land in `RunMetricsFilename()`, with the exposition text pinned the way
  `cmd/queue-metrics/render_test.go` pins its own — a rename must be a visible edit.

## Risks

**A held switch is invisible outside Prometheus.** Someone reading `systemctl list-timers`
sees a healthy fleet that is doing nothing. Mitigated by the paused gauge; not eliminated.
The `EX` convention is the practical defence.

**Load is shed with a lag.** In-flight runs finish. On a host where the heaviest ingest
boards run ~50 minutes, the switch is a way to stop the queue from refilling, not a way to
free the disk in the next minute.

## Related

- `internal/worker/AGENTS.md` — the metrics filename rule and the `exported_job` trap
- `docs/superpowers/specs/2026-08-15-ingest-observability-design.md` — the queue-depth
  metrics this gauge sits beside
