## Why

Three catalogue findings in the mail/notifications area, each a rule with more copies than it
should have. All three verdicts were **overstated**, and checking that before acting changed what
the work is.

**#27 — the pending-suggestion predicate has two spellings.** Three statements ask
`e.application_id IS NULL`; the tracking board asks `e.job_id IS NULL`. Two of the three carry a
comment saying the test is "the same … so the two cannot disagree about one message".

**#28 — the "one channel abstraction" the docs promise is two.** `Notifier`, `Router`,
`ErrChannelNotConfigured` and `recipient()` each exist twice, and `notifications.md` claimed
adding a channel "means adding a package, not touching `notify` or `reminder`" — which is false.

**#29 — the silence ladder is shared, the arithmetic under it is not.** Three surfaces each carry
their own "whole days, floored at zero", held together by comments naming one another.

## What Changes

- **#27:** the board's predicate becomes `e.application_id IS NULL`, matching the other three.
- **#28:** `notify.ValidChannel` is exported and the two hand-built `map[string]bool` allowlists
  in `subscription` and `reminder` are deleted. `notifications.md` now states the real cost of a
  new channel — two notifiers, two `recipient` cases, two mains — instead of the claim that was
  not true.
- **#29:** `userjob.DaysSilent(now, last)` beside the ladder it feeds; the three copies call it.

## What I did NOT do, and why

- **No shared view for the last-activity expression (#27).** The remedy says to skip it, and a
  fifth copy is the trigger to reconsider.
- **No `notify.Router[T]`/`Notifier[T]` generics (#28).** They would make `notify` depend on both
  delivery row shapes — worse coupling than the duplication. Deferred until a third channel lands
  and shows which half generalises.
- **No single `userjob.Silence(...)` (#29).** The `applied_at` fallback and the "is this even an
  application" precondition are shaped by three different query results, and `ghost` deliberately
  does not want `jobtracking`'s fallback semantics.

## Two claims I could not confirm, stated plainly

**#27's live disagreement is not reachable that I can construct.** It requires a message carrying
a live suggestion *and* a set `job_id`. `LinkEmailToJob` and `ConfirmEmailLink` both clear
`suggested_job_id` when they link; `SetEmailClassification` can write both but its only producer
sets one or the other. The change is still right — two spellings of one rule held apart by a
coincidence of writer behaviour is not an invariant — but it is a drift risk closed, not a bug
fixed.

**#29's live disagreement was already refuted by the finding's own verifier** and I confirmed the
reasoning: both queries wrap the expression in `CASE WHEN a.applied_at IS NOT NULL THEN
GREATEST(...)`, and Postgres `GREATEST` ignores NULLs, so `last_activity_at` is NULL exactly when
`applied_at` is. What survives is the duplicated arithmetic.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

(none) — no requirement-level behaviour change. The predicate change is equivalent under the
outer `a.applied_at IS NOT NULL` guard; `tasks.md` is the real artifact and the change archives
with `--skip-specs`.

## Impact

- `internal/db/queries/user_jobs.sql` (+ regenerated), `internal/notify`, `internal/subscription`,
  `internal/reminder`, `internal/userjob`, `internal/jobtracking`, `internal/handler/followup.go`,
  `internal/ghost`, `docs/agents/notifications.md`.
