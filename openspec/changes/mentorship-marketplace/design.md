## Context

The catalogue already holds the vacancy and the company. `internal/engage/referral`
already holds a moderated, *anonymous* insider and a request-to-be-referred. What is
missing is the conversation before the application — and the calendar that makes a
conversation schedulable at all. Nothing in this repository schedules anything: there
is no availability, no slot, no booking, and no concept of a recurring week.

Two nearby things exist and must not be confused with what this builds:

- `internal/application/calsync` reads a *candidate's* Google Calendar to recognise
  interviews their own applications earned, and deliberately discards everything else —
  the schema enforces it (`application_interviews.application_id NOT NULL`). Mentor
  busy-time cannot come from there. What is reusable is the Google grant
  (`gmailsync.CalendarScope`, read-only) and the Calendar HTTP client, in a later change.
- `internal/application/ical` already renders `.ics`, and is reused as-is.

`cal.com` was read (shallow clone at `/Users/i_strelov/Projects/cal.com`, read-only, never
built or executed) as the reference implementation of scheduling. Its data model and
slot pipeline are the basis for the decisions below; where this design departs from it,
the departure is stated and argued.

Two constraints from this specific deployment shape the design more than anything else:
roughly three quarters of the host's traffic is crawlers, so a public endpoint that
computes anything per request is a standing load problem; and the host's nightly
`pg_dump` from 03:00 UTC makes any migration that takes a strong lock a scheduling
question rather than a formality.

## Goals / Non-Goals

**Goals:**

- A named, moderated mentor profile bound to a company in the catalogue.
- A recurring weekly schedule with dated overrides, correct across daylight-saving
  transitions and across the viewer's own timezone.
- Bookable slots computed on read, never stored.
- A booking that two concurrent requests cannot duplicate — guaranteed by the database.
- Confirmation, cancellation, reminders and a post-session review.
- A named, unbuilt seam for Google free/busy sync, and a named, unbuilt seam for money.

**Non-Goals:**

- Payments, pricing, payouts, refunds. No Stripe touch of any kind.
- Google Calendar free/busy sync. The table is created; nothing fills it.
- Rescheduling. Cancel and re-book instead — a reschedule is a cancel plus a book with
  one extra notification, and it can be added later without schema change.
- Multiple session types per mentor, group sessions, recurring mentorship series.
- Team or round-robin scheduling, which is most of `cal.com`'s complexity and none of
  this product's need.

## Decisions

### One session per mentor, not a table of event types

`cal.com` models `EventType` as a first-class table because selling configurable event
types *is* their product. Here, session length, buffers, minimum notice, horizon and
meeting link live as columns on `mentors`.

*Alternative considered:* a `mentor_session_types` table from the start. Rejected as
speculative: it costs a join on every slot computation, a second editing surface, and
a "which type?" step in the booking flow, to serve a distinction ("30-min career chat"
vs "60-min mock interview") no mentor has asked for yet.

*The seam:* when a second type is genuinely needed, those columns move to a
`mentor_session_types` row and `mentor_bookings` gains a `session_type_id`. Slot
computation already takes the session parameters as an argument rather than reading
them from the mentor, so its signature does not change.

### Availability is one table with two row shapes

Directly from `cal.com`: a weekly row carries a weekday, an override row carries a
date, a `CHECK` enforces exactly one of the two. Times are `TIME` — no date, no zone.

*Alternative considered:* two tables, `mentor_weekly_hours` and
`mentor_date_overrides`. Rejected because every read wants both, always, in one
ordered set; two tables means a `UNION` in every query and two writers for one idea.

*Alternative considered:* storing availability as materialised future slots. Rejected
outright — it makes a timezone change, a session-length change or a buffer change into
a backfill, and it is the mistake `cal.com` explicitly does not make.

