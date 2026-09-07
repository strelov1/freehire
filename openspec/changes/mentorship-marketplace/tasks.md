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

## 2. Schema and generated queries

- [ ] 2.1 Write the migration (claim the number immediately before opening the PR):
      `CREATE EXTENSION IF NOT EXISTS btree_gist`, then `mentors`,
      `mentor_availability`, `mentor_bookings`, `mentor_busy_intervals`,
      `mentor_reviews`, with every `CHECK` the specs require
- [ ] 2.2 Add the `EXCLUDE USING gist` non-overlap constraint on `mentor_bookings`,
      predicated on `status = 'confirmed'`
- [ ] 2.3 Add the `jobs` foreign key as `NOT VALID` plus a separate `VALIDATE
      CONSTRAINT`, with the squawk suppression carrying the `55P03` reason beside it
- [ ] 2.4 Run `pnpm check:sql` and confirm the migration passes both squawk passes
- [ ] 2.5 Write the sqlc queries (profile CRUD and moderation, availability CRUD,
      booking write and lists, reminder claim, review upsert) and run `make sqlc`
- [ ] 2.6 Integration test the `EXCLUDE` constraint directly: two concurrent inserts for
      one slot leave exactly one confirmed row, and a cancelled row does not block its
      own time

## 3. Profile domain and moderation

- [ ] 3.1 Profile service: create, read, update, with one profile per account and a
      company that must exist in `companies`; sentinels carry their HTTP mapping in
      comments, following `referral`'s convention
- [ ] 3.2 Moderation: pending queue oldest-first, approve and reject recording the
      deciding moderator and time, and an approved `referral_offers` row surfaced as
      evidence but never as a gate
- [ ] 3.3 Pause and resume without moderation, leaving confirmed bookings standing
- [ ] 3.4 Withdrawal: cancel every confirmed future booking, notify each seeker, retain
      past bookings as history
- [ ] 3.5 Public directory: approved and unpaused only, narrowable by company, topic and
      language, reporting unread parameters in `meta.ignored_params`

## 4. Booking domain

- [ ] 4.1 Booking creation: authenticated only, never one's own profile, re-deriving the
      slot at write time and refusing with a named reason
- [ ] 4.2 Map the `EXCLUDE` violation to the same "no longer available" sentinel a stale
      page gets, so a race and a stale tab are indistinguishable to the client
- [ ] 4.3 Random booking identifiers, and the `job_id` reference that outlives its vacancy
- [ ] 4.4 Cancellation by either party before the start, recording who and when,
      refusing a past or already-cancelled booking, and refusing a stranger without
      revealing whether the booking exists
- [ ] 4.5 Per-party session lists, split upcoming and past, each readable only by its owner
- [ ] 4.6 Completion and review: one review per completed booking, seeker only, editable
      without duplicating, never for a cancelled session; aggregate rating and count on
      the profile

## 5. Notifications

- [ ] 5.1 Confirmation to both parties, each in their own zone, carrying the meeting link
      and an `.ics` from `internal/application/ical`; delivery best-effort, a failure
      logged and never undoing the booking
- [ ] 5.2 Cancellation notice to the other party
- [ ] 5.3 Assert in test that booking messages are delivered with the account
      notification rule disabled, and that saved-job reminders and nudges still are not

## 6. Reminder worker

- [ ] 6.1 `cmd/mentorship-remind`: run-once-and-exit, needs `DATABASE_URL`, a clean
      no-op without a mail transport, non-zero exit on failure
- [ ] 6.2 Send 24h and 1h reminders idempotently per `(booking, offset)`; tests cover a
      re-run sending nothing twice, a cancellation stopping a pending reminder, and a
      missed window not firing late
- [ ] 6.3 Write the systemd unit and timer into `deploy/`, and note in the change that
      it must be copied to the host by hand — `release.sh` never touches a unit

## 7. HTTP layer

- [ ] 7.1 Public routes: mentor directory, mentor profile, slot listing — no auth
- [ ] 7.2 Slot cache in Redis keyed by `(mentor, window, viewer timezone)`, invalidated
      on booking, cancellation and any availability or session-parameter edit;
      degrading to direct computation when the cache is unreachable
- [ ] 7.3 Rate limit the slot endpoint, on the real route
- [ ] 7.4 Authenticated routes: book, cancel, my sessions, review; mentor cabinet routes
      for profile and availability
- [ ] 7.5 Moderator routes behind the existing moderator gate, beside the referral queue
- [ ] 7.6 One error switch mapping every domain sentinel to its status, following
      `internal/api/handler/referrals.go`

## 8. Frontend

- [ ] 8.1 Public mentor directory page with company, topic and language filters
- [ ] 8.2 Public mentor profile with the booking calendar, sending the browser's
      resolved timezone and rendering slots in it
- [ ] 8.3 Booking confirmation and cancellation flows, and the seeker's session list
- [ ] 8.4 Mentor cabinet under `/my/`: profile editing, weekly schedule, date overrides,
      and the mentor's own session list
- [ ] 8.5 Moderation queue screen beside the referral queue
- [ ] 8.6 Entry point from the vacancy and company pages, rendered only where the
      company has an approved unpaused mentor — placement coordinated with the open
      `job-page-cta-hierarchy` change
- [ ] 8.7 Review submission after a completed session

## 9. Verification

- [ ] 9.1 `gofmt -l .` prints nothing; `go vet ./...`, `go test ./...`,
      `go vet -tags=integration ./...` all pass
- [ ] 9.2 `go test -tags=integration ./...` passes with Docker available
- [ ] 9.3 `pnpm --dir web lint` and the web test suite pass; `pnpm check:links` passes
- [ ] 9.4 Write `internal/engage/mentorship/AGENTS.md` covering what is always true here:
      the empty-override trick, the zone-resolution order, the `EXCLUDE` constraint and
      why the application still re-derives, the transactional-notification boundary,
      and the two named-but-unbuilt seams (money, Google sync)
- [ ] 9.5 Re-check the migration number against `main` after the final rebase
