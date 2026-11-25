## Context

`internal/application/gmailsync` already owns every Google scope this platform
requests and the one connection row (`gmail_connections`, `PRIMARY KEY (user_id)`) that
records what a grant covers. It already runs one incremental, separately-consented
scope beyond the base `gmail.readonly`: `CalendarScope` (`calendar.readonly`), via
`Connector.CalendarAuthCodeURL`/`ExchangeCalendar`, its own redirect
(`/api/v1/me/calendar/callback`) and CSRF cookie (`hire_calendar_state`), landing in
the same row through `UpsertCalendarGrant` + `RecordGrantScopes`
(`internal/api/handler/gmail.go:277-332`). `internal/application/calsync` is the one
existing Calendar API caller, and it is read-only (`GET .../events`); nothing in this
codebase has ever written to Calendar. `gmailsync.RevokedGrant(err)`
(`internal/application/gmailsync/grant.go:55`) is the single place that already
distinguishes a revoked grant (401/403, or `invalid_grant` from the token endpoint)
from Google merely having a bad day — every reader of a Google API in this codebase
funnels its failures through `gmailsync.APIError` so this one function stays the only
place that decision is made.

`internal/engage/mentorship` (layer 7, "engage") may import
`internal/application/gmailsync` (layer 6, "application") — a legal downward
cross-block import per the layering table. `notify.go` and `invite.go` already guard
every render on `b.MeetingURL != ""`; no change is needed there.

## Goals / Non-Goals

**Goals:**
- A mentor can connect a `calendar.events` grant, entirely separate from any
  `gmail.readonly`/`calendar.readonly` grant the same account may hold for an
  unrelated feature.
- A connected mentor's confirmed booking carries a real Google Meet link, minted for
  that booking; an unconnected mentor sees no change at all.
- Every failure mode (not connected, revoked, Google unavailable) degrades to "no link
  on this booking," never to a blocked or delayed booking.

**Non-Goals:**
- Free/busy sync (`calendar.readonly` for a mentor, populating `mentor_busy_intervals`)
  — a separate, already-deferred half of the same idea; this change touches neither.
- Any new UI for disconnecting or inspecting this grant beyond what `/my/integrations`
  already renders for the other two.
- Retrying a failed calendar-event creation later, or reconciling `mentor_bookings`
  with Calendar state after the fact.

## Decisions

**A third incremental scope on the same `Connector`, not a new OAuth client.**
`Connector` gains `MentorCalendarAuthCodeURL`/`ExchangeMentorCalendar`, mirroring
`CalendarAuthCodeURL`/`ExchangeCalendar` exactly: own redirect
(`/api/v1/me/mentor-calendar/callback`), own state cookie
(`hire_mentor_calendar_state`), scope `https://www.googleapis.com/auth/calendar.events`
(a new `CalendarEventsScope` constant beside `CalendarScope`), `include_granted_scopes`
so a user who already holds `gmail.readonly` or `calendar.readonly` keeps them. The
callback upserts into the SAME `gmail_connections` row via the SAME
`UpsertCalendarGrant`/`RecordGrantScopes` pair the read-only flow already uses — no new
table, no new "purpose" column. This is what the codebase's own comments already argue
for `calendar.readonly` beside `gmail.readonly`: one grant, one status, one row, and
`RecordGrantScopes` stores whatever Google says the grant covers today, which is why
two unrelated consents on one account safely coexist. What is genuinely new here is
the *effect*: instead of a second read-only capability, this consent additionally
enables a **write** capability — hence its own scope constant and its own explicit
consent screen, never inferred from the other two.

**Detecting "does this mentor have `calendar.events`" reads the same row's `scopes`
column**, via a new narrow repository method
(`GetMentorCalendarGrant(ctx, userID) (refreshTokenEnc string, scopes []string, found bool, err error)`)
on `mentorship.Repository` — not a new interface into `gmailsync`, because the
mentorship package already owns its own `Repository` seam and this is one more query
through it, alongside `ListAvailability`/`ListBusy`.

**The write client is a new small type inside `internal/engage/mentorship`
(`googlemeet.go`), not a new package.** It has exactly one consumer. It depends on
`gmailsync.Connector` (for `HTTPClient`/`TokenSource`) and `tokencrypt.Cipher` (to
decrypt the stored refresh token) — both passed into `mentorship.Config`, mirroring how
`Cache` and `Notifier` are already optional, nil-safe dependencies. It exposes:

```go
type CalendarLinker interface {
    CreateMeetEvent(ctx context.Context, userID int64, in MeetEventInput) (eventID, meetLink string, err error)
    DeleteMeetEvent(ctx context.Context, userID int64, eventID string) error
}
var ErrCalendarNotConnected = errors.New("mentorship: mentor has not connected a calendar")
```

