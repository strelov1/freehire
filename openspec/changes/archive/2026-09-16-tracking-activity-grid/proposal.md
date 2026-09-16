## Why

A job search is a habit before it is a funnel, and nothing in the product tells a candidate
whether they have kept it. Tracking answers *what is in flight* — Board, List, Pipeline — and
Calendar answers *what happened this month*, but neither answers *have I been showing up*. The
application-event ledger already records every action with a date; a year of it, drawn as a grid,
turns that ledger into the one figure that makes somebody come back tomorrow: a streak.

The GitHub contribution graph is the reference because it is already understood — nobody needs a
legend explained — and because the material for it is already published. `GET /me/timeline`
serves a caller's dated events and accepts a span of up to 366 days — a year of squares, less
the margin the request needs and the slack a clock change eats.

## What Changes

- A new **Activity** tab in the tracking section at `/my/tracking/activity`, beside Board, List,
  Pipeline and Calendar.
- A year-long contribution grid: whole weeks by 7 days, each cell shaded by how much the candidate
  did that day, with month labels, a legend, and a hover tooltip naming the day and its count.
- Three figures beneath it: total actions in the last year, the current streak of consecutive
  active days, and the longest such streak.
- Clicking a day opens a panel listing that day's events, captioned with the same vocabulary the
  calendar and the application panel already use. The panel costs no request — the year is
  already in hand.
- **A square counts only what the CANDIDATE did.** `applied`, `follow_up_sent`, and a `stage_set`
  whose source is the candidate, the assistant, or auto-apply. An `employer_reply` and a
  `stage_set` the platform made on its own are excluded: they are somebody else's action, and a
  green square for one is a reward for work the candidate did not do. Measured on production,
  those two account for 30% of all events in the last year — the largest single category being
  `stage_set` from `system`, which records only that a listing was noticed closed.
- No new endpoint, no new table, no migration. The page is served by the existing
  `GET /me/timeline`. The one Go line this change does add publishes the event-SOURCE vocabulary
  through `cmd/gen-contracts` the way the event-KIND vocabulary is already published, so the
  counting rule can be held to it by a test instead of by a hand-kept list.

## Capabilities

### New Capabilities

- `tracking-activity-grid`: the year-long contribution grid at `/my/tracking/activity` — what a
  square counts, how a day is decided, the streak definition, and the day panel.

### Modified Capabilities

<!-- None. The ledger, its endpoint and the calendar keep their current requirements; this change
     adds a second reader of the same published data. -->

## Impact

- `web/src/routes/my/tracking/+layout.svelte` — a fifth tab.
- `web/src/routes/my/tracking/activity/` — new route (`+page.svelte`, `+page.server.ts`).
- `web/src/lib/activityGrid.ts` — new: the grid's arithmetic and the counting rule, pure and
  unit-testable, in the shape `calendarModel.ts` established.
- `web/src/lib/components/ActivityGrid.svelte` — new: the renderer.
- `web/src/lib/components/ApplicationEventList.svelte` — new: the day panel's event list,
  extracted so this view and the calendar share one, rather than the calendar's being copied.
  `TrackingCalendar.svelte` loses its inline copy.
- `web/src/lib/server/tracking.ts` — a year-range loader beside `loadBoard` and `loadTimeline`.
- `cmd/gen-contracts/main.go` — one line emitting `APPLICATION_EVENT_SOURCES` from the existing
  `appevent.Sources`, plus the regenerated `web/src/lib/generated/contracts.ts`.
- Read-only against `GET /me/timeline`. No SQL, no migration, no Meilisearch, no new Go logic.
- Measured on production 2026-09-16: 418 users hold events in the last year, median 5, p95 11,
  maximum 655. One 366-day fetch is the whole page; an aggregate endpoint would be infrastructure
  built for a load nobody produces.
