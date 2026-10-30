## Why

Interview buddies can find each other and chat, but nothing helps them do the one thing
peers are best at: practising an interview together. Today two members who want a mock
have to agree on a time in chat, convert time zones by hand and remember it. Pramp-style
peer mocks (two people, sixty minutes, roles swap halfway) need no expert, so they scale
with the buddy network rather than with the small mentor pool.

The mentorship marketplace already holds a tested slot engine — weekly availability in the
owner's own zone, dated overrides, DST-safe expansion, minimum notice, horizon — so the
hard part exists. It is simply locked inside the mentorship package.

## What Changes

- **Extract the slot engine** (`slots.go`, `expand.go`, `override.go`, `subtract.go`,
  `slice.go`, `schedule.go` and their tests) from `internal/engage/mentorship` into a new
  pure package `internal/engage/slots`. Mentorship behaviour does not change.
- **Mock settings per member**: time zone (required), a meeting link the member provides
  (Zoom, Meet, anything `https://`), the topics they offer (`coding`, `system_design`,
  `behavioral`), and a pause switch. The form warns that the guest may see the host's name
  through that link.
- **Mock availability**: the same weekly-plus-dated-override model mentors use, edited with
  the same calendar component.
- **Booking**: a member opens another member's page, sees open slots in their own zone,
  picks a slot and one of the host's topics, and books. Mocks are 60 minutes. A person can
  never be in two mocks at once, as host or as guest, and Postgres enforces it.
- **Link visibility**: the meeting link is shown only to the two participants, and to the
  guest only after booking.
- **Cancellation** by either participant before the start. Leaving the network, or a block
  between the two, cancels their future mocks.
- **Reminders** 24 hours and 1 hour before, to both participants, as a second pass inside
  the existing `cmd/mentorship-remind`.
- **Two-way blind review**: after the mock each participant rates the other 1-5 with an
  optional comment. A review stays hidden until the other side has written theirs or the
  7-day review window has closed. A member's page shows their completed mock count,
  average rating and recent revealed reviews.
- **Notifications** carry handles only, never a name or an email.
- New page `/my/buddies/mocks` (settings, calendar, upcoming and past mocks); a booking
  panel on `/my/buddies/<handle>`. The mentor schedule editor and slot picker take their
  API calls as props so both features share them.

## Capabilities

### New Capabilities
- `buddy-mock-interviews`: mock settings, availability, booking, cancellation, reminders
  and the blind two-way review between buddy members.

### Modified Capabilities
- None. The engine extraction is a move with no behaviour change; mentorship specs stay as
  they are.

## Impact

- Code: new `internal/engage/slots`; `internal/engage/mentorship` imports it; new
  `mock*.go` files in `internal/engage/buddy`; handlers in `internal/api/handler`;
  `cmd/mentorship-remind` gains a buddy pass; `internal/platform/arch/layering/blocks.go`
  gains `slots`.
- Database: migration `0214_buddy_mocks.sql` (the next free number at implementation time) (six tables), queries in
  `internal/platform/db/queries/buddy_mocks.sql`.
- Web: `/my/buddies/mocks`, booking panel on the member page, refactored
  `MentorScheduleEditor.svelte` / `MentorBookingView.svelte`.
- Ops: none new — the existing `freehire-mentorship-remind` timer runs the extra pass.
- Free for every member. No plan gate.
