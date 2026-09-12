## Why

`mentor_busy_intervals` has existed since migration 0145 and the slot engine already reads
it (`Repository.ListBusy` unions it with confirmed bookings before slicing), but nothing
has ever written to it — the table's own comment says so: "Empty until the calendar sync
ships." A mentor whose Google Calendar already holds an interview, a doctor's appointment,
or another commitment is still offered as available for that hour on freehire, and can only
be protected from a double-booking by remembering to add a manual schedule override. This
closes that gap by finally shipping the sync `mentor-google-meet-link`'s design explicitly
deferred ("a separate, already-deferred half of the same idea").

## What Changes

- A mentor may optionally connect Google Calendar for busy-time sync
  (`calendar.readonly`), on `/my/integrations` alongside the existing Gmail/calendar-read
  and mentor-calendar-write (Meet link) connections — a new capability,
  `mentor-busy-sync`. This is its own explicit consent: it is never inferred from an
  unrelated existing `calendar.readonly` grant (e.g. one made for the candidate-side
  interview-tracking feature), matching the rule `mentor-calendar-link` already
  established for the write scope.
- A new periodic worker (`cmd/mentor-busy-sync`) reads each connected mentor's Google
  Calendar free/busy over a bounded future window and reconciles the result into
  `mentor_busy_intervals`, keyed by `(mentor_id, source, external_id)` — updating rows
  that changed, inserting new ones, and removing ones no longer busy. It reads the
  **free/busy** endpoint, not the events list `internal/application/calsync` uses: this
  worker only ever needs bare intervals, never a title or attendee, and asking Google for
  strictly less data is a stronger privacy guarantee than reading events and discarding
  their fields in application code.
- A failed sync for one mentor (revoked grant, Google outage) never blocks the rest of the
  run; a revocation-shaped failure marks the grant `needs_reconsent`, the same status the
  existing Gmail/calendar-read/write grants already use.
- Disconnecting, or a grant moving to `needs_reconsent`, stops future syncs for that mentor
  but does not retroactively clear rows already written — the same "last known state until
  proven otherwise" the rest of this codebase's calendar sync follows.
- Explicitly out of scope: any change to the slot engine's consumption of the busy set
  (`ListBusy`'s union and the `mentor-availability` spec's "busy time and buffers are
  subtracted before slicing" requirement already describe this generically and need no
  change), any UI beyond the connect card, and syncing a mentor's non-primary calendars.

## Capabilities

### New Capabilities
- `mentor-busy-sync`: a mentor's own opt-in Google Calendar read-only consent for
  busy-time sync, the periodic worker that reconciles `mentor_busy_intervals` from it,
  and the reconsent handling for a revoked grant.

### Modified Capabilities
(none — `mentor-availability`'s busy-set requirement is already source-agnostic, and the
slot engine's consumption of `mentor_busy_intervals` does not change)

## Impact

- **Backend**: `internal/application/gmailsync` (a `MentorBusyAuthCodeURL`/
  `ExchangeMentorBusy` pair for `CalendarScope`, its own redirect and CSRF cookie,
  mirroring the existing incremental-consent pairs); `internal/api/handler` (new
  connect/callback routes, `GmailStatus` extended with a `mentor_busy_sync_connected`
  field); a new `internal/engage/mentorship/busysync` (or similarly-scoped) package
  holding the worker, its Google free/busy reader, and the reconcile logic against
  `mentor_busy_intervals`; a new `cmd/mentor-busy-sync` cron worker plus its systemd unit
  and timer under `deploy/systemd/`.
- **Frontend**: `/my/integrations` (a new connect card, mirroring the existing Google
  cards).
- **No changes** to the slot engine, `mentor-availability`, booking, or any other
  mentorship read path — this only ever adds rows the engine already knows how to
  consume.
