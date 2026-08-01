## 1. Confirm they really are the same

- [x] 1.1 Compare the two implementations step by step before extracting — the previous finding
      of this shape (S17) had the drift backwards, and deleting the "copy" would have kept live
      defects.
- [x] 1.2 Record the one genuine difference: error semantics, which the extraction must preserve
      rather than unify.

## 2. One home, with the interface that already existed

- [x] 2.1 `inbox.ReconcileMailEvent` over an `EventRecorder` pair that `*db.Queries` and a
      `WithTx` copy both satisfy. Return the error; let each caller decide.
- [x] 2.2 `syncLedger` becomes a call plus a log; the worker calls it inside its transaction and
      drops its now-unused `appevent` import.

## 3. Test the rule, not the plumbing

- [x] 3.1 The statements run retract-then-record. Verify the test fires by swapping them.
- [x] 3.2 A failed retraction stops the reconcile — recording anyway is the two-live-events state
      the ordering exists to prevent.
- [x] 3.3 An unknown mail source is rejected before either statement runs.

## 4. Correct the docs that asserted the opposite

- [x] 4.1 `docs/agents/mail-stack.md` — "one reconcile with five callers" was neither: two
      implementations, six callers.
- [x] 4.2 `internal/appevent` named `internal/maillink` as a recording path; it does not
      reference the package.

## 5. Verify and close

- [x] 5.1 `go test ./...` AND `go test -tags=integration ./...`.
- [x] 5.2 Mark S18 ✅ in `docs/reviews/2026-08-01-architecture-review.md`.
