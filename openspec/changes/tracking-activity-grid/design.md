## Context

The tracking section has four views, each its own URL under a shared `TabStrip` layout, and each
a thin page over a component in `$lib/components`. The newest of them, Calendar, established the
pattern this change follows exactly: the arithmetic lives in a pure module
(`web/src/lib/calendarModel.ts`), the component renders that model and nothing else, the page is
served with its data by `web/src/lib/server/tracking.ts`, and a failed server load degrades to a
client fetch rather than a 500.

The material is already published. `GET /me/timeline` serves the caller's application events over
a range and caps one request at `MaxRangeDays = 366`
(`internal/application/apptimeline/apptimeline.go:47`) — a year of squares fits in one request,
though by less margin than it first appears (see the window decision below).
`web/src/lib/events.ts` already owns how an event reads and what tone it is drawn in, shared by
the calendar and the application panel.

Feasibility was measured on production rather than assumed (2026-09-16): 418 users hold events in
the last 366 days, median 5, p95 11, maximum 655. The heaviest user in the database is roughly
200 KB of JSON. Nothing about this page needs an aggregate.

The same measurement produced the rule that shapes the feature. Of every event in the window:
`applied` from a candidate path accounts for 1312, `stage_set` from a candidate path 113,
`follow_up_sent` 1 — against `stage_set` from `system` at 424 and `employer_reply` at 205. Thirty
percent of the ledger is not the candidate's work, and the single largest non-candidate category
is the platform noticing that a listing closed.

## Goals / Non-Goals

**Goals:**

- A year-long contribution grid at `/my/tracking/activity` that reads without instruction.
- A streak figure that means "I have kept showing up", and is therefore built only on the
  candidate's own actions.
- The day panel, filled from data already in hand.
- All grid arithmetic unit-testable without rendering — the timezone and streak math are the
  bug-prone parts, exactly as `calendarModel.ts` argues for the calendar.

**Non-Goals:**

- No new endpoint, table, migration, or stored aggregate.
- No catalogue activity (jobs viewed, jobs saved). Those are not ledger events and live under
  `/my/activity`; the calendar's spec already draws that line and this view keeps it.
- No goal-setting, no reminders, no sharing, no public profile. A grid that can be read is the
  whole change.
- No second colour scale for employer-side events. It was considered and declined (below).

## Decisions

### One pure module owns the arithmetic AND the counting rule

`web/src/lib/activityGrid.ts`, in the shape of `calendarModel.ts`: it takes the flat
`TimelineEvent[]` and today's date, and returns the drawable model — weeks of days, each with its
local day key, its count, its shading level, and its events. The counting predicate lives here
too, beside the arithmetic it feeds, rather than in the component or in a filter applied by the
caller.

*Why here:* a predicate applied at the call site is a predicate each new caller re-decides, and a
second caller applying it slightly differently is how a metric quietly starts meaning two things.
*Alternative considered:* filtering in `+page.server.ts` before handing events to the component —
rejected, because the day panel must show the events that do NOT count, so the unfiltered list has
to reach the component anyway.

### The counting predicate reads kind AND source, and is closed

Counting: `applied`, `follow_up_sent`, and `stage_set` whose source is `user`, `assistant` or
`auto_apply`. Not counting: `employer_reply`, `stage_set` from `system` or any mail/calendar
source, and any kind this build does not recognise.

*Why closed rather than a denylist:* a denylist admits whatever is added to the ledger next, and
the thing most likely to be added next is another observation of somebody else's behaviour. The
failure mode of a closed list is a new candidate action going uncounted until someone notices — a
visible undercount. The failure mode of a denylist is silent inflation, which looks like progress.

*Why source matters for `stage_set` alone:* it is the only kind produced by both the candidate and
the platform. `applied` from `auto_apply` is still the candidate's — they configured it — and
`employer_reply` is never theirs regardless of source.

*The source vocabulary has to be published first.* `cmd/gen-contracts` emits
`APPLICATION_EVENT_KINDS` from `appevent.Kinds` but emits nothing for `appevent.Sources`, so a
predicate reading sources today would be held to a list kept by hand — and a source added in Go
would silently stop counting a candidate action with every test green. One line in the generator
(`emitVocab("ApplicationEventSource", "APPLICATION_EVENT_SOURCES", appevent.Sources)`) makes the
same exhaustiveness test possible for sources that `events.test.ts` already runs for kinds. That
is the entire Go footprint of this change.

*Alternative considered:* counting everything and drawing employer-side events in a second colour.
Declined: it makes the streak undefinable (a streak of what?), doubles the legend, and answers a
question — "am I getting responses?" — that the pipeline view already answers better.

### Grouping by the reader's local day, never the server's

Verbatim the rule `calendarModel.ts` states and for the same reason: `occurred_at` is an instant,
and which square it belongs to depends on a clock only the browser has. Local `Date` accessors
throughout, never the UTC ones; day stepping by calendar arithmetic (`new Date(y, m, d + n)`)
rather than by adding 86 400 000 ms, so a DST transition inside the window does not shift every
later square by an hour.

The stakes are higher here than on the calendar: there, a misfiled event appears on the wrong
square. Here, a misfiled event can break or invent a streak.

