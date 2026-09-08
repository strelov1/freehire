## 1. Slot engine (pure, no database)

The hard part first, and the part that needs no schema. Every task here is table-driven
unit tests plus the function they drive; none of it needs Docker or Postgres.

- [x] 1.1 Create `internal/engage/mentorship` and register it in the block table at
      `internal/platform/arch/layering/blocks.go`; confirm `golangci-lint run` and the
      `layering` test both pass on the empty package
- [x] 1.2 Define the slot-engine value types: an availability row (weekly or dated), the
      session parameters (duration, buffers, minimum notice, horizon), a busy interval,
      and a slot — all zone-aware, none of them database rows
- [x] 1.3 Expand a schedule into concrete ranges in the mentor's zone, then to UTC;
      tests cover a plain week, a window spanning the spring transition, and one
      spanning the autumn transition
- [x] 1.4 Apply overrides so a dated row replaces its whole day, and an empty override
      (equal start and end) closes it; tests cover narrowing a day, closing a day, and
      leaving other days untouched
- [x] 1.5 Subtract busy intervals widened by buffers, with the after-buffer of an
      existing engagement and the before-buffer of a prospective one both applying;
      tests cover the summed gap and a cancelled booking freeing its time
- [x] 1.6 Slice free ranges into slots on half-open boundaries, emitting a slot only
      where the full session fits; tests cover back-to-back sessions not conflicting
      and a gap shorter than the session yielding nothing
- [x] 1.7 Clamp the window by the booking horizon, drop slots inside the minimum-notice
      period after slicing (the grid anchors to the schedule, not to now), and express results in
      the viewer's zone with an unknown zone falling back to UTC and saying so; tests
      cover a slot inside the notice period, a window wider than the horizon, and the
      fallback
- [x] 1.8 Assert the whole pipeline is deterministic: same inputs, same slots, twice
- [x] 1.9 Act on the group-1 review. Four defects it found, all now fixed with a test
      that fails without the fix (each verified by mutation):
      **(a)** the day cursor was an instant built at midnight, which does not exist once
      a year in `America/Santiago`, `America/Havana` and `Atlantic/Azores` — Go
      normalises it backwards and the walk never advanced, an infinite loop on an
      unauthenticated endpoint. The cursor is now a `Date`, stepped in UTC, which
      removes the class rather than the instance and deletes `startOfDay`/`nextDay`/
      `startOfDate` outright.
      **(b)** dated overrides landed on the previous date in those same zones — same
      root cause, fixed by the same change.
      **(c)** the window's near end was clamped to `now`, which moved the slot grid
      every minute (18:33, 19:33, …) against a decision design.md, tasks.md and the
      package comments all state. Clamped to the start of the mentor's day instead.
      **(d)** overlapping or duplicate availability rules emitted duplicate, unsorted
      slots — `expandSchedule` now merges free ranges.
      Also: `"Local"` as a viewer zone returned the SERVER's clock labelled `"Local"`;
      `Slots` returned a nil slice that would marshal as `null`; `subtractOne` could emit
      zero-width remnants; and two comments claimed things that were not load-bearing.
- [x] 1.10 Add `_ "time/tzdata"` to `cmd/server` (~450 KB). The runtime image
      (`debian:stable-slim`) does ship the zone database — verified — so nothing is
      broken today; the point is that this is the first feature to hard-depend on it,
      and if the base image ever changes every mentor zone silently becomes UTC, which
      is indistinguishable from correct behaviour in the response. Belongs with the HTTP
      layer, group 7.

## 2. Schema and generated queries

- [x] 2.1 Write the migration (claim the number immediately before opening the PR):
      `CREATE EXTENSION IF NOT EXISTS btree_gist`, then `mentors`,
      `mentor_availability`, `mentor_bookings`, `mentor_busy_intervals`,
      `mentor_reviews`, with every `CHECK` the specs require — plus
      `mentor_booking_reminders`, whose composite key IS the reminder worker's
      idempotency (task 6.2) and which would otherwise need a second migration
- [x] 2.2 Add the `EXCLUDE USING gist` non-overlap constraint on `mentor_bookings`,
      predicated on `status = 'confirmed'`
