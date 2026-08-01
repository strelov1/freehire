## 1. Restore the signal at the source

- [x] 1.1 `FailEmailClassification` → `:one ... RETURNING attempts, failed_at`, matching the two
      siblings its comment already claimed to mirror. Regenerate with `make sqlc`.
- [x] 1.2 `dbStore.Fail` reports `row.FailedAt.Valid` — the stamp IS the dead-letter signal.

## 2. Carry it up

- [x] 2.1 Widen `maillink.Store.Fail` to `(deadLettered bool, err error)`, with the doc saying
      the bool is what the exit code is built from.
- [x] 2.2 `Runner.Run` returns `Stats{Failed, DeadLettered}`, tallied where the bookkeeping error
      is already logged.
- [x] 2.3 A Fail that itself fails counts as failed and NOT as dead-lettered — the entry is left
      to its lease expiry, so the state is unknown and must not be guessed.
- [x] 2.4 `cmd/classify-mail` ends on `worker.ExitCode`, and logs both counts.

## 3. Tests

- [x] 3.1 Every message failing, all dead-lettering → `Failed=2, DeadLettered=2`.
- [x] 3.2 A retryable failure counts as failed only — counting it as a dead letter would fail a
      run for a queue working exactly as designed.
- [x] 3.3 The unknown case: `deadLetter` would have been true, but the write failed.

## 4. Verify and close

- [x] 4.1 `go test ./...` AND `go test -tags=integration ./...`.
- [x] 4.2 Confirm `cmd/classify-mail` now appears in the `worker.ExitCode` census.
- [x] 4.3 Mark S20 ✅ in `docs/reviews/2026-08-01-architecture-review.md`.
