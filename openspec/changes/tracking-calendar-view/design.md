## Context

`application_events` (migration 0062) has been written by every path that moves an application
since it landed: `internal/maillink`, `internal/inbox`, `jobtracking.MarkApplied`,
`jobtracking.TrackJob`, and the follow-up record action. It is read in exactly one place —
the per-company response aggregate in `internal/db/queries/insights.sql` — and read there by
company, not by date. Nothing reads one caller's events as a series.

Three constraints come from the surrounding code rather than from this feature:

- **`appevent.TrustedForDayMath` already splits observed from recorded.** Mail sources carry a
  date somebody else set; `user` and `assistant` carry the date the candidate updated their
  board. `TestOnlyMailSourcesAreTrustedForDayMath` fails if a future edit promotes a manual
  source, and pins the whole vocabulary so a new source needs an explicit verdict.
- **The ledger is content-free by construction.** Subjects, bodies and addresses live in
  `emails`, and reaching a body through `GET /me/emails/:id` marks the message read.
- **`GET /me/tracking/:slug` is registered in `internal/handler/gmail.go`** while its static
  siblings (`/viewed`, `/saved`, `/pipeline`, `/swipe`, `/analyses`) are registered in two other
  files. They resolve because Fiber matches in registration order and their `Register*` happens
  to run first.

## Goals / Non-Goals

**Goals:**

- One caller's ledger events, over a date range, in one request.
- A month view that tells the truth about which dates were observed and which were typed.
- A reader that a later event kind can flow through without a redesign.

**Non-Goals:**

- Interview times and any forward-looking view. The ledger has no interview time and neither
  does the mail: `interview_invitation` says an invitation arrived, not when the meeting is.
  That date will come from reading the candidate's own calendar, in its own change.
- Writing anything. This change adds no mutation, no migration and no worker.
- An ICS feed, reminders, or export.

## Decisions

### A new package, `internal/apptimeline`, rather than a method on `jobtracking`

`jobtracking` is organised around mutations of one `(user, job)` pair — apply, save, track,
each an idempotent upsert. This read is addressed by a date range across every application a
user has, joins two tables `jobtracking` never touches, and returns a shape with no mutation
behind it. Folding it in would make one package answer two unrelated questions.

The read lives in a service and not in the handler for the reason the mail stack states and the
ledger spec repeats: the in-app assistant calls services directly with the session owner's id
and issues no HTTP request. "Show me what happened last month" is an obvious assistant tool, and
a rule enforced in a Fiber handler is one that reader never meets.

*Alternative considered:* extend `internal/jobtracking`. Fewer files, but the package starts
doing two things, and the next reader has to find out which half applies to them.

### `GET /me/timeline`, not `/me/tracking/calendar`

Adding a fourth static segment under `/me/tracking/` would be a fourth bet on `Register*` call
order against the `:slug` route in `gmail.go`. Nothing enforces that order, and the failure is
quiet: the parameterised route would swallow `calendar` and answer with a 404 for a job slug
that does not exist. A separate path removes the question rather than answering it.

The name is also honest about scope — the series is not the calendar's private data, and the
same endpoint serves an application's own history and an assistant tool later.

*Alternative considered:* register the static route explicitly before the parameterised one.
That works, and it leaves the next person to discover the constraint by breaking it.

### The server serves moments; the browser decides days

`occurred_at` is `timestamptz` — an absolute instant. Which cell it belongs to depends on the
reader's clock, and only the browser knows that. So the response carries instants, the grid is
built client-side, and the range is fetched with one day of margin on each side. Without the
margin the first and last cells of a month are systematically short.

This makes the server render data-only: `+page.server.ts` fetches, the component arranges. The
initial fetch uses the current month in UTC plus the same margin, which covers the reader's
month whatever their offset; navigating months fetches again.

*Alternative considered:* a `tz` query parameter and server-side grouping. It moves timezone
handling to where there are no tests for it, and puts an IANA zone name in a URL that then has
to be validated and kept in sync with the browser's own idea of the zone.

### `observed` is computed on the server

The response carries a boolean per event, taken from `appevent.TrustedForDayMath(source)`.
Deriving it in the browser would fork the trust rule: the source vocabulary has grown before,
and a browser-side list would keep calling a newly added source observed after the Go side had
already refused it. A pin test asserts the served verdict against `TrustedForDayMath` over the
whole `appevent.Sources` collection, so a source added without a verdict fails the build — the
same device `TestOnlyMailSourcesAreTrustedForDayMath` already uses one layer down.

### The subject is joined, never fetched

`LEFT JOIN emails ON application_events.source_ref` supplies the subject and the message id, and
supplies neither when `emails.deleted_at IS NOT NULL`. The event still comes back: the ledger's
position is that deleting a message hides content and does not un-happen the reply, and a series
that dropped the event would reintroduce exactly the distortion the ledger removed.

Fetching per message is the alternative and it is the trap: `GET /me/emails/:id` marks mail
read, so a reader assembled from it would zero its owner's unread count by browsing.

### One range fetch feeds both the grid and the day panel

Selecting a day filters data already in hand. This is a guard, not an optimisation: a panel that
cannot issue a request cannot reach the message endpoint by a later edit. It also removes a
loading state from every click.

The cost is a larger first payload. For the volumes involved — a heavy month is tens of events,
and a heavy user has under a hundred applications in total — this is not a trade worth making
the other way.

### `kind` crosses the wire as a value, not as an enumerated set

`application_events` splits `kind` from `signal` so that a growing classifier vocabulary is not
a change to a table that must not change. The reader honours the same split: it renders an
unknown kind generically rather than failing, so the interview kind this change deliberately
does not add can arrive without touching the response format or the grid.

### The range is bounded

`from` and `to` are required, `from <= to`, and a span longer than 366 days is a 400. The index
`application_events (user_id, occurred_at)` makes the scan cheap and per-user volumes are small,
so this is hygiene rather than a performance fix — but an unbounded range on an
append-only table is the kind of thing that is cheap now and expensive later.

## Risks / Trade-offs

- **A bulk-apply day overflows its cell.** Twenty applications in one evening is normal
  behaviour. → The cell shows what it can and reports the remainder; the day panel lists all of
  them.
- **A user with no connected mailbox sees a thin calendar.** Their `applied` and `stage_set`
  events are there, and most of the latter are hand-recorded, so the month reads as mostly
  unobserved. → That is accurate, and the empty state points at the board. Connecting a mailbox
  is what fills it in, which the view can say.
- **A larger first payload than a per-day fetch.** → Bounded by the range cap and by real
  volumes; measured against the read-marking hazard it removes, this is the cheaper side.
- **Not a risk, though it looks like one:** connecting a mailbox imports a year of ATS mail in
  one pass. The calendar does not show a wall of events on that day, because `occurred_at` is
  the message's own date — this is precisely what `recorded_at` was separated from it to
  prevent.

## Migration Plan

None. No schema change, no backfill, no worker. The change is additive to the API and the SPA
and can be deployed in the normal order; the route is under `mw.key` like the rest of the
`/me` surface. Rolling back is removing the tab and the route — no data written, so nothing to
undo.

## Open Questions

None blocking. The interview-date source is decided (reading the candidate's own calendar) and
scoped to a later change, where the Google verification limit — the OAuth app is unverified and
restricted-scope, so test users only — is the first thing to confront.
