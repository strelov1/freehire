## 1. Database

- [x] 1.1 Add migration: `ALTER TABLE mentor_bookings ADD COLUMN google_event_id text
      NOT NULL DEFAULT ''` (additive, no backfill). Filed as `0154_...` — `0152` and
      `0153` were already taken on `main` by the time this branched.
- [x] 1.2 Added `SetMentorBookingCalendarEvent` query (`UPDATE mentor_bookings SET
      meeting_url = $2, google_event_id = $3 WHERE id = $1`) — `CreateMentorBooking`'s
      own INSERT needs no change, since the column's `DEFAULT ''` already gives every
      new row the right starting value. Also added `GetGoogleGrantForWrite`
      (`gmail.sql`) for task 4.2 while touching the same generation step. Ran
      `make sqlc` — picks up the new column on every existing `RETURNING
      b.*`/`sqlc.embed(m)` query for free, `CancelMentorBooking` included.

## 2. `gmailsync`: the third incremental scope

- [x] 2.1 Add `CalendarEventsScope = "https://www.googleapis.com/auth/calendar.events"`
      beside `CalendarScope` in `connector.go`
- [x] 2.2 Add `Connector.MentorCalendarAuthCodeURL(state string) string` and
      `Connector.ExchangeMentorCalendar(ctx, code) (refreshToken string, scopes
      []string, err error)`, mirroring `CalendarAuthCodeURL`/`ExchangeCalendar`
      exactly: own `mentorCalendarRedirect` field
      (`origin + "/api/v1/me/mentor-calendar/callback"`), scope
      `[]string{CalendarEventsScope}`, `include_granted_scopes=true`
- [x] 2.3 Unit test: `MentorCalendarAuthCodeURL` requests `calendar.events` and not
      `gmail.readonly`/`calendar.readonly`; the redirect differs from both existing
      flows'

## 3. HTTP: connect/callback + status

- [x] 3.1 Add `mentorCalendarStateCookieName = "hire_mentor_calendar_state"` in
      `internal/api/handler/gmail.go`; add `MentorCalendarConnect`/
      `MentorCalendarCallback` handlers mirroring `CalendarConnect`/`CalendarCallback`
      (own state cookie, own redirect target, same `UpsertCalendarGrant`/
      `RecordGrantScopes` persistence, same `?mentor_calendar_error=...` /
      `?mentor_calendar=connected` redirect-to-`/my/integrations` convention)
- [x] 3.2 Register `GET /me/mentor-calendar/connect` (`mw.cookie`) and
      `GET /me/mentor-calendar/callback` (`mw.optionalCookie`) inside the existing
      `if h.gmailReady()` block in `register()`