### The current streak tolerates an empty today

A naive "consecutive days ending today" resets to zero at every midnight and tells somebody they
lost a run they are still in the middle of. The streak therefore ends at today if today has
actions, and at yesterday otherwise; a gap before that ends it.

*Alternative considered:* reporting "0 — act today to continue your 5-day streak". More honest,
more words, and it still renders the headline figure as a zero. The tolerant reading is what the
reference does and what the number is for.

### Shading levels are relative to the reader, with a floor

Four non-zero levels, thresholded against the reader's own busiest day rather than an absolute
scale, with a minimum so that a person whose maximum day is 2 does not see their whole year drawn
at the darkest level. p95 is 11 events for a whole year, so an absolute scale calibrated for a
heavy user would render almost every real grid as a uniform palest green.

### One request, one window, both consumers

`loadActivityYear` in `web/src/lib/server/tracking.ts`, beside `loadBoard` and `loadTimeline`, asks
`myTimeline` for the window with a day of margin either side — the same margin `rangeForMonth`
adds, and for the same reason: the server's date is not the reader's. It returns `undefined` on
failure, letting the component fall back to its own fetch, exactly as the other two do.

Interviews are deliberately NOT fetched. `interview_scheduled` is a ledger event and arrives with
the timeline; `application_interviews` is the calendar's second layer, about meetings that can
still move, and nothing on this page draws a future.

The 366-day cap is the endpoint's, so the requested window is sized to fit inside it *including*
the margin — and with slack, not exactly. **The cap is an absolute duration and a calendar day
is not always 24 hours.** A span of 366 dates that happens to contain two autumn clock changes
lasts 366 days and two hours, and `apptimeline` refuses it outright rather than trimming it, so
every reader in that zone gets the error state on those dates. A first pass of this design said
"364 + 2 is 366 exactly"; exactly was the bug, and it would have fired in Warsaw on 24 and 25
October 2026 and in Los Angeles around 1 November. `WINDOW_DAYS` is therefore 363, leaving
about a day of slack — more than any zone's transitions can consume, Lord Howe's half hour
included.

What holds it shut is the shape of the test, not the number: `rangeForWindow` is asserted over
all 366 start dates of a year, in BOTH timezone suites, because picking one date to check picks
a date that passes. The original single-date assertion was green against the broken window.

### The day panel is extracted, not copied

A first pass wrote this view's day panel as a transcription of `TrackingCalendar.svelte`'s —
the same forty lines of dot, company line, quoted subject, clock and two links, including the
same eslint suppression. Review caught it, and it is the exact failure `$lib/events` was
created to stop one layer down: that module holds the labels and tones because "copying them
would have meant the same event captioned two ways on two screens". The markup around those
labels had the same property and had been duplicated anyway.

It is now `ApplicationEventList.svelte`, used by both panels, and the calendar lost its copy.

**What it does NOT fix is the ledger's own vocabulary.** `eventLabel` is English for all three
of its consumers (this list, the calendar's cells, the job drawer), while this view ships with
a Russian catalog — so a Russian reader gets Russian chrome around English captions. Half-fixing
it inside the one component that happens to have a catalog would be worse than the gap: the
honest fix is one change to `$lib/events` and its three callers, and it belongs to its own
change. The shared list is where it will land when it does.

### The seam left, not built

If a single caller's year ever approaches a few thousand events, this becomes a payload worth
aggregating server-side, and the natural shape is a daily-count endpoint taking the reader's UTC
offset. It is noted here and not built: today's heaviest user is 655 events, and an aggregate
endpoint would be a table, a query, a timezone parameter and a second definition of the counting
rule, all for a load nobody produces.

## Risks / Trade-offs

- **The counting rule disagrees with the Calendar's totals** → Deliberate and worth saying out
  loud in the UI. The grid's caption names what it counts ("your actions"), so a reader comparing
  the two screens finds an answer rather than a discrepancy.
- **A closed kind or source list goes stale when a candidate action is added to the ledger** →
  Tests assert the predicate against the generated `APPLICATION_EVENT_KINDS` and
  `APPLICATION_EVENT_SOURCES` vocabularies the way `events.test.ts` already does for its labels,
  so an addition in Go fails a test rather than silently scoring zero.
- **A 366-day request is the largest read on the page** → Measured; p95 is 11 events. The server
  load carries it, so it costs the reader nothing after first paint.
- **The grid is wide on a phone** → It is horizontally scrollable, ending scrolled to today, so
  the current week is what a phone opens on rather than a year ago.
- **A streak is a motivator and therefore a pressure** → It counts days, not volume: one
  application keeps a streak. The view never says a day was insufficient.

## Migration Plan

None. Additive front-end only: a new route, a new component, a new pure module, one line in the
tracking layout. Rollback is reverting the commit; nothing is written, nothing is migrated, and no
other view reads the new module.

## Open Questions

None outstanding. The three decisions that shaped the feature — what counts, where it lives, and
what the page holds — were settled before the proposal, and the feasibility question behind the
"no aggregate endpoint" decision was settled by measurement on production rather than estimate.
