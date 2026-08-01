## 1. Let a narrow store reach the shared reader

- [x] 1.1 `internal/worker/resilient.go`: give each reader constructor its own narrow
  interface — `NewFullScanReader` takes the three methods `fullScanReader` calls,
  `NewPostedSinceReader` the three `postedSinceReader` calls — instead of both taking the
  five-method `jobQueries`. `*db.Queries` satisfies both, so `cmd/reindex` is unchanged.

## 2. The derive backfill scans resiliently

- [x] 2.1 `cmd/backfill-derive/main.go`: widen `deriveStore` to carry
  `ListJobsByIDAfter` / `ListJobIDsAfter` / `GetJob` plus `UpdateJobDerived`.
- [x] 2.2 `cmd/backfill-derive/main.go`: read the producer's pages through
  `worker.ResilientPage` over `worker.NewFullScanReader(store)`, replacing the raw
  `ListJobsByIDAfter`; drop the `len(jobs) < backfillBatchSize` early return and end the
  scan on `lastID == afterID`.
- [x] 2.3 `cmd/backfill-derive/main.go`: count skipped ids and report them in the run's
  log summary, as `cmd/reindex` does.

## 3. Verification

- [x] 3.1 `go build ./... && go vet ./... && go test ./...` green;
  `go test -tags=integration ./internal/db/` green; `openspec validate
  resilient-derive-backfill --strict` passes.
