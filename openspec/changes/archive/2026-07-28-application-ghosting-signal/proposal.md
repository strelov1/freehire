## Why

A tracked application that goes quiet is indistinguishable from one still in
progress: `/me/tracking` shows a stage and an apply date, and the reader is left
to do the subtraction and guess whether "applied 3 weeks ago" still means
anything. Silence is the single most common outcome of a job search and the one
the product currently says nothing about.

The signal is already in the database — `user_jobs.applied_at` plus the linked
mail in `emails` — so this is a read model over data we already store, not new
collection.

It waited for that data to be worth reading. Drafted when 43% of applications
carried any linked mail, it is now written against 64%, after the suggestion
queue was drained and an `atsPseudoNames` gap was closed that had one application
collecting 23 other employers' acknowledgements.

## What Changes

- `/me/tracking` gains, per application, a **last activity timestamp**, the
  **days silent** derived from it, and a **silence state** computed against a
  stage-aware threshold.
- Last activity is `GREATEST(user_jobs.applied_at, max(emails.received_at))` over
  that application's linked mail. Measured on real data, restricting the maximum
  to stage-advancing mail changes the outcome by one application out of 118 —
  auto-acknowledgements arrive within hours of applying, so they never move the
  maximum. The simpler rule is used deliberately.
- Thresholds vary by stage: `applied` 21, `screening` 16, `responded` 14,
  `interview` 12, `offer` 5 days. Terminal stages (`rejected`, `accepted`,
  `withdrawn`) never accrue silence at all. On the current data this marks 15 of
  92 applications at `applied` and 3 of 6 at `interview`.
- **An application with a pending link suggestion is never reported as silent.**
  It reports a distinct "unconfirmed mail" state instead, so the surface asks the
  user to confirm the link rather than asserting a silence that the unconfirmed
  mail may contradict.

  This rule fires on nothing today — the queue was drained to zero by hand — and
  it is kept anyway. The queue refills with every classification run, and the
  measurement that motivated the rule still stands: when 74 suggestions were
  pending, 7 of the 23 applications that would have been marked silent had mail
  sitting unconfirmed. A rule justified by a backlog that is currently empty is
  not a rule that stopped being true.
- The tracking board renders the state as a per-card marker.

Not in this change: notification delivery for silent applications, follow-up
message drafting, and PDF export of applications. The first is a natural second
consumer of `internal/reminder` once the thresholds have been watched against
live data for a while; the other two are separate capabilities.

Also not here: business-day arithmetic. Every threshold is calendar days, which
is why none goes below five — three days can be entirely weekend. Counting
working days needs a calendar, holidays and the employer's time zone, and earns
its own change if the offer threshold ever proves too blunt.

## Capabilities

### New Capabilities
- `application-silence-signal`: derives and exposes how long a tracked
  application has been without activity, the stage-aware threshold it is judged
  against, and the resulting state — including the precedence of an unconfirmed
  link suggestion over a silence claim.

### Modified Capabilities
- `user-job-tracking`: the `/me/tracking` projection gains the silence fields, so
  the response contract for a tracked application changes.

## Impact

- **Read path only.** No migration: `user_jobs` and `emails` already carry every
  input, and `emails_job_id_idx` (partial, `job_id IS NOT NULL`) already covers
  the per-application mail lookup.
- `internal/db/queries` — the `/me/tracking` query gains the activity aggregation.
- `internal/userjob` — a new stage→threshold table, alongside the existing stage
  vocabulary in `stages.go`.
- `internal/handler/user_jobs.go` and the tracking board in `web/` — wire shape
  and rendering.
- Nothing in the mail stack changes: this is a consumer of `emails.job_id` and
  `emails.suggested_job_id`, not a change to how either is decided.
