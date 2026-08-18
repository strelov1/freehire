## Why

host-2 is disk-bound, and the only way to shed load from the cron fleet today is to stop
~140 systemd timers by hand and remember to start them again. That is slow to do under
pressure and invisible afterwards: a stopped timer looks exactly like a healthy timer that
has not fired yet. Measured on 2026-08-18 during a full reindex, `io full avg60` sat at 28%
while `cpu full` sat at 0%, with the ingest backends taking ~56 MB/s of ~212 MB/s of reads —
so shedding ingest is the lever that matters, and there is no fast way to pull it.

## What Changes

- A worker consults a Redis pause switch before it runs, and exits without doing work when
  the switch is held. The check happens in the shared `worker.Main` wrapper, so it covers
  every one of the ~40 run-once binaries at once.
- Two key shapes: `freehire:pause:all` holds the whole fleet, `freehire:pause:<binary>` holds
  one worker. Either being present is enough.
- `FREEHIRE_IGNORE_PAUSE=1` in the environment bypasses the switch. systemd timer units do
  not carry it, so the bypass admits only what an operator started by hand.
- The check fails open: an unreachable or slow Redis logs one line and the worker runs.
- A held pause publishes `freehire_worker_paused` and stops refreshing the worker's
  `freehire_worker_last_run_*` series, so an age-based alert still fires on a switch nobody
  lifted.
- Not in scope: cancelling a run already in flight. In-flight work drains; only new starts
  are refused.

## Capabilities

### New Capabilities

None. This adds behaviour to an existing capability rather than introducing one.

### Modified Capabilities

- `worker-lifecycle`: two new requirements — a worker MAY be refused a run by an external
  pause switch (with a manual override and fail-open semantics), and a refused run MUST be
  distinguishable from a successful one in the published metrics.

## Impact

- `internal/worker/main.go` — the pause gate runs before `run()`.
- `internal/worker/pause.go` — new; owns the decision.
- `internal/worker/metrics.go` — publishes `freehire_worker_paused` alongside the existing
  `last_run_*` triple.
- `internal/worker/AGENTS.md` — documents the switch and its keys.
- No migration, no schema change, no API change.
- Dependencies: `github.com/redis/go-redis/v9` and `github.com/alicebob/miniredis/v2` are
  already in `go.mod`; nothing new is added.
- Out of this repository: `freehire_worker_paused` becomes part of the metric-name contract
  with the Grafana dashboard and alert rules in `freehire-ops`, which cannot be compiled
  against this repo. Surfacing the gauge there is part of shipping this, not a follow-up.
