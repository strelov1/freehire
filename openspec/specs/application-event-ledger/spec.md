# application-event-ledger Specification

## Purpose
TBD - created by archiving change application-event-ledger. Update Purpose after archive.
## Requirements
### Requirement: Application events are recorded from the service that decides them

Every application event SHALL be written by the service layer that already makes the
decision producing it, never by an HTTP handler. The emitting paths are `internal/maillink`
(the classification worker), `internal/inbox` (suggestion confirmation, manual link,
application-from-mail, external triage), `jobtracking.MarkApplied`, `jobtracking.TrackJob`,
and the follow-up record action.

The mail stack already pins this rule for a reason: the in-app assistant calls
`internal/inbox` directly with the session owner's id and issues no HTTP request, so a rule
enforced in a Fiber handler is a rule the in-process agent never meets.

#### Scenario: The assistant links an email

- **WHEN** the in-app assistant confirms an email→application link through `internal/inbox`
- **THEN** an `employer_reply` event is recorded, exactly as if the link had come from the SPA

#### Scenario: Applying records an event in the same transaction

- **WHEN** `jobtracking.MarkApplied` sets `applied_at` for the first time
- **THEN** the `applied` event is written inside the transaction that already holds
  `LockJobForApply`, so the row and its event cannot diverge

#### Scenario: An unchanged value emits nothing

- **WHEN** `jobtracking.TrackJob` is called with the stage the application already carries
- **THEN** no `stage_set` event is recorded

### Requirement: The ledger carries no message content

An event row SHALL hold only the identity of what happened — user, job, company slug, kind,
signal, timestamps, source, and a reference to the source record. It SHALL NOT hold subject
lines, bodies, sender addresses, or any other message content.

The company slug SHALL be denormalized onto the event at write time. `cmd/prune` is the only
hard-delete path for jobs and clears `job_id` when it runs; an event that lost its company
would silently drop out of the company aggregate, which is the instability this ledger exists
to remove.

#### Scenario: A pruned job leaves the event intact

- **WHEN** `cmd/prune` hard-deletes a job that has recorded events
- **THEN** the events survive with `job_id` cleared and still count toward their company's
  aggregate under the slug recorded at the time

#### Scenario: Deleting the account deletes the events

- **WHEN** a user deletes their account
- **THEN** every event of theirs is removed by the `users` foreign-key cascade

### Requirement: Events distinguish when they happened from when they were learned

Each event SHALL carry `occurred_at` — the moment the event happened, taken from
`emails.received_at` for mail-derived events — and `recorded_at`, the moment the row was
written. All day arithmetic SHALL use `occurred_at`.

Connecting a mailbox imports historical mail in one pass, so a ledger keyed on write time
would report a burst of employer replies on the day of connection.

#### Scenario: Historical mail imports at its own dates

- **WHEN** a user connects a mailbox and a year of ATS mail is imported at once
- **THEN** each `employer_reply` event carries the date its message arrived, not the date of
  the import

### Requirement: A mail-derived event is idempotent under replay

Mail-derived events SHALL be unique on `(user_id, kind, source_ref)`, enforced by a partial
unique index over rows whose `source_ref` is present. Re-running the backfill, or a worker
and the backfill reaching the same email, SHALL produce one row rather than two, in any order.

Manually-sourced events carry no `source_ref` and are outside the constraint: two consecutive
follow-ups are two facts, not a duplicate.

#### Scenario: The backfill and the worker overlap

- **WHEN** `cmd/backfill-application-events` and `cmd/classify-mail` both process the same
  email during the backfill window
- **THEN** exactly one `employer_reply` event exists for it, and neither process needs a lock
  to make that true

#### Scenario: A second chase is a second event

- **WHEN** a candidate follows up twice on the same silent application
- **THEN** two `follow_up_sent` events exist, and the first is still readable

### Requirement: Correcting a link retracts the event, deleting the mail does not

