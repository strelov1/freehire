## 1. Make the bootstrap rule mechanical

- [x] 1.1 Add a failing guard test in `internal/worker` that walks `../../cmd/*/`, selects the
  packages referencing `worker.Bootstrap`, `database.Connect` or `pgxpool`, and asserts each
  non-exempt one reaches the database through `worker.Bootstrap`, with `cmd/server` the one
  declared exemption and its reason in the literal. It must fail now, naming both backfills.
- [x] 1.2 Guard the population floor: a rename of the bootstrap helper or the pool constructor
  must fail the test rather than silently empty its population.

## 2. Convert cmd/backfill-experience

- [x] 2.1 Replace `main`'s body with `worker.Main(run)` and move the work into `run() int`
  built on `worker.Bootstrap(context.Background())`.
- [x] 2.2 Convert the two `log.Fatalf` sites and the trailing `os.Exit(1)` to `return 1`, so the
  deferred Langfuse flush runs on the failed run; end on `worker.ExitCode(failed, 0)`.
- [x] 2.3 Leave `newExtractor`, `resolveStructured`, `eligible` and `decodeStructured`
  unchanged — including the every-piece-optional policy.

## 3. Convert cmd/backfill-resume-structured

- [x] 3.1 Same conversion: `worker.Main(run)` + `worker.Bootstrap`, and all nine `log.Fatalf`
  sites become `log.Printf` + `return 1`.
- [x] 3.2 End on `worker.ExitCode(failed, 0)` instead of returning normally.
- [x] 3.3 Leave the every-piece-fatal policy, the eligibility SQL and the per-user loop as they
  are.

## 4. Close what review found

- [x] 4.1 Both per-user loops now check `ctx.Err()` at the top and stop. The root context became
  signal-bound in this change, so without the guard a SIGTERM would run the loop to the end of
  the list and turn every remaining user into a logged failure — reporting one cancellation as a
  fleet of them. Follows `cmd/backfill-applications/main.go:84`. A cancelled run says how many
  targets it left and exits non-zero.
- [x] 4.2 Harden the guard test: parse and re-print each file without comments before matching,
  so a `// TODO: migrate this to worker.Bootstrap` no longer satisfies it. Verified — the
  comment-only mutant now fails by name.
- [x] 4.3 Assert the other half of the rule the delta spec added: a package using
  `worker.Bootstrap` must contain no `log.Fatal` or `os.Exit`, since bootstrapping does not stop
  a fatal from skipping the deferred flush. Green fleet-wide today; verified it fails when a
  `log.Fatalf` is reintroduced.
- [x] 4.4 Correct two artifact claims review falsified: the design said the test "cannot be
  defeated by anything short of aliasing the import" (a comment defeated it), and the proposal
  implied run-ending errors reach Sentry (no worker in the fleet calls `CaptureException` —
  this change reaches parity, it does not raise the bar).

## 5. Verify

- [x] 5.1 The guard test passes; verified by mutation that it fails when `worker.Bootstrap` is
  removed from a converted worker, when only a comment mentions it, and when a `log.Fatal` is
  reintroduced.
- [x] 5.2 `go build ./...`, `go vet ./...`, `gofmt -l`, `go test ./...` — all clean.
- [x] 5.3 Both binaries still parse their flags and print usage without a database.
