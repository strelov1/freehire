## 1. Textfile write seam

- [ ] 1.1 Extract the write-then-rename in `internal/worker/metrics.go` into an exported `WriteTextfile(dir, name, body string) error`, and have `writeRunMetrics` call it. Extend `metrics_test.go` to cover the helper directly: it writes the body, leaves no `.tmp` behind, and a failed rename leaves any previous file intact.

## 2. Aggregate queries

- [ ] 2.1 Add `internal/db/queries/metrics.sql` with one query per outbox table returning live depth, dead-letter count, and oldest live entry's age in one pass; run `make sqlc`.
- [ ] 2.2 Add the board-fleet query returning healthy / failing / cooled counts, with `cooled` taking precedence over `failing` so the three states are mutually exclusive; run `make sqlc`.
- [ ] 2.3 Add the catalogue-freshness query returning the newest job's `created_at`, distinguishing "no jobs" from a zero timestamp; run `make sqlc`.
- [ ] 2.4 Add an integration test (`//go:build integration`) in `internal/db` covering all five queries against a seeded database: the live/dead split, an empty queue, a board counted as cooled rather than failing, and an empty catalogue.

## 3. The worker

- [ ] 3.1 Create `cmd/queue-metrics/main.go` following `internal/worker/AGENTS.md`: `worker.Main(run)`, `worker.Bootstrap`, `worker.ExitCode`. Exit zero without querying anything when `PROM_TEXTFILE_DIR` is unset.
- [ ] 3.2 Render the collected values as Prometheus text format with `# HELP` and `# TYPE` per family, omitting `freehire_catalogue_newest_job_timestamp_seconds` when the catalogue is empty while publishing explicit zeros for drained queues.
- [ ] 3.3 Add a golden test pinning the exact rendered output — metric names, label sets, help and type lines — since these are the contract the `freehire-ops` alert rules bind to. Cover the empty-catalogue and drained-queue cases.
- [ ] 3.4 Wire collection failure to a non-zero exit with a logged error, and cover it with a test.

## 4. Verification

- [ ] 4.1 `gofmt -l .` prints nothing; `go vet ./...` and `go test ./...` pass.
- [ ] 4.2 `go vet -tags=integration ./...` passes, and `go test -tags=integration ./internal/db/` passes against Docker.
- [ ] 4.3 Run the worker against a local database with `PROM_TEXTFILE_DIR` set and confirm the emitted file parses as Prometheus text format and carries every metric the design names.

## 5. Documentation

- [ ] 5.1 Add `cmd/queue-metrics` to the worker list in `CLAUDE.md`, noting that it needs `DATABASE_URL` and is a no-op without `PROM_TEXTFILE_DIR`.
- [ ] 5.2 Record the published metric names and label sets in `internal/worker/AGENTS.md`, including the `exported_job` relabelling trap, so a future query is written against the right label.
