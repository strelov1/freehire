## 1. The ledger read

- [x] 1.1 Add `ListApplicationEventsInRange` to `internal/db/queries/application_events.sql`:
      the caller's events between two timestamps, `retracted_at IS NULL`, ordered by
      `occurred_at`. `LEFT JOIN applications` for `role_title` and `LEFT JOIN emails ON
      ae.source_ref = e.id` for `subject` and `id`, with both email columns NULL when
      `e.deleted_at IS NOT NULL`. Select `kind`, `signal`, `source`, `occurred_at`,
      `company_slug`, `application_id`, `job_id`. Run `make sqlc`.
- [x] 1.2 Add `internal/apptimeline` with a package doc stating what it is (the ledger's first
      dated reader) and why it is not in `jobtracking` (that package is per-`(user, job)`
      mutations; this is a range read, and the in-app assistant will call it without HTTP).
      Define the `Event` type and a `Range(ctx, userID, from, to)` method over a narrow
      `Queries` interface, following the shape `internal/inbox` uses.
- [x] 1.3 Set `Observed` on each event from `appevent.TrustedForDayMath(source)` inside
      `apptimeline`, never at the handler or in the SPA. Comment says why: the source
      vocabulary has grown before, and a second copy of the rule would call a newly added
      source observed after the first had refused it.
- [x] 1.4 Validate the range in the service: `from` and `to` required, `from <= to`, span
      capped at 366 days, returning `apptimeline`'s invalid-input error so both the handler
      and a future in-process caller meet the same rule.

## 2. Tests for the read

- [x] 2.1 Integration test (build tag `integration`, `internal/db`): an event whose email was
      deleted comes back with its date, employer and signal, and with neither subject nor
      email id.
- [x] 2.2 Integration test: a retracted event is absent and its replacement is present at the
      same `occurred_at`; and a second user's events on the same day are not returned.
- [x] 2.3 Unit test in `apptimeline`: `Observed` equals `appevent.TrustedForDayMath` for every
      entry in `appevent.Sources`, asserting the collection length first so a source added
      without a verdict fails the test rather than being skipped — the device
      `TestOnlyMailSourcesAreTrustedForDayMath` uses.
- [x] 2.4 Unit test in `apptimeline`: the range validation — missing bounds, inverted bounds
      and an over-long span are refused; a single-day range is accepted.

## 3. The endpoint

- [x] 3.1 Add `GET /me/timeline` under `mw.key` in `internal/handler`, parsing `from`/`to` as
      RFC3339 and rendering `{"data": [...], "meta": {"from", "to", "count"}}`. Comment the
      path choice: not `/me/tracking/calendar`, because `GET /me/tracking/:slug` is registered
      in `gmail.go` and its static siblings resolve only on `Register*` call order — the same
      reasoning already recorded above `/me/applications/:id` in `user_jobs.go`.
- [x] 3.2 Serve `kind` and `signal` as plain strings from their vocabularies; do not enumerate
      today's four kinds in the response type or in any switch that would fail on a fifth.
- [x] 3.3 Handler tests: unauthenticated is 401, a bad or inverted range is 400 before any DB
      touch, and a valid range renders the documented envelope.
- [x] 3.4 Add the route to the API documentation surface alongside the other `/me` reads.

## 4. The month model

- [x] 4.1 Add `web/src/lib/calendarModel.ts`: pure functions turning a flat event series into a
      month grid — the weeks and days of a given month, each day's events, and the fetch range
      for a month with one day of margin on each side. Follow `activityChart.ts`: the
      bug-prone arithmetic lives here so it is testable without rendering.
- [x] 4.2 Group by the reader's local day, not by UTC. The model takes instants and derives the
      day from them locally; nothing in it formats a date server-side.
- [x] 4.3 Vitest for `calendarModel.ts`: an event at 23:40 UTC lands on the next local day for
      a reader ahead of UTC; the first and last cells of a month receive events that fall in
      them only under the local offset; an empty month yields a full grid of empty days; a day
      with more events than the cell holds reports the remainder.

## 5. The calendar view

- [x] 5.1 Add the Calendar tab to `web/src/routes/my/tracking/+layout.svelte`, following the
      existing `tablist` pattern and its `aria-selected` / `activeTabId` handling.
- [x] 5.2 Add `web/src/routes/my/tracking/calendar/+page.server.ts` fetching the current UTC
      month plus margin through a `loadTimeline` helper in `web/src/lib/server/tracking.ts`,
      returning `undefined` on a transient failure the way `loadBoard` does.
- [x] 5.3 Add the timeline call to `web/src/lib/api.ts` and the event type to the web types,
      matching how the contract types are generated or declared for the other `/me` reads.
- [x] 5.4 Add `web/src/routes/my/tracking/calendar/+page.svelte` and a `TrackingCalendar`
      component rendering the model: month grid, previous/next month, per-kind marks with
      observed drawn filled and hand-recorded drawn outlined, and an overflow count.
- [x] 5.5 Day panel beneath the grid, filtered from data already loaded — no fetch on select.
      Each entry shows employer, role, what happened, the subject when there is one, and links
      to the application and, when the message still exists, to it in `/my/inbox`.
- [x] 5.6 Render an unrecognised kind generically (date, employer, role) rather than dropping
      it or throwing, so the interview kind can arrive without a change here.
- [x] 5.7 Narrow viewport: present the month as a list of days holding events instead of a
      seven-column grid.
- [x] 5.8 Empty state: a caller with no events at all is told so and pointed at the board,
      rather than shown an empty month.

## 6. Verify

- [x] 6.1 `go build ./...`, `go vet ./...`, `go test ./...`, and `go test -tags=integration
      ./internal/db/` — both runs, since the untagged run does not compile the integration
      tests.
- [x] 6.2 `web/`: install the linked design system first, then `svelte-check`, `eslint` and
      `vitest`; all green against the repository's existing baseline.
- [x] 6.3 Visual check against a running authed stack: the tab is selected and reload-safe at
      `/my/tracking/calendar`; a month with events renders marks; selecting a day opens the
      panel with no network request in the devtools panel; a mail event links to the message
      and a deleted-message event does not; the narrow layout lists days.
