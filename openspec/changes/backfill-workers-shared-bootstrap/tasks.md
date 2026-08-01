## 1. Make the bootstrap rule mechanical

- [ ] 1.1 Add a failing guard test in `internal/worker` that walks `../../cmd/*/`, selects the
  packages referencing `database.Connect` or `pgxpool`, and asserts each also references
  `worker.Bootstrap`, with `cmd/server` as the one declared exemption and its reason in the
  literal. It must fail now, naming `backfill-experience` and `backfill-resume-structured`.

## 2. Convert cmd/backfill-experience

- [ ] 2.1 Replace `main`'s body with `worker.Main(run)` and move the work into `run() int`
  built on `worker.Bootstrap(context.Background())`, dropping the hand-rolled `config.Load` /
  `context.Background` / `database.Connect` / `defer pool.Close()`.
- [ ] 2.2 Convert the two `log.Fatalf` sites and the trailing `os.Exit(1)` to `return 1`, so the
  deferred Langfuse flush runs on the failed run; end on `worker.ExitCode(failed, 0)`.
- [ ] 2.3 Leave `newExtractor`, `resolveStructured`, `eligible` and `decodeStructured`
  unchanged — including the every-piece-optional policy.

## 3. Convert cmd/backfill-resume-structured

- [ ] 3.1 Same conversion: `worker.Main(run)` + `worker.Bootstrap`, and all nine `log.Fatalf`
  sites become `log.Printf` + `return 1`.
- [ ] 3.2 End on `worker.ExitCode(failed, 0)` instead of returning normally, so a run where
  users failed no longer exits `0`.
- [ ] 3.3 Leave the every-piece-fatal policy, the eligibility SQL and the per-user loop as they
  are.

## 4. Verify

- [ ] 4.1 The guard test now passes; confirm it still fails if `worker.Bootstrap` is removed
  from one of the two converted workers.
- [ ] 4.2 `go build ./...`, `go vet ./...`, `gofmt -l`, `go test ./...`.
- [ ] 4.3 Confirm both binaries still parse their flags and print their usage — `go run
  ./cmd/<name> --help` — without a database.
