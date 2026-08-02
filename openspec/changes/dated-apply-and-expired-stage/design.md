## Context

Two records describe one application date. `applications.applied_at` answers "when did this
happen"; the `applied` row in `application_events` answers the same question for every
aggregate, because the ledger — not the column — is what the pipeline snapshot and the
per-company response rate read. `MarkJobApplied` writes both in one statement, deliberately:
its comment records that two accounts of one transition, decided separately, eventually
disagree.

That statement already accepts an optional date. It was added for applications reconstructed
from employer mail, where the message proves the application existed by the time it was
written, and it deliberately refuses to overwrite a date already present — a later recording
is not a later application. Nothing exposes the parameter over HTTP.

The stage vocabulary lives in `internal/userjob` as four tables keyed on one list: which
stages exist, how far along each is and whether it is settled, how long each tolerates
silence, and which group it shows as. Three tests bind them, so a stage added to one table and
forgotten in another fails the build rather than rendering as an invisible column.

## Goals / Non-Goals

**Goals:**

- Record an application on the day it actually happened, through the API and the CLI.
- Correct a date already recorded, moving its ledger event with it.
- Give the vocabulary a settled outcome for "nobody answered", shown in the `closed` column.

**Non-Goals:**

- Automatic closing. No worker decides an application expired; the candidate does.
- Bulk operations. The tracker has none, and this change does not introduce the first.
- A second silence mechanism. `silence_state` already reports who is overdue; `expired` is an
  outcome a person records, not a verdict a threshold reaches.
- Any migration. Both columns exist and already hold what this writes.

## Decisions

### The date goes in the apply body, not in the track patch

`PATCH /track` edits the working state of an application — its stage and notes. The date an
application was sent is not working state; it is an attribute of the act itself, which is what
`POST /apply` records. Putting it in the body of `apply` also keeps import to one request per
application rather than two.

Alternative considered: a field on `PATCH /track`. Rejected because `track` cannot create an
application, so importing a history would mean `apply` (dated today) followed immediately by
`track` (to fix the date) — two writes, and a moment where the ledger holds the wrong day.

### A stated date wins; an inferred one does not

The date a person names is better evidence than anything we derive, so it overwrites. The mail
path keeps the opposite rule, because its date is an upper bound read off a message: if the
candidate said "I applied on the 3rd" and an ATS acknowledgement arrives on the 5th, the 3rd
is right. The two paths therefore call different service methods, and the existing
`MarkAppliedAt` keeps its semantics unchanged.

### `MarkJobApplied` is not modified; re-dating is a second statement

The upsert decides the `applied_count` transition and the ledger insert under one predicate.
Adding an overwrite mode would fork that predicate: on a correction the event already exists,
so the ledger needs `UPDATE` where the statement does `INSERT`, and the counter must not move
at all. Instead a separate `RedateApplication` performs both updates, and a service method
runs create-then-redate inside one transaction. Each statement keeps one job, and the
correction path cannot disturb the counter it never mentions.

Alternative considered: one statement with a mode flag. Rejected — the most consequential
query in tracking grows a branch, in exchange for saving a round trip inside a transaction
that is already open.

### A calendar day, stored at noon UTC

The wire format is `YYYY-MM-DD`, matching the ghost report, whose comment explains why: a
person is stating a day, and a timezone-bearing instant reads as a different day either side
of a border. The storage column is `timestamptz`, so the day must become an instant, and
midnight is the wrong one — `2026-07-27T00:00:00Z` renders as 26 July for every reader west of
Greenwich. Noon UTC survives every offset in use. The ghost report has no such problem; its
column is a `DATE`.

Bounds are the ghost report's: not in the future, not more than a year ago, otherwise `400`.
Reusing them keeps one answer to "which dates are believable" rather than two that drift.

### `expired` is terminal, ungraded and unthresholded

An application does not pass through "expired" on its way anywhere, so it gets no `activeRank`
and no silence threshold — settled applications wait on nobody. It joins `closed` alongside
the other three outcomes, and the card keeps showing its own label, so "Expired" and
"Rejected" remain distinguishable inside one column.

The existing rule that automatic advancement never enters or leaves a terminal stage gives the
feature its safety property for free: mail classified after the candidate marked an
application expired cannot resurrect it.

### A backdated application reaches two public aggregates, and that is accepted here

`applied_at` feeds the ghost-job evidence query, and the `applied` event's `occurred_at` feeds
the per-company response rate and median reply time. A date the candidate states therefore
moves both, and a backdated apply produces mature ghost evidence immediately rather than after
the silence ladder has run.

That is the same reach an application has always had — the tracker's own dates were never
audited — and the alternative, excluding user-dated applications from the rollups, would make
an imported history invisible in exactly the statistics it exists to inform. Two things bound
it: the ghost evidence path already requires a connected mailbox, and the year-old limit stops
stale history from speaking about a live posting.

Note the deliberate asymmetry with the ghost report, which states in its own package comment
that a claim is "their word, not an observation, which is why it never reaches the tracking
board's `applied_at`", and caps filings at twenty a day. That separation is about a claim made
against **someone else's** posting. This date is a claim about the candidate's **own**
application, so it belongs on their own record — and the honest reading of the response rate is
"how employers treat applications people say they sent". If that ever needs tightening, the
lever is the rollup's own filter, not this endpoint.

## Risks / Trade-offs

**A correction silently disagrees with the ledger** → The service method owns both writes in
one transaction, and an integration test asserts the column and the event report the same
instant after a correction. This is not hypothetical: it is exactly the divergence that had to
be repaired by hand on production, with a second `UPDATE` remembered only because the ledger's
role was already understood.

**Noon UTC is a lie about the hour** → It is. The stored instant is not when the person
clicked "apply", and nothing claims it is: the wire contract is a day. Anyone needing the hour
has the `applied` event's `recorded_at`, which is untouched.

**`expired` overlaps with `silence_state` in a reader's mind** → They answer different
questions, and the split is enforced by structure rather than documentation: silence is
computed from thresholds and reports nothing for settled applications, while `expired` is a
stored outcome no threshold can reach. A card cannot show both, because the moment it becomes
`expired` the silence verdict goes quiet.

**Two vocabularies could drift between Go and the SPA** → They cannot: the labels and groups
are generated, and a type-level check in the required `pnpm run check` gate fails when a
generated stage is missing from a generated group.