- [x] 3.3 Extend `GmailStatus`'s response with `"mentor_calendar_connected":
      slices.Contains(conn.Scopes, gmailsync.CalendarEventsScope)`, alongside the
      existing `calendar_connected` (both branches: the connected row and the
      no-row/not-connected default)
- [x] 3.4 Unit test for `MentorCalendarConnect` (RequiresAuth +
      SendsToGoogleForCalendarEventsAlone — own cookie, own scope, none of the other
      two flows' scopes or cookies), mirroring `me_calendar_test.go` exactly. No unit
      test added for `MentorCalendarCallback`: `CalendarCallback` — the identically-
      shaped existing handler this mirrors — has none either (it needs a database),
      so this doesn't invent a coverage gap, it matches the one already there.

## 4. Domain: `internal/engage/mentorship` — the calendar-write seam

- [ ] 4.1 Add `GoogleEventID string` to `Booking` and to the `mentor_bookings` scan in
      `repository.go`
- [ ] 4.2 Add to `Repository` interface:
      `GetMentorCalendarGrant(ctx, userID int64) (refreshTokenEnc string, scopes
      []string, found bool, err error)` and
      `SetBookingCalendarEvent(ctx, bookingID uuid.UUID, meetingURL, eventID string)
      error` and `MarkCalendarGrantNeedsReconsent(ctx, userID int64) error`
      (the last can share the existing `needs_reconsent` status-set query if one
      already exists at the `db.Queries` level for `gmail_connections`; otherwise add
      one narrow query for it)
- [ ] 4.3 Implement the three on `QueriesRepository`
- [ ] 4.4 Add `CalendarLinker` interface and `ErrCalendarNotConnected` to a new
      `googlemeet.go`:
      ```go
      type CalendarLinker interface {
          CreateMeetEvent(ctx context.Context, userID int64, in MeetEventInput) (eventID, meetLink string, err error)
          DeleteMeetEvent(ctx context.Context, userID int64, eventID string) error
      }
      ```
      `MeetEventInput` carries what one event needs: start/end instants, the seeker's
      email, a summary. Implementation depends on `*gmailsync.Connector` and
      `*tokencrypt.Cipher` (both injected, both nil-safe the way `Notifier`/`Cache`
      already are on `Config`)
- [ ] 4.5 `CreateMeetEvent`: read the grant via `GetMentorCalendarGrant`; no row, or a
      row whose `scopes` lacks `CalendarEventsScope`, returns `ErrCalendarNotConnected`
      immediately (no HTTP call). Otherwise decrypt the refresh token, `POST
      https://www.googleapis.com/calendar/v3/calendars/primary/events?conferenceDataVersion=1`
      with one attendee (the seeker's email) and `conferenceData.createRequest` set
      (a random request id per call), mirroring `calsync.APIReader`'s plain-`net/http`
      style; wrap a non-200 response in `gmailsync.APIError{Op, StatusCode, Status}`;
      parse `id` and `hangoutLink` from the response
- [ ] 4.6 `DeleteMeetEvent`: same grant lookup, `DELETE
      .../calendars/primary/events/{eventID}`; a 404/410 (already gone) is treated as
      success, not an error
- [ ] 4.7 Add `CalendarLinker CalendarLinker` to `mentorship.Config`, `calendar
      CalendarLinker` to `Service`, wire it in `New()` — nil-safe throughout
- [ ] 4.8 Unit tests (fake `CalendarLinker` in `fake_repo_test.go`-style): not-connected
      → `ErrCalendarNotConnected`; connected + success → event id and Meet link parsed
      from a canned response; connected + 401/403 → wrapped as `gmailsync.APIError`
      and `RevokedGrant` reports true on it; a non-auth failure (500) → `RevokedGrant`
      reports false

## 5. `Book()`/`Cancel()`: use the seam

- [ ] 5.1 In `Book()`: after `s.repo.CreateBooking` succeeds, if `s.calendar != nil`
      call `CreateMeetEvent`. `ErrCalendarNotConnected` → do nothing further (the row
      already has `mentor.MeetingURL`, today's behavior, unchanged). Success → call
      `SetBookingCalendarEvent`, and patch the in-memory `booking.MeetingURL`/
      `GoogleEventID` before the notifier is invoked, so the confirmation email/ICS
      carries the real link. Any other error → log it (reusing `logDeliveryFailure`'s
      style), leave the row's link empty (insert `MeetingURL: ""` instead of
      `mentor.MeetingURL` whenever the mentor holds *any* `calendar.events` grant,
      connected-or-failing — see design.md), and if `gmailsync.RevokedGrant(err)` call
      `MarkCalendarGrantNeedsReconsent`
- [ ] 5.2 In `Cancel()`: after the existing cancel + notify, if
      `booking.GoogleEventID != ""` and `s.calendar != nil`, call `DeleteMeetEvent`
      best-effort, logging (never returning) a failure
- [ ] 5.3 Unit tests: a connected mentor's booking gets the fake `CalendarLinker`'s
      returned link and event id, and the notifier receives a `Booking` with that
      link (not the mentor's static one); an unconnected mentor's booking is
      byte-for-byte identical to today (regression guard, extending the existing
      booking tests); a `CreateMeetEvent` failure still returns a confirmed booking
      with an empty link and calls `MarkCalendarGrantNeedsReconsent` only when the
      fake reports a revocation-shaped error; cancelling a booking with an event id
      calls `DeleteMeetEvent`; a `DeleteMeetEvent` failure does not fail `Cancel()`

## 6. Profile: conditional meeting-link requirement

- [ ] 6.1 Add `HasCalendarLink bool` to `ProfileInput`
- [ ] 6.2 Change `validateMeetingURL(raw string, hasCalendarLink bool) error`: an empty
      `raw` returns `nil` when `hasCalendarLink`, otherwise unchanged; thread the new
      parameter through `validateProfile`'s one call site
- [ ] 6.3 In `internal/api/handler/mentorship_write.go`'s `mentorProfileBody` (or
      wherever `profileRequest.toInput(userID)` is called), look up the caller's
      `gmail_connections` scopes the same way `GmailStatus` does and set
      `in.HasCalendarLink` before calling `SubmitProfile`/`UpdateProfile`
- [ ] 6.4 Unit tests: `validateProfile` — empty link + `hasCalendarLink=false` →
      refused (regression); empty link + `hasCalendarLink=true` → accepted; a
      non-empty link is validated identically either way

## 7. Frontend

- [ ] 7.1 `/my/integrations`: add a "Connect Google Calendar for automatic Meet links"
      action beside the existing Gmail/Calendar cards, reading
      `mentor_calendar_connected` from the Gmail-status response and linking to
      `/api/v1/me/mentor-calendar/connect`
- [ ] 7.2 `MentorProfileEditor.svelte`: fetch whether the caller has
      `mentor_calendar_connected` (reuse the same status call `/my/integrations`
      already makes) and render the "Meeting link" field as optional — no required
      marker/validation-on-submit-message change needed beyond what the backend
      already refuses — when it is true
- [ ] 7.3 Manual check via the `run` skill in a real browser: connecting is out of
      reach without live Google credentials in dev, so verify what is testable
      without them — the profile form's meeting-link requirement toggles correctly
      when `mentor_calendar_connected` is stubbed/forced true via a direct API
      response, and confirm the unconnected path (the overwhelming common case) is
      pixel-identical to before this change

## 8. Verification

- [ ] 8.1 `gofmt -w`, `go vet ./...`, `go test ./...` on touched Go packages; confirm
      `internal/engage/mentorship` importing `internal/application/gmailsync` and
      `internal/platform/tokencrypt` passes the layering guard (both are legal
      downward imports — engage is layer 7, application and platform are 6 and 1)
- [ ] 8.2 `go vet -tags=integration ./...`; run the full integration suite for
      `internal/platform/db` and `internal/api/handler` if the SQL or handler
      signatures changed
- [ ] 8.3 `node scripts/check-migrations.mjs` on the new migration
- [ ] 8.4 `svelte-check`, `eslint`, full frontend `vitest run`
- [ ] 8.5 Confirm `notify.go`/`invite.go` need no change (already verified during
      design: both already guard every render on `MeetingURL != ""`) by adding one
      test each asserting an empty-link booking renders without a join link/URL line
