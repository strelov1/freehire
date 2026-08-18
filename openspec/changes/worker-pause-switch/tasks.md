## 1. The pause decision

- [x] 1.1 Add `internal/worker/pause.go` with a decision function that reports whether this
      process may run, given a binary name and a Redis URL. Cover the no-key case (runs) and
      the `freehire:pause:all` case (refused) against `miniredis`.
- [x] 1.2 Honour the per-binary key `freehire:pause:<binary>`: `ingest` is refused while
      `search-drain` runs, and every `ingest` invocation is refused regardless of the board
      file argument (the key matches the binary name only).
- [x] 1.3 Honour `FREEHIRE_IGNORE_PAUSE=1`: with the fleet-wide key present and the variable
      set, the decision is "run".
- [x] 1.4 Fail open on an unreachable Redis, on a malformed URL, and on a response slower than
      the 250ms timeout — each logs and returns "run". Assert the slow case completes within a
      bound so a regression to the dial default is caught.

## 2. Metrics

- [x] 2.1 Extend `internal/worker/metrics.go` so a completed run publishes
      `freehire_worker_paused 0` alongside the existing `freehire_worker_last_run_*` triple,
      carrying the same `job`/`instance` labels.
- [x] 2.2 Add a refused-run publisher that writes `freehire_worker_paused 1` and no last-run
      series, to the same `RunMetricsFilename()` the worker already owns.
- [x] 2.3 Pin the exact exposition text of both outputs by test, the way
      `cmd/queue-metrics/render_test.go` pins its own, so a metric rename is a visible edit
      rather than a silent break of the `freehire-ops` contract.

## 3. Wiring

- [x] 3.1 Gate `worker.Main` on the decision before `run()` is called: a refused run publishes
      the paused metrics and exits zero, without initializing Sentry or opening the pgx pool.
      Assert `run` is never invoked when refused.

## 4. Documentation

- [x] 4.1 Document the switch in `internal/worker/AGENTS.md`: both key shapes, the
      presence-as-signal rule, the `EX` convention, the override variable, the fail-open rule,
      and the paused-gauge/stale-timestamp pairing.
- [x] 4.2 Record the out-of-repo contract: `freehire_worker_paused` must be surfaced on the
      worker panel and in the staleness alert's annotation in `freehire-ops`, which cannot be
      compiled against this repo.

## 5. Verification

- [x] 5.1 Run `gofmt -l .` (must print nothing), `go vet ./...`, `go test ./...`, and
      `go vet -tags=integration ./...` before pushing.
