## 1. Publish the source vocabulary

- [x] 1.1 Emit `APPLICATION_EVENT_SOURCES` / `ApplicationEventSource` from `appevent.Sources` in
      `cmd/gen-contracts/main.go`, beside the existing `APPLICATION_EVENT_KINDS` line, then run
      the generator and commit the regenerated `web/src/lib/generated/contracts.ts`. Verify with
      `go test ./...` and `gofmt -l .` printing nothing.

## 2. The grid model and the counting rule

- [x] 2.1 RED: write `web/src/lib/activityGrid.test.ts` covering the counting predicate —
      `applied` and `follow_up_sent` count from any source; `stage_set` counts only from `user`,
      `assistant`, `auto_apply`; `employer_reply` never counts; `stage_set` from `system` and from
      the mail/calendar sources never counts; an unrecognised kind never counts.
- [x] 2.2 GREEN: implement the predicate in `web/src/lib/activityGrid.ts`.
- [x] 2.3 RED: extend the test for exhaustiveness — walk `APPLICATION_EVENT_KINDS` and
      `APPLICATION_EVENT_SOURCES` and assert every member has an explicit verdict, so a kind or
      source added in Go fails here rather than silently scoring zero. Mirror the shape
      `web/src/lib/events.test.ts` uses.
- [x] 2.4 GREEN: make the predicate exhaustive over both vocabularies.
- [x] 2.5 RED: test the day grouping — a local-day key derived through local `Date` accessors, an
      instant that falls on a different date in UTC than in the reader's zone filed by the
      reader's date, and day stepping across a DST transition leaving every later date correct.
- [x] 2.6 GREEN: implement grouping and day stepping (calendar arithmetic, never fixed
      millisecond spans), borrowing the shape of `web/src/lib/calendarModel.ts`.
- [x] 2.7 RED: test the window and grid shape — the last square is today, the window spans no more
      than 366 days, every week is seven cells, and a day with no counting actions is present at
      level zero rather than absent.
- [x] 2.8 GREEN: build the week/day model with per-day count, shading level and the day's full
      (unfiltered) event list.
- [x] 2.9 RED: test shading levels — thresholds relative to the reader's own busiest day, with a
      floor so a maximum of 2 does not render the whole year at the darkest level, and an empty
      series producing no division by zero.
- [x] 2.10 GREEN: implement the level scale.
- [x] 2.11 RED: test the streaks — consecutive days ending today; a streak intact when today is
      empty but yesterday was not; a gap ending it; the longest run found anywhere in the window;
      all three figures zero for an empty series.
- [x] 2.12 GREEN: implement current streak, longest streak and the window total.

## 3. The renderer

- [x] 3.1 Create `web/src/lib/components/ActivityGrid.svelte`: a renderer over the model only
      — squares, month labels above the week columns, weekday row labels, legend from less to
      more, and a per-square tooltip naming the date and its count. No arithmetic in the
      component.
- [x] 3.2 Add the caption naming what a square counts ("your actions"), so a reader comparing this
      with Calendar's totals finds an answer rather than a discrepancy.
- [x] 3.3 Add the three figures beneath the grid (total in the window, current streak, longest
      streak), and the worded empty state for a caller with no counting actions.
- [x] 3.4 Add day selection: clicking a square opens a panel listing that day's events — ALL of
      them, including the non-counting ones — captioned through `eventLabel`/`eventTone` from
      `web/src/lib/events.ts`; clicking the selected square closes it. No fetch on selection.
- [x] 3.5 Make the grid horizontally scrollable on narrow viewports, opening scrolled to today
      rather than to a year ago.
- [x] 3.6 Add `ActivityGrid.messages.ts` following the catalog pattern the other components
      use, and route every visible string through it.
- [x] 3.7 Confirm tokens, not raw colours, for the shading scale. The gate is
      `pnpm -C design-system check:tokens` — a repo-boundary check that reads `web/src` against a
      per-file baseline, so a NEW file carrying a colour literal, a Tailwind arbitrary value or a
      raw palette utility (`bg-emerald-500`) fails. A token with an alpha modifier
      (`bg-primary/25`) is none of those and is the scale this uses.

## 4. The route

- [x] 4.1 Add `loadActivityYear` to `web/src/lib/server/tracking.ts`: one `myTimeline` call for
      the window with a day of margin either side, sized to stay inside the endpoint's 366-day
      cap including that margin, returning `undefined` on failure — the same contract
      `loadBoard`/`loadTimeline` hold.
- [x] 4.2 Create `web/src/routes/my/tracking/activity/+page.server.ts` and `+page.svelte`, the
      page a thin wrapper over the component with its own `<title>`.
- [x] 4.3 Have the component fall back to its own client fetch when the server payload is absent,
      with a friendly error state — mirroring `TrackingCalendar.svelte`, including the
      newest-request-wins guard if more than one fetch can be in flight.
- [x] 4.4 Add the fifth tab to `web/src/routes/my/tracking/+layout.svelte` and update the layout
      comment that currently lists four views.

## 5. Verify

- [x] 5.1 `pnpm -C web check` and `pnpm -C web test` green; `pnpm check:dead` clean (an exported
      name nothing imports fails CI, exported types included).
- [x] 5.2 `go build ./...`, `go vet ./...`, `go test ./...`, `gofmt -l .` clean for the generator
      change.
- [x] 5.3 Prove the model reaches the SCREEN, not only the numbers. Written as
      `web/src/lib/components/ActivityGrid.spec.ts` rather than checked by eye, because the
      three things worth confirming are all repeatable: a day holding only an employer reply
      renders as an empty square whose panel still lists that reply; selecting a day issues no
      request; and the span the fallback fetch asks for is one the endpoint will answer. A
      by-eye pass confirms them once, on one account, on one day.
- [x] 5.4 `web/AGENTS.md` does not enumerate the tracking views — no edit needed. The list that
      does is the `SECTIONS` array in the tracking layout, updated in 4.4 along with the comment
      above it.

## 6. Review fixes

- [x] 6.1 **The window overflowed the endpoint's cap on two days a year.** `MaxRangeDays` is an
      ABSOLUTE duration and a calendar day is 25 hours when the clocks go back, so a 366-date
      span containing two autumn transitions lasts 366 days and two hours and is refused
      outright — the error state for every reader in that zone, measured in Warsaw on 24–25
      October 2026. `WINDOW_DAYS` 364 → 363. The old guard test asserted on ONE date and was
      green; both timezone suites now walk all 366 start dates of a year.
- [x] 6.2 Reconcile the spec with the code it describes: the window is bounded by what the
      endpoint answers rather than being "366 days", a month's caption sits on the column its
      week ENDS in, and the weekday rail labels alternate rows and is hidden from assistive
      technology. Same for the stale figures in the proposal and design.
- [x] 6.3 Extract `ApplicationEventList.svelte` (+ its messages catalog) and use it from BOTH
      day panels; remove the ~40 duplicated lines from `TrackingCalendar.svelte`. Record in the
      design why `eventLabel`'s English vocabulary is deliberately left for its own change.
- [x] 6.4 Use the `Card` primitive instead of five hand-rolled `rounded-lg border bg-card`
      divs, and record the adoption-baseline gain.
- [x] 6.5 Unify the naming — `activityGrid.ts`, `ActivityGrid.svelte`, `ActivityDay/Week/Grid`,
      `/my/tracking/activity`, the "Activity" tab. It was four names for one concept.