- [x] 2.3 Add the `jobs` foreign key in its own statement. NOT `NOT VALID`: that skips
      the scan of the referencing table (empty here) and takes the same lock on `jobs`
      regardless — see the corrected decision in design.md. The runner's 5s
      `lock_timeout` is what bounds the `55P03` risk
- [x] 2.4 Run `pnpm check:sql` and confirm the migration passes both squawk passes —
      done, 0 issues, with per-column `prefer-bigint-over-int` suppressions carrying
      their argument beside them
- [x] 2.4a Apply the whole migration history to a throwaway Postgres and probe every
      constraint by hand: overlapping bookings rejected, back-to-back accepted, a
      cancellation freeing its slot, both availability CHECKs, the review and reminder
      keys, and one profile per account
- [x] 2.5 Write the sqlc queries (profile CRUD and moderation, availability CRUD,
      booking write and lists, reminder claim, review upsert) and run `make sqlc` — 28
      methods. Two shapes worth noting: the publication predicate (`approved AND NOT
      paused`) lives in the QUERIES, not in the service, so the directory and the
      vacancy-page check cannot disagree about what "has a mentor" means; and
      `CancelMentorBooking` packs all three guards — confirmed, not yet started, and the
      canceller is one of the two parties — into one statement, so a stranger's attempt
      is indistinguishable from a booking that does not exist
- [x] 2.6 Integration test the `EXCLUDE` constraint directly: two concurrent inserts for
      one slot leave exactly one confirmed row, and a cancelled row does not block its
      own time — plus the cancellation guards, the reminder claim's idempotency, the
      late-reminder suppression, the publication predicate and the directory's
      "NULL means unfiltered" filters

## 3. Profile domain and moderation

- [x] 3.1 Profile service: create, read, update, with one profile per account and a
      company that must exist in `companies`; sentinels carry their HTTP mapping in
      comments, following `referral`'s convention
- [x] 3.2 Moderation: pending queue oldest-first, approve and reject recording the
      deciding moderator and time, and an approved `referral_offers` row surfaced as
      evidence but never as a gate
- [x] 3.3 Pause and resume without moderation, leaving confirmed bookings standing
- [x] 3.4 Withdrawal: cancel every confirmed future booking, notify each seeker, retain
      past bookings as history
- [x] 3.5 Public directory: approved and unpaused only, narrowable by company, topic and
      language, reporting unread parameters in `meta.ignored_params`

## 4. Booking domain

- [x] 4.1 Booking creation: authenticated only, never one's own profile, re-deriving the
      slot at write time and refusing with a named reason
- [x] 4.2 Map the `EXCLUDE` violation to the same "no longer available" sentinel a stale
      page gets, so a race and a stale tab are indistinguishable to the client. Note the
      trap this uncovered: an EXCLUDE violation is SQLSTATE **23P01**, not a unique
      violation's 23505, so `pgerr.IsUniqueViolation` does not match it and the refusal
      would have surfaced as a 500. `pgerr` gained `IsExclusionViolation` and
      `ExclusionViolationConstraint`
- [x] 4.3 Random booking identifiers, and the `job_id` reference that outlives its vacancy
- [x] 4.4 Cancellation by either party before the start, recording who and when,
      refusing a past or already-cancelled booking, and refusing a stranger without
      revealing whether the booking exists
- [x] 4.5 Per-party session lists, split upcoming and past, each readable only by its owner
- [x] 4.6 Completion and review: one review per completed booking, seeker only, editable
      without duplicating, never for a cancelled session; aggregate rating and count on
      the profile

## 5. Notifications

- [x] 5.1 Confirmation to both parties, each in their own zone, carrying the meeting link
      and an `.ics`; delivery best-effort, a failure logged and never undoing the booking.
      **NOT from `internal/application/ical`** — that package PARSES incoming invitations
      for the calendar sync and the mail reader, and writing one shares no code with
      reading one. The generator lives in `mentorship/invite.go`; if a second feature ever
      needs to write invitations, that file is what moves.
      `emailnotify` gained `SendWithAttachments` (SES raw MIME): a calendar invitation is
      only an invitation when it is an attachment with `method=REQUEST` — as a link it is
      a file to download, and no client offers "add to calendar" for it
