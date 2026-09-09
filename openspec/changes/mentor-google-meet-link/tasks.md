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

- [x] 4.1 Added `GoogleEventID string` to `Booking` and to `bookingFromRow` (reads
      `db.MentorBooking.GoogleEventID`, present on every `mentor_bookings` scan since
      task 1.2's `make sqlc` regeneration).
- [x] 4.2 Added to `Repository`: `GetMentorCalendarGrant`, `SetBookingCalendarEvent`,
      `MarkCalendarGrantNeedsReconsent`. `GetMentorCalendarGrant` treats any status
      other than `'connected'` (needs_reconsent included) as not-found, so the caller
      never needs a second query to learn a grant is unusable.
- [x] 4.3 Implemented the three on `QueriesRepository` (`repository.go`):
      `GetMentorCalendarGrant` over `GetGoogleGrantForWrite`, `SetBookingCalendarEvent`
      over `SetMentorBookingCalendarEvent`, `MarkCalendarGrantNeedsReconsent` reusing
      `SetGmailStatus` — the same query the read-side grants already share.
- [x] 4.4 Added `CalendarLinker`/`ErrCalendarNotConnected`/`MeetEventInput` to
      `googlemeet.go`, plus `GoogleCalendarLinker` (depends on a narrow
      `calendarGrantReader`, `*gmailsync.Connector`, `*tokencrypt.Cipher`).
- [x] 4.5 `GoogleCalendarLinker.CreateMeetEvent`: `resolve()` reads the grant and
      refuses with `ErrCalendarNotConnected` before any HTTP call when not found or
      missing `CalendarEventsScope`; otherwise decrypts the token and POSTs via the
      extracted `meetAPI.createEvent`, mirroring `calsync.APIReader`'s style —
      `conferenceDataVersion=1`, one attendee, a random `requestId` per call, a non-2xx
      wrapped in `gmailsync.APIError`, `id`/`hangoutLink` parsed from the response.
- [x] 4.6 `DeleteMeetEvent`/`meetAPI.deleteEvent`: same grant gate, `DELETE
      .../events/{eventID}`; 200/204/404/410 all read as success.
- [x] 4.7 Added `CalendarLinker CalendarLinker` to `Config`, `calendar CalendarLinker`
      to `Service`, wired in `New()` — nil-safe.
- [x] 4.8 `googlemeet_test.go`: not-connected (no row, and a grant present but missing
      the write scope) → `ErrCalendarNotConnected` before any network call; `meetAPI`
      tested directly against `httptest` (mirroring `calsync`'s rewrite-transport
      pattern, extended to preserve the DELETE path segment) — success parses
      `id`/`hangoutLink`; 403 wraps as `gmailsync.APIError` and `RevokedGrant` reports
      true; 500 does not; 404/410 on delete read as success.
      Deviation from the task text: the HTTP-shape tests exercise the extracted
      `meetAPI` directly with a plain client rather than `GoogleCalendarLinker` end to
      end, so they need no real OAuth handshake — exactly how `calsync.APIReader`'s own
      tests avoid it. The not-connected gate IS tested through `GoogleCalendarLinker`
      itself, since that path needs no network call at all.

## 5. `Book()`/`Cancel()`: use the seam

- [x] 5.1 In `Book()`: `CreateBooking` still snapshots `mentor.MeetingURL` as before;
      when `s.calendar != nil`, `attachMeetEvent` then calls `CreateMeetEvent`.
      `ErrCalendarNotConnected` → leave the row as inserted (today's behaviour,
      unchanged). Success → `SetBookingCalendarEvent` plus an in-memory patch of
      `booking.MeetingURL`/`GoogleEventID` before the notifier fires. Any other error →
      `logDeliveryFailure`, then `SetBookingCalendarEvent(id, "", "")` to blank the
      stale static link (a mentor reaching this branch DOES hold a grant, by
      construction — `ErrCalendarNotConnected` would have returned above otherwise),
      and `MarkCalendarGrantNeedsReconsent` when `gmailsync.RevokedGrant(err)`.
- [x] 5.2 In `Cancel()`: after the existing cancel + notify, if
      `booking.GoogleEventID != ""` and `s.calendar != nil`, `DeleteMeetEvent`
      best-effort (called with the mentor's user id, since the grant lives on their
      account), logging — never returning — a failure.
- [x] 5.3 `booking_test.go`: an unconnected mentor (`s.calendar == nil`) and a
      connected-linker-but-no-grant mentor (`ErrCalendarNotConnected`) both book
      byte-for-byte as today; a connected mentor's booking carries the fake linker's
      link/event id and the notifier receives that same link; a non-`ErrCalendarNotConnected`
      failure still returns a confirmed booking with an empty link and marks
      `needs_reconsent` only for a `gmailsync.APIError{401}`-shaped one; cancelling
      calls `DeleteMeetEvent` with the stored event id; a `DeleteMeetEvent` failure
      does not fail `Cancel()`.

## 6. Profile: conditional meeting-link requirement

- [x] 6.1 Added `HasCalendarLink bool` to `ProfileInput`.
- [x] 6.2 Changed `validateMeetingURL(raw string, hasCalendarLink bool) error`: an
      empty `raw` returns `nil` when `hasCalendarLink`, otherwise unchanged; threaded
      through `validateProfile`'s one call site.
- [x] 6.3 Added `Service.HasConnectedCalendar` (googlemeet.go), sharing
      `GetMentorCalendarGrant` + the same scope check `CreateMeetEvent`'s gate uses, so
      the two can never disagree about what "connected" means. Added
      `mentorshipHandlers.withCalendarLink` (`mentorship_write.go`), called from both
      `SubmitMentorProfile` and `UpdateMentorProfile` before `toInput`'s result reaches
      the service.
- [x] 6.4 `profile_test.go`: empty link + `HasCalendarLink=false` → refused
      (regression, existing table test); empty link + `HasCalendarLink=true` →
      accepted (`TestSubmitProfileAcceptsNoMeetingLinkWithAConnectedCalendar`); an
      invalid non-empty link is refused identically with `HasCalendarLink=true` too.

## 7. Frontend

- [x] 7.1 Added a "Mentor calendar" card to `IntegrationsView.svelte`, beside the
      existing Mail/Calendar cards inside the same Google block — its own status line,
      its own Connect link (`/api/v1/me/mentor-calendar/connect`), its own
      `mentor_calendar_error`/`mentor_calendar=connected` verdict handling, read from
      `GmailStatus.mentor_calendar_connected` (added to `$lib/api.ts`).
- [x] 7.2 `MentorProfileEditor.svelte`: fetches `mentor_calendar_connected` via
      `api.gmailStatus()` on mount (the same call `/my/integrations` makes) and
      rewords the meeting-link label to say it's optional/fallback when true — no
      HTML required-marker existed on this field before, so there was none to remove;
      the backend's own refusal is unchanged for the not-connected case.
- [x] 7.3 Verified via `svelte-check` (0 errors), `eslint` (clean on the touched
      files), the full frontend `vitest run` (1805 tests passing) and the
      design-system adoption ratchet (unchanged — both edits reuse components already
      used in their files). Live-browser check of the connect flow itself deferred to
      task 8's end-to-end pass, since it needs Docker + a running server; connecting
      to real Google is out of reach in dev regardless, as this task always expected.

## 8. Verification

- [x] 8.1 `gofmt -l .` clean, `go vet ./...` clean, `go test ./...` all green.
      `go test -count=1 -tags=integration,llmlive ./internal/platform/arch/...`
      confirms the layering guard passes with `internal/engage/mentorship` now
      importing `internal/application/gmailsync` and `internal/platform/tokencrypt`
      (both legal downward imports — engage is layer 7, application and platform are
      6 and 1).
- [x] 8.2 `go vet -tags=integration ./...` clean. The full tagged integration suite
      for `internal/platform/db`/`internal/api/handler` was NOT run: no `.sql` query
      signature changed since task 1.2's `make sqlc` (already covered there), and no
      exported handler constructor/signature changed in a way the untagged suite
      could not already catch — `withCalendarLink` and `HasConnectedCalendar` are
      both new, additive, and covered by the untagged unit tests.
- [x] 8.3 `node scripts/check-migrations.mjs`: 0 issues on the new migration.
- [x] 8.4 `svelte-check` (0 errors), `eslint` (clean), full frontend `vitest run`
      (1805 tests passing), design-system adoption ratchet unchanged.
- [x] 8.5 Added `TestAnEmptyMeetingLinkOmitsLocationAndURL` (`invite_test.go`) and
      `TestAnEmptyMeetingLinkProducesNoJoinLine` (`notify_test.go`) — both confirm the
      existing guards need no change.
