## Why

The employer-reply ledger reconcile is a two-statement **ordering** rule: the retraction must
land before the insert. Both statements are data-modifying CTEs, so they read the same
pre-statement snapshot — run the other way round, the insert's conflict check still sees the
superseded row as live and the message ends with two live events or none.

That rule was documented in three places as having one home, and implemented in two. The second
copy sat in `cmd/classify-mail` — the highest-volume writer of `employer_reply` events, the one
furthest from the rule's documentation, and the one **outside every domain package's test
surface**. Neither copy had a test for the ordering at all.

Unlike S17, the two implementations really were identical: same three steps, same order, same
parameters. Verified by comparison before extracting, because the previous finding of this shape
turned out to have it backwards.

## What Changes

- `inbox.ReconcileMailEvent(ctx, q EventRecorder, userID, emailID int64, mailSource string) error`
  holds the rule. `EventRecorder` is the two-method pair `*db.Queries` and a `WithTx` copy both
  satisfy, which is what lets a best-effort caller and a transactional one share it.
- `inbox.syncLedger` collapses to a call plus a log line; `cmd/classify-mail` calls the same
  function inside its transaction. The error is returned rather than handled, because the callers
  genuinely differ — one is best-effort and idempotent, the other must roll back.
- **Three tests, none of which existed:** the statements run retract-then-record; a failed
  retraction stops the reconcile rather than recording alongside a row nobody retracted; an
  unknown mail source is rejected before either statement runs. The ordering test was verified to
  fire by swapping the two statements.
- Two docs that asserted the opposite are corrected: `docs/agents/mail-stack.md` said "one
  reconcile with five callers" (there were two implementations and six callers), and
  `internal/appevent`'s package doc named `internal/maillink` as a recording path — it does not
  reference `appevent` at all.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

(none) — no requirement-level behaviour change. The same two statements run in the same order
with the same parameters; `tasks.md` is the real artifact and the change archives with
`--skip-specs`.

## Impact

- `internal/inbox` (a new `ledger.go` + tests), `internal/inbox/mutate.go`,
  `cmd/classify-mail/store.go`, `internal/appevent/appevent.go`, `docs/agents/mail-stack.md`.
- `cmd/classify-mail` gains an `internal/inbox` import. That is the right direction: a `cmd/`
  binary depending on a domain package is how every other worker is built, and it is what moved
  the rule inside a test surface.
