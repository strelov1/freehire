## Context

Two existing features meet here:

- `internal/engage/buddy` — the anonymous peer network. A member is a handle; nothing that
  identifies the account reaches another member. Chat lives in the messaging core
  (`internal/engage/messaging`, kind `buddy`). There is no scheduling and no member time
  zone.
- `internal/engage/mentorship` — the mentor marketplace. Its slot engine (`Slots(SlotRequest)`
  and the expand/override/subtract/slice pipeline) is pure: no database, no mentor id.
  Everything around it — tables, repository, reminder SQL, Svelte components — is
  mentor-specific.

Both packages are in the `engage` block (layer 7), so either may import a sibling package
in the same block.

## Goals / Non-Goals

**Goals:**
- A member publishes when they can run a mock; another member books it themselves.
- One mock per person per instant, enforced by the database.
- Reminders, cancellation, and a two-way review that cannot be used for retaliation.
- No account detail reaches the other member through anything freehire sends.

**Non-Goals:**
- Automatic pairing by time and topic (Pramp's matcher). Later, fed by the availability
  this change collects.
- A call room on freehire. The host supplies their own link.
- A question bank or in-call timer.
- Suggesting a mock before a real interview found by calendar sync. Later.
- Calendar (`.ics`) attachments. The mentor invitation addresses attendees by email, which
  buddies must not reveal.
- Exposing mocks over MCP, OJCP or the public API.

## Decisions

### D1. Extract the engine into `internal/engage/slots`
Move the pure files (including `calendar.go`, which calls the engine's unexported
functions) and their tests verbatim, export `withID` as `WithID` for the repositories, rename the mentor-flavoured identifiers
(`MentorZone` → `OwnerZone`, `ErrNoMentorZone` → `ErrNoOwnerZone`), and have mentorship
import the new package. Add `slots` to the engage list in
`internal/platform/arch/layering/blocks.go`.
*Alternative:* buddy imports `mentorship` for the engine. Rejected: it would pull a whole
marketplace into buddy to compute times, and the engine's comments already argue in
general terms.

### D2. Fixed session parameters
Mocks are 60 minutes, no buffers, 12 hours minimum notice, 30-day horizon. Constants in
`buddy`, not columns. A peer mock has one shape; per-member tuning is a mentor feature.

### D3. Tables (migration `0214_buddy_mocks.sql` (the next free number at implementation time))
- `buddy_mock_settings(user_id PK → buddy_members, timezone, meeting_url, topics text[],
  paused, created_at, updated_at)` — `topics` non-empty and within the vocabulary,
  `meeting_url` must start with `https://`.
- `buddy_mock_availability` — the same shape as `mentor_availability`: `weekday` XOR
  `on_date`, zoneless `start_time`/`end_time`.
- `buddy_mocks(id uuid, host_user_id, guest_user_id, starts_at, ends_at, topic, status
  confirmed|cancelled, guest_timezone, cancelled_by, cancelled_at, cancel_reason
  participant|left|blocked, cancel_announced_at, created_at)` with `host <> guest`.
  `guest_timezone` is the zone the guest booked in (D9 renders their notices in it);
  `cancel_reason` picks the notice wording; `cancel_announced_at` lets a cancellation made
  inside SQL be announced afterwards (D7).
- `buddy_mock_participants(mock_id, user_id, starts_at, ends_at, active)` — two rows per
  mock, with `EXCLUDE USING gist (user_id WITH =, tstzrange(starts_at, ends_at, '[)') WITH &&)
  WHERE (active)`. Mentors only constrain the mentor; here either person can be host or
  guest, and a single constraint over a per-person row is the only way the database can
  refuse "host at 19:00 and guest at 19:00". Cancelling sets `active = false` in the same
  statement as the mock's status.
- `buddy_mock_reviews(mock_id, author_user_id, subject_user_id, rating 1-5, comment ≤ 1000,
  created_at, PK (mock_id, author_user_id))`.
- `buddy_mock_reminders(mock_id, offset_minutes, sent_at, PK (mock_id, offset_minutes))`.

Every `users` reference cascades and is indexed (`TestEveryUserForeignKeyIsIndexed`).

### D4. The meeting link is read live, not snapshotted
A mentor booking snapshots the link. A mock shows the host's CURRENT link, so a host who
fixes a typo reaches guests who already booked. Changing the link is the host's own act on
their own sessions.

### D5. Booking path
`Book(ctx, BookInput{HostHandle, GuestUserID, StartsAt, Topic, GuestZone})`:
1. Both are active members; neither has blocked the other; host has settings and is not
   paused; topic is one the host offers; guest is not the host.
2. Guest has fewer than 3 future confirmed mocks as guest (spam bound).
3. Re-derive the slot with `slots.Slots`, where busy = the host's future active
   participant rows (as host or guest). Not offerable → `ErrSlotUnavailable`.
4. Insert the mock and both participant rows in one transaction. A 23P01 exclusion
   violation (either person) → `ErrSlotUnavailable`. One error for every ordinary failure,
   the rule mentorship already argues.
5. Notify both, best-effort.

The slot listing reads availability and busy rows the same way, without a cache: buddy
traffic is members-only and small.

### D6. Blind review
A participant may review once, between the mock's end and 7 days after it, and only a
confirmed mock. A member page shows rating, comment, topic and day, never the author's
handle, so the page does not publish who practised with whom. A review is revealed when the counterpart's review exists or the window
has closed. Reveal is computed in the read query, not stored. The member page aggregates
only revealed reviews.

### D7. Cancellation triggers
Either participant, before `starts_at`. A leave and a block both cancel the member's (or
the pair's) future mocks in the same statement, so a mock can never outlive the
relationship that allowed it:
- **Leave:** the existing `LeaveBuddyMember` statement also cancels the member's mocks.
- **Block:** the block is written by the messaging core (`BlockFromConversation`), which
  must never import a feature. The buddy migration adds an `AFTER INSERT` trigger on
  `user_blocks` that cancels the pair's future mocks, which covers every path that writes a
  block.

A trigger returns no rows, so notices are decoupled: every cancellation records its reason
and leaves `cancel_announced_at` NULL, and `MockService.AnnounceCancellations(actor)` claims
and announces the unannounced ones. The HTTP layer, the only one that sees both features,
calls it after the buddy leave and after the conversation block route. A cancellation by
leave or block tells the other side only "the mock was cancelled".

### D8. Reminders inside `cmd/mentorship-remind`
Add `buddy.SendDueMockReminders` with the same claim → send → release-on-failure order
and the same one-hour window per offset. The command calls both passes; a failure in one
does not skip the other, and the run exits non-zero if either failed.
*Alternative:* a new `cmd/buddy-mock-remind` with its own timer. Rejected: one more unit
in `freehire-ops` for the same schedule.

### D9. Notifications
Email (SES transport) plus a notification-center row, worded with handles only. Kinds:
`buddy_mock_booked`, `buddy_mock_cancelled`, `buddy_mock_reminder`. Times are rendered in
each recipient's own zone (the host's from settings, the guest's from the booking), falling
back to UTC and saying so — the mentorship rule.

### D10. Shared Svelte components
`MentorScheduleEditor.svelte` and `MentorBookingView.svelte` take their data-loading and
saving functions as props. Mentorship pages pass the mentor API; buddy pages pass the mock
API. No behaviour change for mentors.

## Risks / Trade-offs

- **The host's link can reveal their name.** Accepted by product decision; the settings
  form says so before the link is saved.
- **Few members → few slots.** Self-booking works with any count; auto-pairing waits for
  data.
- **Ratings invite gaming.** Blind reveal removes retaliation. Review moderation (report,
  hide) is out of scope until abuse is seen; a block between the two still cancels their
  future mocks.

## Migration Plan

Migration first (release.sh runs it), then the binary. The engine extraction ships in the
same release; mentorship tests guard it. Rollback: the tables are new and nothing else
reads them.
