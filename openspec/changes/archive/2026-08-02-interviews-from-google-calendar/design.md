## Context

The mail stack already runs this exact shape: a `Connector` builds an incremental Google
consent and exchanges the code for a refresh token, `gmail_connections` stores it
encrypted with a per-user cursor, and `cmd/gmail-sync` walks connected users best-effort,
marking `needs_reconsent` on a token failure. `internal/gmailsync/connector.go` already
passes `include_granted_scopes=true`, so a second consent extends the same grant rather
than replacing it.

Three facts from the surrounding code shape the design:

- **`application_events.occurred_at` means when a thing happened**, and the migration says
  all day arithmetic reads it. `insights.sql` filters by kind explicitly and
  `last_activity_at` is built from `applications` and `emails` columns, so a new kind
  would not poison today's aggregates — but the tracking calendar reads *every* kind in a
  range, and a future-dated row would appear there under a visual language that means
  "observed".
- **`mailmatch` allows only a deterministic tier to auto-link**; a confident model pick
  becomes a suggestion. The reason is that a wrong link transplants one employer's history
  onto another, and the mail stack has the scar to prove it.
- **The ledger is content-free by construction.** Storing only what matched is the same
  discipline applied to a more invasive source.

## Goals / Non-Goals

**Goals:**

- The date of an interview, attached to the right application, without inference.
- A privacy boundary that holds because the data is absent, not because a query is careful.
- One more kind flowing through the reader and the view without either being rewritten.

**Non-Goals:**

- Writing to the candidate's calendar. Reading answers the question; writing is a
  different feature with a different scope and a different consent.
- Reminders and notifications. The digest machinery exists; pointing it at interviews is
  its own change once the data is real.
- Reading a calendar the candidate has not connected, or any calendar-derived analytics.
- Chasing Google verification. It is a business step, not a code change; the design has
  to be honest about the limit, not remove it.

## Decisions

### `internal/calsync` mirrors `internal/gmailsync` rather than extending it

Same shape: a narrow `Store` over `db.Queries`, a `CalendarReader` behind a
`ReaderFactory` so tests inject a fake, a `Worker.RunOnce` that is best-effort per user.
The two syncs share a Google grant and nothing else — different APIs, different windows,
different persistence, and mail's self-learning sender cache has no analogue here.
Folding them together would make one package answer two questions.

*Alternative considered:* extend `gmailsync` with a calendar mode. It shares the OAuth
plumbing, which is the part already factored into `Connector`, and would entangle the
parts that differ.

### The calendar consent is its own step

`gmail.readonly` is a restricted scope; `calendar.readonly` is a sensitive one. Asking for
both at the mail connect would make the mail feature carry the calendar's consent cost,
and a candidate who wants an inbox and not a calendar would have no way to say so. The
connection row records which scopes it holds, so the worker can skip a user whose token
predates the calendar grant instead of learning it from a 403 on every run.

### The meeting gets a table; the ledger gets an observation

`application_interviews` is keyed `(user_id, ical_uid)` and is mutable: a reschedule
updates it, a cancellation marks it. The ledger records `interview_scheduled` dated at the
observation.

Putting the meeting time into `application_events` instead would cost twice. The ledger is
append-only, so a reschedule could not be expressed; and `occurred_at` in the future turns
"what happened, and when" into a schedule — which the calendar view would then draw as a
thing that has already occurred.

### The `iCalUID` is the only automatic link

The invitation's `text/calendar` part and the calendar entry carry the same `UID`, and
the invitation is already linked to an application by the deterministic matcher. So the
link needs no inference at all. Capturing that UID is why `emails.ical_uid` is added at
ingest, in both `gmailsync` and `mailingest`.

Everything else is a suggestion. The organiser's domain is the specific trap: `mailmatch`
bans domain matching for mail because ATS relays send it, and an ATS schedules meetings
from its own domain just as readily — so a domain is evidence about who sent the
invitation, not about who is interviewing.

*Alternative considered:* match on title plus time proximity to the invitation. It is the
sort of heuristic that works in a demo and mislabels an interview under load, and a
mislabelled interview is worse than an absent one — the candidate prepares for the wrong
employer.

### The window is ±90 days

Wide enough to recover the interviews a candidate already sat and to hold anything
scheduled ahead, narrow enough that a sync is one API page for a normal calendar. The
sync is incremental on the same cursor pattern the mail sync uses.

## Risks / Trade-offs

- **Coverage is a function of the ATS, not of our code.** An invitation with no
  `text/calendar` part yields no UID and therefore no automatic link. → The suggestion
  tier is the fallback, and it is a queue the candidate resolves, not a silent gap.
  Measure the ratio before adding heuristics.
- **The grant reaches test users only until Google verification.** → Say so on the connect
  surface. The alternative — letting a consent screen refuse an unlisted candidate — reads
  as a fault in freehire.
- **A calendar the candidate shares with an employer may contain other people's data.** →
  Only matched events are stored, and a matched event is one the candidate's own
  application already names.
- **Not a risk, though it looks like one:** a cancelled interview leaves the
  `interview_scheduled` event standing. The scheduling did happen, and the ledger's whole
  claim is that its contents were observed.

## Migration Plan

One migration, `0070` — `0069` is the newest on disk and the newest in production's
ledger, verified directly. It adds `emails.ical_uid`, `gmail_connections.scopes`, and
`application_interviews`. All three are additive; no backfill. `cmd/cal-sync` does nothing
until a candidate grants the scope, so the code can ship before anyone has.

Rolling back is removing the worker's timer: nothing else writes these columns.

## Open Questions

- **Does `gmail_connections` keep its name?** It is now the Google grant row, and a
  candidate could hold a calendar grant with no mailbox. Renaming it to
  `google_connections` is honest and is a migration plus edits across the mail stack.
  Deferred deliberately: the name is wrong in a way a column comment can carry, and
  churn across a working stack is the more expensive mistake.
