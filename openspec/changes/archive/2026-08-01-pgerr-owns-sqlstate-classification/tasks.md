## 1. Move the recognition, leave the policy

- [x] 1.1 Add `pgerr.IsDataCorrupted` with the `XX001` constant, next to the other two, and say
      in its doc that acting on the condition is the caller's decision.
- [x] 1.2 Delete `worker.IsCorruptedRow` and `corruptDataSQLState`; call the shared predicate at
      `resilient.go`'s two sites and drop the now-unneeded `pgconn` import.
- [x] 1.3 Point `internal/enrich` and `internal/embed` at `pgerr` and confirm neither imports
      `internal/worker` any more.

## 2. Make the claim checkable

- [x] 2.1 Move the classification test out of `worker` into `pgerr`, covering all three codes and
      the wrapped case that matters in practice.
- [x] 2.2 `TestOnlyThisPackageUnwrapsAPgError`: no non-test file outside `internal/pgerr` may
      mention `pgconn.PgError`. Guard against a vacuous pass by requiring the walk to have
      scanned a plausible number of files.
- [x] 2.3 Prove the rule fires rather than assuming: add a throwaway unwrap to
      `worker/resilient.go` and confirm the test names that file.

## 3. Verify and close

- [x] 3.1 `go test ./...` AND `go test -tags=integration ./...`.
- [x] 3.2 Mark S15 ✅ in `docs/reviews/2026-08-01-architecture-review.md`.