`CreateMeetEvent` reads the mentor's grant via the new repository method; a missing
grant or a grant without `calendar.events` returns `ErrCalendarNotConnected` — the
ordinary, frequent, not-an-error case for the majority of mentors who never connect
one. Any other error is a real failure worth logging. The Calendar insert call is
`POST .../calendars/primary/events?conferenceDataVersion=1` with one attendee (the
seeker's email) and the request body's `conferenceData.createRequest` set — mirroring
`calsync.APIReader`'s request-building style (plain `net/http`, no
`google.golang.org/api` dependency introduced), and wrapping a non-200 response in the
same `gmailsync.APIError{Op, StatusCode, Status}` `calsync` already uses, so
`gmailsync.RevokedGrant` keeps working unmodified on this new call site too.

**Booking insert, then a best-effort update — never the reverse.** `Book()` calls
`s.repo.CreateBooking` first, exactly as today (this is what the EXCLUDE constraint
race-guards); only once that succeeds does it attempt `CreateMeetEvent`. Creating the
Calendar event *before* the database insert would leave an orphaned Google event
whenever the booking lost the race. On success, a new repository method
(`SetBookingCalendarEvent(ctx, bookingID, eventID, meetingURL) error`) updates the
row's `meeting_url` and `google_event_id`, and the in-memory `Booking` the rest of
`Book()` uses (notification, the returned value) is patched with the same two fields
before `notifier.BookingConfirmed` is called — so the confirmation email is never sent
with a stale or empty link when generation actually succeeded. `ErrCalendarNotConnected`
is not logged and leaves `MeetingURL: mentor.MeetingURL` (today's exact behavior,
inserted as the initial value so a crash between the two writes is harmless for the
overwhelming majority of mentors who are not connected). Any other `CreateMeetEvent`
error is logged, leaves `MeetingURL` empty (the initial insert therefore uses `""` when
the mentor OWNS a `calendar.events` grant at all — the connected-mentor's static field
is simply never written into a booking row, matching the mentor-profile delta's own
"never reads it" scenario), and — if `gmailsync.RevokedGrant(err)` — marks that grant
`needs_reconsent` through the same repository method the profile side of this change
already needs for status reporting.

**Cancellation deletes best-effort, using the row's own `google_event_id`.**
`Cancel()` already gets the full post-cancel `Booking` back from
`s.repo.CancelBooking` (its `RETURNING b.*` picks up the new column automatically once
migrated). If `GoogleEventID != ""` and `s.calendar != nil`, `Cancel()` calls
`DeleteMeetEvent` after the notification best-effort dispatch, logging — never
returning — a failure. No new repository read is needed.

**`validateMeetingURL` takes a second, server-computed argument.** `ProfileInput`
gains `HasCalendarLink bool`, set by the HTTP handler (`mentorship_write.go`) — which
already has direct access to `db.Queries` to check the caller's `gmail_connections`
row — before calling `req.toInput(userID)`. `internal/engage/mentorship` stays pure
and IO-free at the validation layer: `validateMeetingURL(raw string, hasCalendarLink
bool) error` returns `nil` for an empty `raw` exactly when `hasCalendarLink` is true;
every other branch (shape, scheme) is unchanged. This mirrors `creating bool` already
threaded into `validateProfile` for the same reason — a caller-supplied fact
`validateProfile` itself has no way to derive.

## Risks / Trade-offs

- **A mentor with both a candidate-side `calendar.readonly` grant and this
  `calendar.events` grant sees one merged row.** → By design, matching how
  `gmail.readonly` and `calendar.readonly` already coexist on one row; `scopes`
  simply grows. Revoking one at Google's own account settings revokes the whole
  grant, exactly as it already does for the existing two — not a new failure
  mode this change introduces.
- **The Calendar insert call runs synchronously inside `Book()`'s request path.** →
  Accepted for a first cut: it is a single HTTP call, gated behind an explicit opt-in a
  small fraction of mentors will take, and a slow or failing call already degrades to
  "no link" rather than blocking. Revisit only if it is measured to matter.
- **A booking's meeting link can very briefly be wrong between the two writes** (insert
  with a placeholder, then update) if the process crashes in between. → Accepted: the
  next read of that booking already tolerates an empty link everywhere
  (`notify.go`/`invite.go`), and the alternative — a database transaction spanning an
  external HTTP call — would hold the EXCLUDE-constraint row locked for the Calendar
  API's latency, which is worse.
- **Two consecutive bookings for the same connected mentor must not confuse or reuse
  event ids.** → Each `Book()` call is independent; `CreateMeetEvent` is called once
  per booking and its result is written to that booking's own row only.

## Migration Plan

- New migration: `ALTER TABLE mentor_bookings ADD COLUMN google_event_id text NOT NULL
  DEFAULT ''` (additive, no backfill — every existing row simply has none).
- No change to `mentors` or `gmail_connections` schema.
- Ship the OAuth plumbing and the profile validation change together with the booking
  behavior — there is no useful intermediate state to deploy separately, since the
  connect flow is inert until `Book()` also knows to use it.