- [x] 5.2 Cancellation notice to both parties, carrying a CANCEL invitation with the SAME
      UID — a different one adds a second event instead of removing the first
- [x] 5.3 Assert in test that booking messages are delivered with the account
      notification rule disabled, and that saved-job reminders and nudges still are not.
      In `internal/platform/db/mentorship_integration_test.go`, because both halves are
      properties of the SQL and a fake repository would record the assumption instead:
      one account with the rule EXPLICITLY off (an absent row is the never-configured
      default, which is enabled, and would prove nothing), whose session still appears on
      `ListBookingsDueForReminder` while `GetReminderForDelivery` and
      `GetNudgeForDelivery` both still report the rule off. It fails the day somebody
      joins `notification_settings` into the booking-reminder page.

## 6. Reminder worker

- [x] 6.1 `cmd/mentorship-remind`: run-once-and-exit, needs `DATABASE_URL`, a clean
      no-op without a mail transport, non-zero exit on failure. The no-op must not open
      the pool at all — claiming a reminder it could not deliver would mark it sent
      forever
- [x] 6.2 Send 24h and 1h reminders idempotently per `(booking, offset)`; tests cover a
      re-run sending nothing twice, a cancellation stopping a pending reminder, and a
      missed window not firing late. The CLAIM happens before the send: a worker that
      sends first and records afterwards sends twice whenever the second step fails
- [x] 6.3 Write the systemd unit and timer into `deploy/`. Two host facts they carry:
      the unit reads `/opt/freehire/.env.notify` as well as `.env`, because the mail
      credentials live only there and a worker missing them soft-skips silently forever;
      and the timer is `Persistent=false`, unlike the reconciling workers, because a
      replayed window would fire reminders for sessions that have already started
- [ ] 6.4 On deploy: build the binary on the host (`release.sh` builds the API, not every
      command in `cmd/`) and copy the unit and timer across by hand — `release.sh` never
      touches a unit. Until both are done the feature works and reminders simply never
      fire

## 7. HTTP layer

- [x] 7.1 Public routes: mentor directory, mentor profile, slot listing — no auth. They
      live in their own `registerPublic`, because the public-read-limiter guard drives
      every GET a register mounts and requires the limiter FIRST — which the cabinet
      routes cannot satisfy, since auth must run before them
- [x] 7.2 Slot cache keyed by `(mentor, window, viewer timezone)`, degrading to direct
      computation when the cache is unreachable. **NOT invalidated explicitly** — the
      shared cache interface has no deletion, and a booking would have to invalidate
      every overlapping window in every viewer's zone, a set the writer cannot
      enumerate. A one-minute expiry bounds the staleness and the booking path does not
      read the cache, so a taken hour may be offered for up to a minute and is refused
      the moment somebody takes it. The spec now says this rather than the invalidation
      it originally asked for
- [x] 7.3 Rate limit the slot endpoint, on the real route — its own budget, below the
      shared public-read one, so a robot walking a calendar cannot exhaust what the rest
      of the site reads on
- [x] 7.4 Authenticated routes: book, cancel, my sessions, review; mentor cabinet routes
      for profile and availability
- [x] 7.5 Moderator routes behind the existing moderator gate, beside the referral queue
- [x] 7.6 One error switch mapping every domain sentinel to its status, following
      `internal/api/handler/referrals.go`

## 8. Frontend

- [x] 8.1 Public mentor directory page with company, topic and language filters.
      `/mentors`, server-rendered for the current filters so a shared link renders
      filtered. The query is whitelisted to the three params the endpoint reads, the same
      rule `/companies` holds — a param it does not read would widen the answer while the
      address bar still claimed it narrowed it. The filter OPTIONS come from an
      unfiltered read: a narrowed list cannot offer what it just excluded, so building
      the controls from the visible rows makes every filter a one-way door. Filtering
      itself stays on the endpoint — a copy of the publication predicate in the browser
      is the drift `mentor-profile` warns about. A selected value the options no longer
      carry is offered as its own option, because a `<select>` whose value names no
      `<option>` renders BLANK and then lies about what the page is showing.
