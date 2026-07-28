## Context

`/me/tracking` shows a stage and an apply date and leaves the reader to do the
subtraction. Silence — the most common outcome of a job search — is the thing the
product says nothing about.

Every input already exists. `user_jobs.applied_at` gives the floor,
`emails.job_id` gives the mail, and `emails_job_id_idx` (partial, linked rows
only) already covers the lookup. This is a read model, not new collection.

It deliberately waited for the data underneath it. When first drafted, 43% of
applications carried any linked mail; it is now written against 64%, after the
suggestion queue was drained by hand and an `atsPseudoNames` gap was closed that
had let one application absorb 23 other employers' acknowledgements — an
application that, precisely because it collected fresh mail daily, could never
have registered as silent.

## Goals / Non-Goals

**Goals:**

- Say how long an application has been waiting, and whether that is unusual for
  the stage it is in.
- Be wrong in the forgiving direction when wrong at all.
- Make the provenance of every threshold visible, so nobody later mistakes a
  guess for a measurement.

**Non-Goals:**

- Notification delivery. `internal/reminder` is the natural second consumer once
  the thresholds have been watched against live data.
- Follow-up drafting and PDF export — separate capabilities.
- Business-day arithmetic. Every threshold is calendar days, which is why none
  goes below five.

## Decisions

### Last activity is a read-side aggregate, not a column

`GREATEST(user_jobs.applied_at, max(emails.received_at))` over the application's
linked mail, computed on read. No migration, no write path to keep in sync, and
no possibility of the stored value disagreeing with the mail it summarises.

*Alternative considered:* a `last_activity_at` column maintained by the mail
linker. Rejected — it adds a second source of truth for something one aggregate
already answers, and every link, unlink, confirm, reject and delete would have to
maintain it. If the aggregate ever becomes a measured cost at this scale, the
column is still available later; nothing about the wire shape would change.

### Any linked mail counts, not only stage-advancing mail

Restricting the maximum to mail that advanced a stage was measured against the
alternative and changed the outcome by one application out of 118. The reason is
worth recording, because the intuition points the other way: an
auto-acknowledgement arrives within *hours* of applying, so it never moves
`GREATEST(applied_at, …)` — the apply timestamp already dominates it. The
theoretical worry about robot mail resetting the clock is real and empirically
almost nil.

The simpler rule is chosen on that evidence, not on convenience.

### A pending suggestion outranks a silence claim

An application with mail the matcher believes belongs to it, still unconfirmed,
reports `unconfirmed` rather than `silent`. The surface asks for a confirmation
instead of asserting a silence the unconfirmed mail may contradict.

This rule currently fires on nothing: the queue was drained to zero. It stays,
because the queue refills on every classification run and the measurement that
motivated it stands — when 74 suggestions were pending, 7 of the 23 applications
that would have been marked silent had mail sitting unconfirmed. A rule justified
by a backlog that happens to be empty today has not stopped being true.

It also gives the false alarm somewhere to go: instead of "you were ignored", the
card says "two messages may be from them — is this yours?", which is a question
the user can actually answer.

### Thresholds carry their provenance in the source

Five specific numbers read as measurement whether or not they are one. Two are:
`applied` 21 (n=92) and `interview` 12 (n=6). `screening` and `responded` are
linear interpolation between those anchors, stepping evenly by three days so the
shape of the ladder shows which rungs were derived. `offer` 5 is judgement from a
job seeker's experience — no application in the sample has reached that stage,
and the single message ever classified an offer is genuine but from a job search
three years earlier.

The project's dictionaries never guess: an unknown value emits nothing. A
threshold cannot do that — a stage with no threshold cannot be judged — so the
next best thing is to make the guessing visible at the point of definition rather
than let it hide inside a plausible table.

### Errors lean toward under-reporting

Where a choice exists, the design prefers to miss a ghost than to invent one. The
`applied_at` of an application recorded from mail is that mail's timestamp, which
is an upper bound and therefore under-reports elapsed silence. A pending
suggestion suppresses the claim entirely. Terminal stages never accrue silence.

A missed ghost is a non-event. A fabricated one tells a person they were ignored
when they were not, on the product's most emotionally loaded surface.

## Risks / Trade-offs

- **Three of five thresholds rest on little or no data.** Recorded per value
  rather than smoothed over → revisit once the sample grows; the interpolated
  rungs are expected to move.

- **Coverage is 64%, so a third of applications are judged on `applied_at`
  alone.** Their clock is honest but coarse → they can only ever look *more*
  silent than they are, which is the safe direction. Coverage rises as the mail
  surface is used.

- **A read-side aggregate over per-application mail is O(applications) subqueries
  per page.** At a personal mailbox's scale with the partial index this is
  immaterial → if a page ever measures slow, the aggregate becomes a lateral join
  before it becomes a column.

- **Silence is computed against `now()` at read time**, so two requests a day
  apart can report different states for an unchanged application. That is correct
  — the silence really did grow — but it means the value must never be cached
  without its timestamp.

## Open Questions

- Should `days_silent` count from the last *inbound* message only, once outbound
  mail exists in the system? Today every linked message is inbound, so the
  question is not yet answerable.