**The empty override closes a day.** An override row whose `start_time` equals its
`end_time` produces a zero-length range, which survives long enough to displace the
weekly rows for that date and is then dropped. This is `cal.com`'s trick verbatim
(`date-ranges.ts`: "remove 0-length overrides that were kept to cancel out working
dates until now"). It means "I am away on the 16th" is one inserted row, not a
deletion and re-creation of the weekly schedule.

### The mentor's timezone resolves the stored time — never the reverse

A stored `18:00` is resolved through the mentor's IANA zone into an instant, per date.
The expansion happens in the mentor's zone and is converted to UTC afterwards.

This is the only ordering that survives daylight saving. Converting `18:00` to UTC once
and repeating it weekly shifts every slot by an hour for half the year — the bug is
silent, seasonal, and impossible for a mentor to diagnose. Go's `time.Date` normalises
the two pathological wall-clock cases (a local time the zone skips in spring, and one
it repeats in autumn); both get an explicit test, because normalisation being *correct*
and being *what we want* are different claims.

An already-confirmed booking stores absolute instants, so a mentor changing their
timezone moves future availability and moves no booking.

### Slot computation is a pure function

The pipeline — clamp the window, expand the schedule, subtract busy intervals and
buffers, slice, drop what the notice period forbids, express in the viewer's zone —
takes schedule rows, session parameters, busy intervals and a `now`, and returns slots.
No database handle, no clock, no network. It lives in its own file in
`internal/engage/mentorship`.

**The minimum notice filters slots; it does not clamp the window.** The horizon does
clamp it, because it only ever moves the far end. Clamping the near end by `now +
notice` would move the *anchor* the slicing counts from: a free range starting at 14:37
because that is when the notice period elapses yields slots at 14:37, 15:37, 16:37,
rather than the mentor's own 18:00 and 19:00. So the grid is anchored to the schedule,
and slots earlier than `now + notice` are dropped after slicing. The specs already say
it this way ("SHALL offer no slot beginning sooner than"); this paragraph corrects the
earlier wording here, which said "clamp" of both.

This is what makes the hard cases testable at all: daylight-saving days, half-open
boundaries, buffer arithmetic, and horizon clamping are table-driven unit tests that
run in milliseconds without Docker. Only the *fetching* of the inputs touches Postgres.

### The non-overlap guarantee lives in Postgres, not in Go

```sql
ALTER TABLE mentor_bookings ADD CONSTRAINT mentor_bookings_no_overlap
  EXCLUDE USING gist (
    mentor_id WITH =,
    tstzrange(starts_at, ends_at, '[)') WITH &&
  ) WHERE (status = 'confirmed');
```

`btree_gist` is required so a scalar (`mentor_id`) and a range share one GiST index. It
is **verified available on prod** — `pg_available_extensions` reports version 1.8, not
yet installed — so `CREATE EXTENSION IF NOT EXISTS btree_gist` in the migration will
succeed, as `pg_trgm` and `vector` already have.

*Alternative considered:* `cal.com`'s approach — re-check availability inside the
booking transaction. Rejected: it is a check-then-act race, and their own code carries
it. Two requests both pass the check, both insert, and one mentor has two people in the
same hour. A constraint closes the window with no lock and no retry loop.

The `WHERE status = 'confirmed'` predicate is load-bearing: without it a cancelled
19:00 booking blocks 19:00 forever.

The application still re-derives the slot before inserting — not for the race, but so
that the ordinary refusals (paused mentor, elapsed notice, removed availability) return
a reason rather than a constraint violation. The constraint's violation is caught and
mapped to the same "no longer available" outcome the stale-page path returns, so the
loser of a race and a stale tab are indistinguishable to the client. This mirrors
`referral`'s existing pattern of mapping a unique violation to a domain sentinel.

### The public slot endpoint is cached and rate-limited from day one

Cache key: `(mentor_id, window, viewer timezone)` in Redis, short TTL, invalidated on
booking, cancellation, and any edit to availability or session parameters.

The timezone belongs in the key. Without it the first viewer's zone is served to
everyone, which is the class of bug that reads as "the site shows me the wrong times"
and is never reproduced by the developer.

Cache unavailability degrades to direct computation — the existing `catalogstats`
precedent: a read never fails because a cache is down.

*Alternative considered:* no cache, add one when load appears. Rejected on measured
grounds specific to this host: crawlers are most of its traffic, and a public URL that
computes a month of slots per hit is exactly what they will hit hardest. This is not
premature optimisation; it is the known load profile.

### `job_id` gets a `NOT VALID` foreign key, validated separately

`mentor_bookings.job_id` references `jobs`, which holds ~11M rows. `ALTER TABLE ... ADD
CONSTRAINT ... FOREIGN KEY` takes a `SHARE ROW EXCLUSIVE` lock on **both** tables,
blocking writes to `jobs` for the duration — and this host already has migrations fail
with `55P03` against the nightly dump and the similar-jobs worker.

The constraint is therefore added `NOT VALID` (a brief lock, no scan) and validated in
a following statement, which takes only `SHARE UPDATE EXCLUSIVE`. The new table is
empty, so validation is instant. squawk will flag the pattern; the suppression carries
this reason beside it.

### Booking messages are transactional and bypass the notification rule

`notification-settings` currently says one flag governs *every* notification with no
per-kind override. Taken literally, a user who silenced saved-job reminders would also
miss the reminder for a session a mentor is holding an hour for.

The delta redraws the rule's boundary rather than punching a hole in it: the rule
governs notifications the *system* originates about the user's activity; it does not
govern messages about a commitment the *user* made and a second person is relying on.
Confirmations, cancellations and pre-session reminders are on the far side of that line,
alongside the mailed verification code — which was never under the rule either.

### Reminders are a run-once cron worker, idempotent per (booking, offset)

`cmd/mentorship-remind` follows the repository's cron-worker shape: needs
`DATABASE_URL`, exits non-zero on failure, and is a clean no-op without a mail
transport. A sent reminder is recorded per `(booking, offset)`, so re-running sends
nothing twice and a missed window does not fire late — a "your session starts in one
hour" delivered after the session is worse than silence.

*Alternative considered:* folding this into an existing notification worker. Rejected —
that worker is gated on the account notification rule this deliberately sits outside,
and threading an exception through it would put the exception in the wrong place.

### Placement

`internal/engage/mentorship`, block `engage` (layer 7), which may already reach
`identity`, `candidate`, `job` and `application`. HTTP in
`internal/api/handler/mentorship.go`. **The package must be added to the table in
`internal/platform/arch/layering/blocks.go`** or both layering guards fail — a package
in neither is a guard failure by design.

Moderation reads `referral_offers` for corroborating evidence only. Both packages are
in `engage`, so the import is legal; it is one read of an approved-offer existence
check, and it is not a gate.

## Risks / Trade-offs

- **A supply-side marketplace with no supply.** → Mentors are onboarded by hand by the
  owner, who has said so explicitly. The moderation queue is built for a human, not for
  volume. If nobody signs up, the calendar was still the cheap half; the expensive half
  (payments, sync) was deliberately not built.

- **The public slot endpoint is a computed, crawler-facing surface.** → Redis cache with
  a timezone-bearing key, rate limit, and a horizon that bounds how much any single
  request can ask for. Watch it after deploy the way per-route traffic is already watched.

- **Daylight-saving bugs are seasonal and silent.** → Expansion happens in the mentor's
  zone; the two pathological wall-clock cases are explicit tests; the pure function makes
  them cheap to write and to keep.

- **The FK to `jobs` can fail the migration with `55P03`.** → `NOT VALID` plus a separate
  `VALIDATE`. Deploy outside the 03:00 UTC dump window regardless; the repository's
  existing guidance on stopping the similar-jobs worker applies to anything touching `jobs`.

- **A migration number collision.** → Three files already collided on `0144` in `main`.
  Take the next free number immediately before opening the PR, and re-check after any
  rebase; ordering resolves alphabetically and there is no gate.

- **A mentor's public slots leak their private calendar's shape** once Google sync
  lands. → Inherent to any booking page and accepted by every product in this category.
  Mitigated by storing only `(start, end)` with no title or attendee, by the booking
  horizon, and by the mentor's own choice of published hours.

- **A mentor who stops showing up.** → Out of scope for this change; reviews make it
  visible and pausing makes it self-correcting. A no-show status is a later concern.

## Migration Plan

1. One migration file, number claimed immediately before the PR opens:
   `CREATE EXTENSION IF NOT EXISTS btree_gist`; the five tables; the `EXCLUDE`
   constraint; the `jobs` FK as `NOT VALID` followed by `VALIDATE CONSTRAINT`.
   Deploy outside the 03:00 UTC dump window.
2. `make sqlc` after the queries land; the pre-commit hook and CI both regenerate and
   diff, so a query edited without regenerating cannot ship.
3. Add `internal/engage/mentorship` to `internal/platform/arch/layering/blocks.go`
   in the same change.
4. `cmd/mentorship-remind` needs a systemd unit and timer on the host. `release.sh`
   flips the app and never touches a unit, so the unit is copied to the host by hand;
   until then the feature works and reminders simply never fire.
5. **Rollback** is to stop offering the entry points and leave the schema in place. The
   tables are additive and referenced by nothing else; dropping them would destroy
   booking history for no benefit. The `btree_gist` extension is likewise harmless left
   installed.

## Open Questions

- **Where does the mentor's display name and avatar come from** — the account profile,
  or fields on the mentor profile itself? A mentor may reasonably want a different
  photo here than on their CV. Defaulting to the account profile with an override is
  the likely answer; confirm before building the editing surface.
- **Does the vacancy entry point count as a CTA that competes with "Apply"?** There is
  an open change (`job-page-cta-hierarchy`) on exactly that page. Coordinate placement
  with it rather than adding a fourth button independently.
- **Reminder offsets (24h, 1h) are a guess.** Ship them fixed; revisit once real
  no-show data exists rather than making them configurable up front.