- [x] 8.2 Public mentor profile with the booking calendar, sending the browser's
      resolved timezone and rendering slots in it. The profile is server-rendered; the
      SLOTS deliberately are not — the server does not know the viewer's zone, so
      rendering them would mean showing UTC and swapping every visible hour after
      hydration. Booker state lives in `?month`/`?date`/`?slot` as cal.com's does, which
      is what survives the sign-in redirect a signed-out seeker is about to take.
      **Nothing in the day/time path constructs a `Date`**: `local_start` is already in
      the viewer's zone, and re-deriving it here re-interprets it against the browser's —
      Tokyo's 16th at 01:00 is still the 15th in UTC, so the slot files under the day
      before the one offered. The grid is integer arithmetic (Sakamoto) for the same
      reason. The UTC offset is rendered on every slot: on the autumn transition two
      slots share a wall clock and differ only there. Re-asks every 60s and on tab
      focus — 60s because the endpoint caches a window for a minute, so a faster poll
      spends the rate limit for an identical answer. No reservation system, unlike
      cal.com: the `EXCLUDE` constraint refuses the second booking outright.
- [x] 8.3 Booking confirmation and cancellation flows, and the seeker's session list.
      The chosen hour lives in the URL, so the sign-in bounce returns to it — and the
      sign-in link is built from `location.search`, not `promptSignIn()`, which reads
      `page.url` and would hand back a returnTo missing the very slot it is carried for.
      A booked session carries only its instants, so the zone is applied in the browser;
      that is not the slot trap, and the comment says why. The cancel control is hidden
      rather than shown-and-refused, tested on both sides of the start instant.
- [x] 8.4 Mentor cabinet under `/my/`: profile editing, weekly schedule, date overrides,
      and the mentor's own session list. One `/my/mentorship` section for both sides of
      the marketplace; the offer-to-mentor form appears only when asked for. The week is
      edited and saved whole, mirroring the endpoint. **A backend gap surfaced here and
      was fixed rather than worked around**: the owner's read did not carry the buffers,
      notice or horizon, so a whole-object save after correcting a headline silently
      reset them to the form's defaults. `toOwnMentorResponse` now returns them, as
      pointers — `omitempty` cannot tell a zero buffer from a field that is not yours.
- [x] 8.5 Moderation queue screen beside the referral queue — a fifth tab of the existing
      hub, mirrored in `?tab=` like the rest. The approved referral offer is rendered as
      evidence and nothing acts on it, which is what the spec means by "never a gate".
- [x] 8.6 Entry point from the vacancy and company pages, rendered only where the
      company has an approved unpaused mentor. Asked from the DIRECTORY narrowed to that
      company, never a separate "has a mentor?" endpoint — a second way to ask is a
      second copy of the publication predicate. Asked in the BROWSER, not in `load`:
      the job page is the busiest surface here and about three quarters of this host's
      traffic is crawlers, so a server-side call would spend a request on every bot fetch
      to answer a question no bot acts on. Placed beside the referral block rather than
      as a fourth button, since `job-page-cta-hierarchy`
- [x] 8.7 Review submission after a completed session. Offered only to the SEEKER, and
      the wire never says which party is reading — what it says is that `seeker_email`
      reaches the mentor alone, so its absence identifies the reader. Indirect, and
      therefore tested rather than assumed.

## 9. Verification

- [ ] 9.1 `gofmt -l .` prints nothing; `go vet ./...`, `go test ./...`,
      `go vet -tags=integration ./...` all pass
- [ ] 9.2 `go test -tags=integration ./...` passes with Docker available
- [ ] 9.3 `pnpm --dir web lint` and the web test suite pass; `pnpm check:links` passes
- [x] 9.4 Write `internal/engage/mentorship/AGENTS.md` covering what is always true here:
      the empty-override trick, the zone-resolution order, the `EXCLUDE` constraint and
      why the application still re-derives, the transactional-notification boundary,
      and the two named-but-unbuilt seams (money, Google sync). Linked from `CLAUDE.md`'s
      module table and `internal/engage/AGENTS.md`; `deploy/AGENTS.md`'s "five workers
      that send mail" and "billing-sync is the first addition" both said something now
      false and are corrected
- [ ] 9.5 Re-check the migration number against `main` after the final rebase
