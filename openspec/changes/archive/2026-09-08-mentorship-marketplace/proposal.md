## Why

A seeker reading a vacancy has one question the posting never answers: what is it
actually like to work there, and am I a plausible candidate? The catalogue already
carries the vacancy and the company; `employee-referrals` already carries a verified
insider willing to help — but only anonymously, and only as a referral hand-off. There
is no way to simply *talk* to that person for half an hour before applying.

Mentorship closes that gap on the asset the product already has: catalogue traffic and
a pool of moderated insiders. A booked conversation is also the natural front of the
referral funnel — the insider who spoke to a candidate is the insider best placed to
refer them, and that flow already exists.

## What Changes

- **A public mentor profile.** An insider at a company in the catalogue publishes a
  named, moderated profile: photo, role, company, topics, languages, session length.
  Unlike a referral offer, a mentor is **not anonymous** — a mentorship marketplace with
  faceless cards gives a seeker nothing to choose between.
- **Manual moderation.** A profile is created by its owner and reaches `approved` only
  by a moderator's hand. No automated employment verification is built; the approved
  `referral_offers` row is corroborating evidence a moderator may consult, never a gate.
- **A weekly availability schedule with date overrides.** The mentor states a recurring
  week ("Tue and Thu, 18:00–20:00") in their own IANA timezone, plus one-off overrides
  for a specific date — including an empty override, which closes that day.
- **Bookable slots, computed on read.** Slots are never stored. A pipeline expands the
  schedule into concrete ranges, subtracts busy time and buffers, and slices what
  remains. Anyone may view slots; only an authenticated user may book.
- **Bookings that cannot overlap.** The no-double-booking rule is enforced by a Postgres
  `EXCLUDE` constraint over a `tstzrange`, not by a re-check in application code —
  the database refuses the second concurrent booking outright.
- **Confirmation, cancellation and reminders.** Both parties get an email carrying an
  `.ics` invitation and the meeting link. Either party may cancel before the start,
  which notifies the other and frees the time. Reminders fire 24h and 1h before.
- **A review after the meeting.** The seeker may rate and comment on a completed
  session; the aggregate shows on the mentor's public profile.
- A `mentor_busy_intervals` table is created and left **empty**: Google Calendar
  free/busy sync is a separate follow-up change. Only the seam is built here.
- **No money.** No pricing, no payouts, no Stripe. Sessions are free. The seam is named
  in design.md and deliberately not built.

## Capabilities

### New Capabilities

- `mentor-profile`: who a mentor is, how a profile is created, moderated, published,
  paused and withdrawn; the public mentor directory and the entry points from a
  vacancy and a company page.
- `mentor-availability`: the weekly schedule, date overrides, timezone handling, and
  the rules that turn all of it into bookable slots — buffers, minimum notice, booking
  horizon, and what makes a slot offerable.
- `mentor-booking`: the lifecycle of one session — booking, the non-overlap guarantee,
  confirmation and calendar invitation, cancellation by either party, reminders,
  completion, and the post-session review.

### Modified Capabilities

- `notification-settings`: the existing rule states that one account-level flag governs
  **every** notification, with no per-kind override. That rule must now say what it
  actually means: it governs notifications the *system* originates about a user's
  activity (saved-job reminders, follow-up and interview-prep nudges). It SHALL NOT
  govern transactional messages about a session the user is A PARTY TO — a booking
  confirmation, a cancellation, or a pre-session reminder — and that covers BOTH sides.
  "A commitment the user made themselves" would have been the wrong test: a mentor did
  not make the booking, the seeker did, and the mentor is the party holding an hour for
  it. A user who silenced nudges still turns up to the meeting, and neither side's time
  is wasted. This is a clarification of scope, not a new override.

## Impact

**New code**
- `internal/engage/mentorship` — the domain: profile, schedule, slot computation,
  booking lifecycle. The slot pipeline is a pure, dependency-free file (no database,
  no clock beyond an injected one, no network) so its timezone and boundary cases test
  without Postgres.
- `internal/api/handler/mentorship.go` — HTTP: public read routes, authenticated
  booking and mentor-cabinet routes, moderator routes.
- `cmd/mentorship-remind` — a run-once-and-exit cron worker that sends the 24h and 1h
  reminders. Needs `DATABASE_URL`; a missing mail transport makes it a clean no-op.
- `internal/platform/arch/layering/blocks.go` — the new package MUST be added to the
  block table, or both layering guards fail.

**Schema** — one migration adding `mentors`, `mentor_availability`, `mentor_bookings`,
`mentor_busy_intervals`, `mentor_reviews`, and `CREATE EXTENSION IF NOT EXISTS
btree_gist` (verified available on prod: version 1.8, not yet installed). Followed by
`make sqlc`.

**Reused, unchanged** — `internal/application/ical` for the invitation;
`internal/engage/notify` and the SES mail path for delivery; `companies.slug` as the
company key; `internal/platform/cache` (Redis) for the slot cache.

**Frontend** — `web/`: a public mentor directory and profile-with-booking page, a
mentor cabinet under `/my/`, a booking list for the seeker, and a moderation queue
beside the existing referral queue. Primitives from `design-system/`.

**Load** — the public slot endpoint is computed per request on a host where roughly
three quarters of traffic is crawlers. It is cached in Redis and rate-limited; the
cache key includes the viewer's timezone, or a visitor in Tokyo is served Berlin's
slots.

**Not in scope** — payments; Google Calendar free/busy sync; rescheduling (cancel and
re-book instead); group sessions; multiple session types per mentor.
