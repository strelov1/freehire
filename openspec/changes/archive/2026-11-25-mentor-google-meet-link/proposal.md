## Why

A mentor must currently paste their own meeting link by hand before they can even
submit a profile, and that same static link is reused for every booking regardless of
what it actually points to. This is unnecessary friction for a mentor who already
manages their calendar in Google: Google Calendar's own API can mint a fresh Google
Meet link per event, and this platform already holds Google OAuth machinery (the
"Connect Gmail" grant, its incremental-authorization pattern, its encrypted token
storage) that a new, narrower consent can reuse rather than duplicate.

## What Changes

- A mentor may optionally connect Google Calendar with write access
  (`calendar.events`), on `/my/integrations` alongside the existing Gmail/calendar-read
  connections — a new capability, `mentor-calendar-link`.
- When a mentor has connected it, the mentor-profile create/edit form's meeting-link
  field becomes optional; without it, the field stays required exactly as today.
- When a booking is confirmed for a mentor with the grant, the system creates a Google
  Calendar event (the seeker as attendee, `conferenceDataVersion: 1`) and uses the
  returned Meet link as that booking's own meeting link, instead of the profile's
  static one. A mentor without the grant keeps today's exact behavior.
- A failed calendar-event creation (revoked grant, Google outage) never blocks the
  booking: it confirms with an empty meeting link, and a revocation-shaped failure
  marks the grant `needs_reconsent` — the same status the existing Gmail/calendar-read
  grant already uses.
- Cancelling a booking that created a calendar event best-effort deletes that event
  too.
- Explicitly out of scope: mentor free/busy sync (`calendar.readonly`, still
  undelivered — `mentor_busy_intervals` stays empty), any new UI to manage or
  disconnect the grant beyond what Gmail's own connection already offers, and any
  change on the seeker's side (they receive the event as a Google Calendar attendee
  invite over email, without connecting anything themselves).

## Capabilities

### New Capabilities
- `mentor-calendar-link`: a mentor's own opt-in Google Calendar write consent — the
  OAuth flow, its scope, and where its encrypted grant is stored.

### Modified Capabilities
- `mentor-profile`: the meeting link is required only for a mentor who has not
  connected Google Calendar.
- `mentor-booking`: a connected mentor's booking gets an auto-generated Google Meet
  link instead of the profile's static one; a failed generation still confirms the
  booking with no link; cancelling a booking best-effort removes its calendar event.

## Impact

- **Backend**: `internal/application/gmailsync` (a new `AuthCodeURL`/`Exchange` pair
  for the `calendar.events` scope, its own redirect and CSRF cookie, mirroring the
  existing calendar-readonly incremental flow); `internal/api/handler` (new
  connect/callback routes, `mentorship_write.go`/`profile.go` validation change); a
  new narrow Calendar-write client (event insert and delete — the first write call
  this codebase makes to the Calendar API; every existing call, in
  `internal/application/calsync`, is read-only); `internal/engage/mentorship`
  (`booking.go` creates/attaches the event at confirmation, `CancelBooking` best-effort
  deletes it, `profile.go`'s `validateMeetingURL` becomes conditional); `notify.go` and
  `invite.go` (render correctly when a booking's meeting link is empty); a migration
  adding nullable `mentor_bookings.google_event_id`.
- **Frontend**: `/my/integrations` (a new connect action), `MentorProfileEditor.svelte`
  (the meeting-link field's required-ness depends on whether the grant exists).
- **No changes** to the seeker's side, to `mentor_busy_intervals`/free-busy sync, or to
  the existing Gmail/calendar-readonly connect flow's own behavior.
