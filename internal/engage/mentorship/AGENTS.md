# Mentorship conventions

## Scope
The mentorship marketplace: a named, moderated insider at a company in the catalogue
publishes when they are free, and a seeker books half an hour of it. The slot engine is
in slots.go/expand.go/override.go/subtract.go/slice.go; the domain services in
profile.go, booking.go, remind.go and schedule_service.go; the sqlc adapter in
repository.go and booking_repository.go; the mail path in notify.go and invite.go. HTTP
lives in internal/api/handler/mentorship*.go — the public surface in its own
`registerPublic`, because the public-read-limiter guard drives every GET a register
mounts and requires the limiter FIRST, which the cabinet routes cannot satisfy.

This is referral's opposite number. `internal/engage/referral` keeps its insider
anonymous because a referral is a favour asked of a stranger; a mentor is chosen, so a
mentor has a name — `mentors.display_name`, required, and the reason the two features are
not one. They share the company key and nothing else.

## Always true

- **A stored availability time carries no zone and no date.** It is resolved through the
  mentor's own IANA zone, per date, and only then becomes an instant. Convert once and
  repeat weekly and every slot moves by an hour for half the year, silently, and no mentor
  could diagnose it (expand.go's `resolve`).

- **The day cursor is a `Date`, not an instant, and steps through UTC.**
  `America/Santiago`, `America/Havana` and `Atlantic/Azores` move their clocks AT
  midnight, so once a year that midnight does not exist and Go normalises it BACKWARDS to
  23:00 of the previous date. A cursor built at midnight stops advancing — an infinite
  loop on an unauthenticated endpoint (schedule.go:`Date.next`, and the regression tests
  in midnight_test.go).

- **The slot grid is anchored to the SCHEDULE, never to `now`.** `Slots` moves the
  window's near end DOWN to a day boundary; the minimum notice then filters slots.
  Clamping the near end to `now` instead offers 18:33, 19:33, 20:33 for a mentor who
  stated 18:00 — and this was got wrong TWICE: the first fix only raised a bound that was
  already earlier, which the HTTP default (`from = now`) never is. Tests that supply
  midnight cannot see it; anchor_test.go now calls the engine the way the handler does.

- **An empty dated override CLOSES its date, and beats every other override on it.** That
  last part departs from cal.com, where an empty override is a zero-length range filtered
  out alongside the rest. The asymmetry decides it: a slot wrongly withheld costs a
  booking, a slot wrongly offered pulls a mentor out of a declared holiday.

- **Two confirmed bookings for one mentor cannot overlap, and Postgres says so** — an
  `EXCLUDE USING gist` over a `tstzrange` with half-open bounds, matching
  `Interval.Overlaps`. The service still re-derives the slot before inserting, but for a
  different job: to give an ordinary refusal a REASON. **Its violation is SQLSTATE 23P01,
  not a unique violation's 23505** — `pgerr.IsUniqueViolation` does not match it, and
  missing it turns "somebody took that hour" into a 500.

- **One answer for every ordinary way a booking fails to land.** Taken, withdrawn from the
  schedule, inside the notice period, past the horizon, or lost to a race — all
  `ErrSlotUnavailable`. To a seeker a stale tab and a lost race are one event, and telling
  them apart tells a stranger which.

- **Withdrawal MARKS the profile, it does not delete it.** Bookings and reviews reference
  it `ON DELETE CASCADE`, so a delete erases every session that ever happened. Future
  bookings are cancelled and their seekers told FIRST; the past survives.

- **The publication predicate lives in the SQL** (`approved AND NOT paused`), in every
  query that reads publicly, so the directory and the profile read cannot disagree. There
  is deliberately no Go helper saying the same thing, and no separate "does this company
  have a mentor?" call — the directory narrowed to a company answers that.

- **The publication check runs BEFORE the slot cache.** Otherwise pausing does nothing
  until the entries expire, at exactly the moment somebody uses that button.

- **The cache key includes the VIEWER'S ZONE.** The stored value carries wall-clock times
  already rendered; a Tokyo visitor served Berlin's entry sees times that are wrong and
  look plausible. There is no explicit invalidation — the shared cache has no deletion,
  and a booking would have to invalidate every overlapping window in every zone — so a
  one-minute expiry bounds it and the BOOKING PATH DOES NOT READ THE CACHE.

- **Booking confirmations, cancellations and reminders bypass the account notification
  rule.** That rule governs what the system originates about a user's activity; these
  concern a session the user is A PARTY TO, and the test is participation rather than
  authorship — a mentor did not make the booking.

- **Both parties are told in their OWN zone.** A zone that does not resolve falls back to
  UTC and says so, never to the other party's: a time labelled with somebody else's zone
  is undetectably wrong.

- **A reminder is CLAIMED before it is sent**, and the claim is released if delivery
  fails. Sending first and recording after sends twice on any failure; holding the claim
  through a failure loses the reminder permanently. A missing "your session starts in an
  hour" costs somebody the session; a duplicate costs them a duplicate.

- **`invite.go` WRITES iCalendar; `internal/application/ical` READS it.** They share no
  code. The invitation must be an ATTACHMENT with `method=REQUEST` — as a link it is a
  file to download — and a cancellation must carry the SAME UID, or it adds a second event
  instead of removing the first.

## How it works

`New(repo, Config{Notifier, Cache, Now})`. A nil Notifier is a deployment with no mail
transport and a nil Cache one with no Redis; both disable a capability without disabling
the feature, and `Now` is the injectable clock that makes the slot engine a pure function.

The engine takes `SessionParams` as an argument rather than reading them off a mentor,
which is the seam a per-session-type table would use: the five columns move, the engine's
signature does not.

`cmd/mentorship-remind` drains the reminders. `deploy/systemd/freehire-mentorship-remind.*`
is its unit and timer — **not installed by `release.sh`**, which never touches a unit, and
its binary is not built by it either.

## Testing

The fake repository in fake_repo_test.go describes the contract; it cannot prove it. Three
defects hid behind it: a booking whose confirmation reached nobody (the fake filled fields
the SQL does not return), a withdrawal that erased history (a fake has no cascades), and a
paused mentor served from cache. Anything that is a property of the SCHEMA belongs in
`internal/platform/db/mentorship_integration_test.go`.

There is a THIRD layer, and it exists because the first two leave a gap between them.
`internal/platform/db`'s tests call the generated query directly: they prove the columns
come back, not that anything reads them. The fake proves the domain struct is filled,
because it fills it. A field the query SELECTS and the mapping forgets is invisible to
both — which is how a seeker's session list shipped naming nobody, while the
single-session read set the same field and looked right. `repository_integration_test.go`
drives `QueriesRepository` against a real Postgres, so it sees the seam itself. A test
written for that defect in `internal/platform/db` passes with the fix removed; this one
does not, and that difference is the whole reason the file is here.

Run the slot-engine tests with `-count=1`: the layering guard next door reports `ok
(cached)` on a package it has never seen, and the habit is worth keeping here too.
