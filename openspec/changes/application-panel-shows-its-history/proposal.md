## Why

The application panel's header carries a strip that reads as a timeline and is not one:

```
● Viewed yesterday ————————————— ● Applied 6 days ago
```

Left to right runs backwards in time. It is an engagement funnel — viewed, saved, applied —
ordered by depth rather than by date, and the component says so in a comment the reader never
sees. It is also nearly empty: three timestamps from `user_jobs`, so an application with seven
linked emails, two stage changes and a follow-up displays none of them.

Those facts are already recorded. Every movement writes to `application_events`, and
`internal/apptimeline` reads that ledger for the Calendar tab. Nothing reads it for one
application, so the panel shows a funnel dressed as history while the history sits unread.

## What Changes

- `ListApplicationEvents(user_id, job_id, limit)` reads one application's live events,
  newest first — the same columns, joins and retraction rule as the calendar's range read, so
  the two cannot disagree about what an event is. Served by the existing partial index
  `application_events_app_idx`.
- `internal/apptimeline` gains `ForApplication`, its second shape of read. It belongs in the
  service rather than a handler because the in-app assistant calls services directly and never
  passes through Fiber — "what happened to this application" is a question it should be able to
  ask.
- `GET /me/tracking/:slug` carries the events. The panel already makes that call for its linked
  mail and its stage suggestion, so the history costs no new route and no new browser request.
- The event labels and colours move out of `TrackingCalendar.svelte` into a shared module, and
  `appevent.Kinds` is emitted into the generated contracts so the map can be checked against the
  vocabulary — the treatment the stage vocabulary just received, for the same reason.
- The header strip is deleted. The history renders in the Application tab, above Stage and Notes.

Deliberately not done: `viewed` and `saved` stay out — they are marks on a posting, and
`viewed_at` is refreshed on every view, so at the foot of a history it would claim to be a first
view while holding the most recent date, a worse lie than the strip it replaces. Events do not
link anywhere; the Emails tab is one click away and does that job properly.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `application-timeline`: the ledger becomes readable for a single application, not only as a
  dated range, and that read is what the application panel renders as its history.

## Impact

- **Go**: `internal/db/queries/application_events.sql`, `internal/apptimeline`,
  `internal/handler/inbox_linking.go` (the detail response), `cmd/gen-contracts`.
- **SQL**: one new query. No migration — the index it needs exists.
- **Frontend**: new `web/src/lib/events.ts`, `TrackingCalendar.svelte` (reads it),
  `JobDrawer.svelte` (strip out, history in), `web/src/lib/types.ts`.
- **Docs**: `docs/API.md`, `web/src/lib/docs/api-spec.ts`.
