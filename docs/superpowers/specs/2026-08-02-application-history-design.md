# The application panel shows what happened

## Problem

The panel's header carries a strip that reads as a timeline and is not one:

```
● Viewed yesterday ————————————— ● Applied 6 days ago
```

Left to right runs *backwards in time*. The strip is an engagement funnel — viewed, then saved,
then applied — ordered by depth rather than by date, and `JobDrawer.svelte` says so in a comment:
"Applied is the deepest step, so it anchors the right end regardless of the raw last-viewed
timestamp". A reader has no way to know that. They see dates in a row and read a sequence.

It is also nearly empty. It draws three timestamps from `user_jobs`, so an application with seven
linked emails, two stage changes and a follow-up shows none of them. The facts exist: every
movement is written to `application_events`, and `internal/apptimeline` already reads that ledger
for the Calendar tab. Nothing reads it for one application.

## Goals

1. The panel shows the application's real history, newest first, from the ledger.
2. The event vocabulary — labels and colours — has one definition, shared with the Calendar.

## Non-goals

- `viewed` and `saved` do not appear. They are marks on a posting, not events of an application,
  and `viewed_at` is refreshed on every view — placed at the foot of a history it would claim to
  be a first view while holding the date of the most recent one, which is a worse lie than the
  strip it replaces.
- Events do not link anywhere. A mail event reads as a line, not a link: the Emails tab is one
  click away and does that job properly.
- The Calendar tab is untouched beyond taking its labels from the shared module.

## Design

### Reading one application's ledger

A new query beside the calendar's, in `internal/db/queries/application_events.sql`:

```sql
-- name: ListApplicationEvents :many
SELECT ... FROM application_events ae
 WHERE ae.user_id = $1 AND ae.job_id = $2 AND ae.retracted_at IS NULL
 ORDER BY ae.occurred_at DESC, ae.id DESC
 LIMIT $3;
```

Same columns as `ListApplicationEventsInRange`, same exclusion of retracted rows, same joins for
the role title and the message subject — and the same reason for each, so the two reads cannot
disagree about what an event is. It is served by the existing partial index
`application_events_app_idx (user_id, job_id, kind) WHERE retracted_at IS NULL`.

`internal/apptimeline` gains `ForApplication`. The package is already "the ledger's first dated
reader"; this is its second shape of read, and it belongs there rather than in a handler for the
reason `internal/inbox` states — the in-app assistant calls services directly and never passes
through Fiber, so "what happened to this application" put in a handler is a question it cannot
ask.

### The wire

Events ride on the existing `GET /me/tracking/:slug` as `events`. The panel already makes that
call — for the linked mail and the stage suggestion — so the history costs no new route and no
new request from the browser.

The cap is 100 events. An application accrues a handful; the limit is hygiene, matching
`apptimeline.MaxRangeDays`'s reasoning — an unbounded read of an append-only table costs nothing
now and something later.

### One event vocabulary

`TrackingCalendar.svelte` holds the labels (`Applied`, `Employer replied — …`, `Followed up`,
`Moved to …`) and a colour per kind. Those move to `web/src/lib/events.ts`, and `appevent.Kinds`
is emitted into the generated contracts so the SPA's map can be checked against it — the same
treatment the stage vocabulary just received, for the same reason: a kind added in Go and missed
in the SPA renders as a blank row with every test green.

### The panel

The header strip is deleted. The history renders in the Application tab, above Stage and Notes,
newest first, each row a dot, a relative time, and the event's label. An application with no
events yet — saved but never applied — shows nothing rather than an empty frame.

## Testing

- `ListApplicationEvents` under `-tags=integration`: newest-first order, retracted rows absent,
  another user's events absent, the limit respected.
- `apptimeline.ForApplication`: the mapping from row to event, and that a deleted message yields
  an event with no subject rather than dropping the event.
- The label map covers every kind in the generated vocabulary — verified by mutation, by adding
  a kind and watching the check fail.
- The panel renders newest-first and renders nothing when the ledger is empty.
