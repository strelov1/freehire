## Context

host-2 runs the API, Meilisearch, Postgres, and a cron fleet of ~40 run-once worker binaries
(~140 timer units once per-board ingest is counted) on one bare-metal box. The host is
disk-bound: measured 2026-08-18 during a full reindex, `io full avg60 = 28%` against
`cpu full = 0%`, with per-process read attribution putting the ingest backends at ~56 MB/s of
~212 MB/s total.

Two load-shedding mechanisms already exist and neither fits:

- `/opt/freehire/bin/ingest-slot.sh` is a counting `flock` semaphore (`INGEST_SLOTS=10`). It
  bounds *how many* boards crawl at once and governs ingest only. It cannot express "none".
- `systemctl stop <unit>.timer` works, but there are ~140 of them, it must be undone by hand,
  and a stopped timer is indistinguishable from one that simply has not fired.

The seam that makes a single switch cheap already exists: `worker.Main(run func() int)` in
`internal/worker/main.go` wraps every worker. `internal/worker/AGENTS.md` records it as the
one entry wrapper, and a spec requirement already enforces mechanically that no `cmd/` binary
opening a pool bypasses the shared bootstrap.

Redis is already a hard dependency of `cmd/server` (rate limiting, response cache) and
`config.Settings.RedisURL` is loaded for every binary, defaulting to
`redis://localhost:6379/0`. `go-redis/v9` and `miniredis/v2` are both already in `go.mod`.

## Goals / Non-Goals

**Goals:**

- Shed load from the entire cron fleet, or from one binary, with a single command.
- Let an operator run one worker by hand while the fleet is held — the workflow that prompted
  this: hold everything, then run `backfill-derive` against a quiet host.
- Make a held switch visible in the monitoring that already exists, and make a forgotten
  switch eventually page someone.
- Cost nothing on the normal path: one Redis `GET` before any pool is opened.

**Non-Goals:**

- Cancelling a run already in flight. The heaviest ingest boards run ~50 minutes, so the
  switch stops the queue refilling rather than freeing the disk immediately.
- Per-board granularity. Holding one ingest board is a job for that board's timer.
- Replacing `ingest-slot.sh`. That answers "how many"; this answers "whether at all". They are
  orthogonal and both stay.
- Any enforcement that a pause carries an expiry.

## Decisions

### The gate lives in `Main`, not `Bootstrap`

`Bootstrap` reports failure by returning an error, and every worker answers an error with
`return 1`. Routing a pause through it would make each held switch look like a failed run and
page someone. `Main` can exit on its own terms.

Placing it in `Main` also means a paused run never initializes Sentry and never opens the pgx
pool — the cost of a held switch is one `GET`.

*Alternative considered:* a sentinel error from `Bootstrap` that workers recognize. Rejected —
it needs an edit in all ~40 workers and one missed worker fails open in the wrong direction,
silently running while the fleet is held.

### Two keys, presence-as-signal

```
freehire:pause:all       # the whole fleet
freehire:pause:<binary>  # one binary, e.g. freehire:pause:ingest
```

Either key present holds the worker. `<binary>` is `filepath.Base(os.Args[0])` — the same
string already used as the `job` metric label, so the key an operator types matches the label
they read on the dashboard.