A retracted event SHALL be marked with `retracted_at` and excluded from every aggregate, and
the row SHALL remain — an event recorded in error is itself a fact. Re-linking an email to a
different application SHALL retract the event produced by the previous link and record a new
one against the corrected application.

Deleting or hiding the message SHALL NOT retract anything. The two actions carry different
intentions: deletion says the candidate does not want to see the message, re-linking says the
fact belongs to a different employer. A wrong link left standing would poison a named
company's public rate permanently — the case the mail stack already met when one catalogue
company sharing an ATS's name collected twenty-three acknowledgements belonging to other
employers.

#### Scenario: The candidate deletes an answered application's mail

- **WHEN** an email that produced an `employer_reply` event is deleted
- **THEN** the event stands and the company's response rate does not move

#### Scenario: A mislinked email is re-pointed

- **WHEN** an email auto-linked to company A is manually re-linked to company B
- **THEN** A's event is retracted and drops out of A's aggregate, and a new event is recorded
  for B at the same `occurred_at`

### Requirement: A linked message records a reply whether or not it is classified

An `employer_reply` event SHALL be recorded for any message linked to an application,
regardless of whether it carries a classification. The signal, when present, is detail
about what the reply said; it is not the evidence that a reply arrived.

Requiring a classification reads as the stricter rule and is the opposite. `external` mail
— the tier a caller's own harness pushes — is never classified server-side by design, so
those users' replies would never count and their employers would appear more silent than
they were. That is the distortion this ledger exists to remove.

#### Scenario: Unclassified mail still answers an application

- **WHEN** a message is linked to an application but never classified
- **THEN** an `employer_reply` event exists for it, carrying an empty signal, and the
  application counts as answered

### Requirement: Only events with a real date are backfilled

`cmd/backfill-application-events` SHALL replay only the events whose timestamp exists in the
data today: `employer_reply` from `emails.received_at`, `applied` from `user_jobs.applied_at`,
and `follow_up_sent` from `user_jobs.followed_up_at`. It SHALL walk by keyset, in the manner
of `cmd/backfill-derive`.

Stage history SHALL NOT be backfilled. `user_jobs.stage` is a mutable column with no
transition date, so any date assigned to it would be an invention, and the ledger's whole
claim is that its contents were observed.

#### Scenario: Existing applications enter the ledger

- **WHEN** the backfill runs over a tracker holding applications, linked mail, and one
  recorded follow-up
- **THEN** `applied`, `employer_reply`, and `follow_up_sent` events exist at their original
  dates, and no `stage_set` event is created for any of them

#### Scenario: Stage velocity starts empty and honest

- **WHEN** the ledger is queried for stage-to-stage timings immediately after the backfill
- **THEN** it reports no observations rather than timings derived from the current stage column

### Requirement: Correcting an application's date corrects its applied event

When an application's `applied_at` is corrected, the system SHALL move the `occurred_at` of its
`applied` event to the same instant, in the same transaction as the correction.

The event states when the person applied, not when the system was told, and every aggregate
that counts applications or measures how long an employer took to answer reads `occurred_at`.
Correcting one record and not the other leaves the card reporting one month and the statistics
another — two accounts of a single transition, which is the divergence this ledger exists to
prevent.

The correction SHALL NOT write a second `applied` event. One application produced one
application event; restating its date is a repair of that event, not a new fact, and a second
row would inflate the denominator of every response-rate aggregate.

#### Scenario: The column and the ledger agree after a correction

- **WHEN** an application recorded today is corrected to a date last month
- **THEN** its `applied` event carries that same date
- **AND** the pipeline snapshot and the per-company response rate count it under that date

#### Scenario: A correction adds no event

- **WHEN** an application's date is corrected
- **THEN** it still has exactly one `applied` event

#### Scenario: Recording when we learned it is untouched

- **WHEN** an application's date is corrected
- **THEN** the event's `recorded_at` still reports when the row was first written, so the
  distinction between when it happened and when it was learned survives the repair

