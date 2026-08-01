## Why

`application_events` has recorded every movement of every application since migration 0062 —
applied, employer replies, follow-ups, stage changes — each with the date it happened and the
provenance that says whether anyone observed it. Nothing reads it as a series. The writes are
there, `insights.sql` aggregates it per company, and the migration's own comment concedes that
`stage_set` is "written from day one and read by no timing yet".

The candidate therefore cannot see their own search laid out in time. The board answers "where
is this application now" and the list answers "what am I waiting on", but neither answers "what
happened in July" — how many applications went out, when the answers came back, how long the
gaps ran. That question is what a calendar is for, and the data for it is already on disk.

## What Changes

- A new **Calendar** tab under `/my/tracking`, beside Board · List · Pipeline: a month grid of
  the caller's application events with arrows to move month to month.
- Selecting a day opens a panel beneath the grid listing that day's events — employer, role,
  what happened, and the email subject when the event came from mail. Each event offers two
  exits: the application, and the message in `/my/inbox` when there is one.
- A new read of the ledger, `GET /me/timeline?from=&to=`, and the service behind it. This is
  the ledger's first reader for a dated series; today it is only written and aggregated.
- Events carry their provenance to the surface. An event a mail source dated is drawn as
  observed; one the candidate recorded by hand is drawn differently and says so. The verdict is
  the server's, taken from `appevent.TrustedForDayMath`, so the rule has one home.
- No new tables, no migration, no backfill. Every row this reads is already being written.

Not in this change, and deliberately: interview dates and any forward-looking view. The ledger
holds no interview time and neither does the mail — `interview_invitation` means an invitation
arrived, not when the meeting is. That date will come from reading the candidate's own calendar
in a change of its own.

## Capabilities

### New Capabilities
- `application-timeline`: reading the application-event ledger as a dated series for one
  caller — the range read, what each event carries to the wire, how provenance is reported,
  and what happens to an event whose message was deleted.
- `tracking-calendar-view`: the calendar surface itself — the tab, the month grid, the day
  panel and its two exits, the day boundary under the reader's own clock, and the narrow
  layout.

### Modified Capabilities

None. The ledger's existing requirements govern how events are written, retracted and
backfilled; a reader adds to them without changing any of them.

## Impact

- **New**: `internal/apptimeline` (the ledger reader), a query in `internal/db/queries/`
  (`make sqlc`), a handler for `GET /me/timeline`, `web/src/routes/my/tracking/calendar/`,
  and the grouping helper it is tested through.
- **Modified**: `web/src/routes/my/tracking/+layout.svelte` gains a fourth tab; the web API
  client gains the timeline call.
- **Read, not changed**: `application_events`, `applications`, `emails` (subject only).
- **Route hazard**: the new path is `/me/timeline`, not `/me/tracking/calendar`.
  `GET /me/tracking/:slug` is registered in `internal/handler/gmail.go`, and its static
  siblings live in other files — they resolve only because their `Register*` runs first.
  A fourth static segment under that prefix would be a fourth bet on call order.