The per-invocation `instance` label (cmd/ingest's board file) is deliberately excluded from
the key. Holding one board is a timer's job; an incident switch that needs 140 keys to quiet
ingest is not a switch.

The stored value is ignored, so `SET ... 1` and `SET ... "reindex running"` behave the same
and an operator can leave themselves a note.

*Alternative considered:* a Redis set of paused worker names. Rejected — a set cannot carry a
per-entry TTL, which is exactly the property that makes a forgotten switch self-healing.

### TTL is a convention, not an enforcement

`SET freehire:pause:all 1 EX 10800` is the documented form, but a key without `EX` is honoured.
Refusing it would mean the switch can fail at the moment it is most needed. The forgotten-switch
case is handled by visibility (below), not by making the switch harder to set.

### Override by environment variable

`FREEHIRE_IGNORE_PAUSE=1` skips the check. systemd timer units do not carry it, so the bypass
admits only a hand-started run.

*Alternative considered:* a Redis exception key (`freehire:pause:except`). Rejected — it puts
two keys with independent TTLs into a state that must be kept consistent, and the exception
outliving the pause is a silent, confusing failure. An environment variable holds no state
between invocations, so there is nothing to forget to clean up.

### Fail open

A Redis error, a malformed URL, or a response slower than a 250ms timeout logs one line and
the worker runs.

The inverse would turn an optional convenience into a single point of failure for the whole
catalogue: one Redis blip stops all ~40 workers, silently, with the dashboard reporting clean
exits. `cmd/server` depends on Redis hard because rate limiting is load-bearing for serving;
nothing about shedding background load justifies the same coupling.

The bounded timeout matters as much as the fallback: an unhealthy-but-reachable Redis would
otherwise delay every worker start by the dial default.

### Metrics: one file, a paused gauge, and a deliberately stale timestamp

`RunMetricsFilename()` is unchanged — the "one worker, one filename" rule in
`internal/worker/AGENTS.md` exists because `Main` writes that file *last*, so a second
publisher would be silently overwritten. Only the contents differ:

| Run | Contents |
|---|---|
| Completed | the `freehire_worker_last_run_*` triple, plus `freehire_worker_paused 0` |
| Refused | `freehire_worker_paused 1` alone |

While a pause is held the last-run timestamp ages, so any age-based staleness rule eventually
fires — the safety net for a switch nobody lifted — and `freehire_worker_paused 1` beside it
tells whoever is paged that the silence was deliberate.

*Alternative considered:* exit zero and stamp a successful run. Rejected on evidence from this
host: `freehire-reindexw` recorded skipped cycles as successes, which kept the dashboard green
while the search index went stale for days.

Both gauges carry the existing `job` and `instance` labels and are therefore subject to
node_exporter's documented `exported_job` rename. Queries must use `exported_job`.

### Testability

The decision is a pure function of (binary name, override env, Redis contents), so it lives in
its own file and is tested against `miniredis` — no live Redis, no worker process.

## Risks / Trade-offs

- **A held switch is invisible outside Prometheus** → `systemctl list-timers` shows a healthy
  fleet doing nothing. Mitigated by the paused gauge and by the documented `EX` convention;
  not eliminated.
- **Load sheds with a lag** → in-flight runs finish, up to ~50 minutes for the heaviest ingest
  boards. Accepted; cancelling live work is a much larger claim to make across 40 workers and
  is explicitly out of scope.
- **A new metric name is a cross-repo contract** → `freehire_worker_paused` must be surfaced in
  the Grafana dashboard and alert annotations in `freehire-ops`, which cannot be compiled
  against this repo, so nothing here catches the omission. Treated as part of shipping, and
  the exposition text is pinned by test so a rename is a visible edit.
- **Redis becomes a dependency of every worker start** → mitigated by the fail-open rule and
  the 250ms bound, which together cap the worst case at a quarter-second of added start
  latency and no behavioural change.

## Migration Plan

No schema change, no data migration, no API change. Deploy is the ordinary `release.sh` path.

Before deploy the feature is inert: no keys exist, so every worker takes the fail-open or
no-key path and runs exactly as today. Rollback is a redeploy of the previous binary; any key
left in Redis becomes meaningless rather than harmful, and expires on its own if it was set
with `EX`.

The `freehire-ops` dashboard and alert change ships alongside, not after.

## Open Questions

None. The four decisions that were genuinely open — how deep the pause cuts, its granularity,
how a single worker is excepted, and how a pause appears in monitoring — were settled with the
user during brainstorming and are recorded above with their rejected alternatives.
