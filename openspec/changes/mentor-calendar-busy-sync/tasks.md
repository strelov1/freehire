## 1. Database

- [x] 1.1 Add migration: `ALTER TABLE gmail_connections ADD COLUMN
      mentor_busy_sync_opted_in boolean NOT NULL DEFAULT false` (additive, no backfill).
      Check the latest filed migration number under `migrations/` and file the next one.
      Filed as `0157_gmail_connections_mentor_busy_sync_opted_in.sql`.
- [x] 1.2 Add `SetMentorBusySyncOptedIn` (`gmail.sql`, sets the new column true for a
      user id) and `ListMentorBusySyncConnections` (`gmail.sql`, joins `gmail_connections`
      to `mentors` — `status = 'connected'`, `mentor_busy_sync_opted_in`, `calendar.readonly
      = ANY(scopes)`, and the mentor's own profile `status = 'approved'` — returning
      `mentor_id`, `user_id`). Add `UpsertMentorBusyInterval` and
      `DeleteMentorBusyIntervalsInWindow` to `mentorship.sql` (the latter scoped to
      `mentor_id`, `source = 'google_calendar'`, and `starts_at < window_end`). Run
      `make sqlc`. Also extended `GetGmailConnection` with the new column, needed by
      task 3.3. The scope string is passed as a query parameter
      (`sqlc.arg(calendar_scope)`) rather than spelled in SQL, matching
      `ListConnectedGmailUsers`'s own comment about why that string must live in one
      place.

## 2. `gmailsync`: the fourth incremental scope consumer

- [x] 2.1 Add `Connector.MentorBusyAuthCodeURL(state string) string` and
      `Connector.ExchangeMentorBusy(ctx, code) (refreshToken string, scopes []string, err
      error)`, mirroring `MentorCalendarAuthCodeURL`/`ExchangeMentorCalendar` exactly but
      over the EXISTING `CalendarScope` (not a new constant — Google has one read scope
      for the whole calendar, per design.md): own `mentorBusyRedirect` field
      (`origin + "/api/v1/me/mentor-busy-sync/callback"`), scope `[]string{CalendarScope}`,
      `include_granted_scopes=true`.
- [x] 2.2 Unit test: `MentorBusyAuthCodeURL` requests `calendar.readonly`, and its redirect
      differs from `CalendarAuthCodeURL`'s, `MentorCalendarAuthCodeURL`'s, and the sign-in
      flow's own — four distinct redirects sharing at most a scope, never a redirect.

## 3. HTTP: connect/callback + status

- [x] 3.1 Add `mentorBusyStateCookieName = "hire_mentor_busy_state"` in
      `internal/api/handler/gmail.go`; add `MentorBusySyncConnect`/`MentorBusySyncCallback`
      handlers mirroring `MentorCalendarConnect`/`MentorCalendarCallback` (own state
      cookie, own redirect target, `UpsertCalendarGrant`/`RecordGrantScopes` persistence
      exactly as the existing read-only calendar flow uses, PLUS
      `SetMentorBusySyncOptedIn` on success — the flag design.md calls for since the scope
      alone cannot distinguish this consent from the pre-existing candidate-side calendar
      grant), same `?mentor_busy_error=...` / `?mentor_busy=connected` redirect-to-
      `/my/integrations` convention.
- [x] 3.2 Register `GET /me/mentor-busy-sync/connect` (`mw.cookie`) and
      `GET /me/mentor-busy-sync/callback` (`mw.optionalCookie`) inside the existing
      `if h.gmailReady()` block in `register()`.
- [x] 3.3 Extend `GmailStatus`'s response with `"mentor_busy_sync_connected":
      conn.MentorBusySyncOptedIn && slices.Contains(conn.Scopes, gmailsync.CalendarScope)
      && conn.Status == "connected"`, alongside the existing `calendar_connected` and
      `mentor_calendar_connected` (both branches: the connected row and the no-row/
      not-connected default). Done in task 1.2 already (`GetGmailConnection` extended,
      `make sqlc` run).
- [x] 3.4 Unit test for `MentorBusySyncConnect` (RequiresAuth +
      SendsToGoogleForCalendarReadonlyAlone — own cookie, own scope, none of the other
      three flows' cookies), mirroring `me_mentor_calendar_test.go`. No unit test for
      `MentorBusySyncCallback`, matching the precedent `MentorCalendarCallback` already
      set (needs a database). Integration test (`gmail_integration_test.go`, extends
      `TestGmailInboxEndToEnd`) for `GmailStatus`: opted-in but scope missing reports
      `mentor_busy_sync_connected: false`; scope present but not opted in also reports
      `false` (the unrelated-grant case); both together reports `true`; `status =
      'needs_reconsent'` with both still present reports `false`.

## 4. Domain: `internal/engage/mentorship/busysync` — the sync worker

- [x] 4.1 New package `internal/engage/mentorship/busysync`. Define `Connection{MentorID,
      UserID int64}`, `BusyPeriod{Start, End time.Time}`, a `FreeBusyReader` interface
      (`ListBusy(ctx, from, to time.Time) ([]BusyPeriod, error)`) behind a
      `ReaderFactory(ctx, refreshToken string) FreeBusyReader`, and a `Store` interface:
      `ListConnections(ctx) ([]Connection, error)`, `RefreshToken(ctx, userID int64)
      (encToken string, err error)`, `ReplaceBusyWindow(ctx, mentorID int64, windowEnd
      time.Time, periods []BusyPeriod) error` (the transactional delete-then-insert
      design.md specifies), `SetNeedsReconsent(ctx, userID int64) error`. Mirrors
      `calsync`'s `Store`/`Worker` shapes deliberately (see design.md's Context).
- [x] 4.2 `Worker{store, cipher, newReader, now}` with `NewWorker(...)` and `RunOnce(ctx)
      error`, copying `calsync.Worker.RunOnce`'s best-effort-per-connection shape and
      `RevokedGrant` handling verbatim in structure: one connection's non-revocation
      error is logged and counted, a revocation-shaped one calls `SetNeedsReconsent`, and
      the run returns a "N of M failed" error when any failed.
- [x] 4.3 `syncMentor`: reads the token, decrypts it, calls
      `ListBusy(now, now.AddDate(0,0,busyWindowDays))` (`busyWindowDays = 60`, per
      design.md's fixed-forward-window decision — a named constant, not inlined), and
      calls `ReplaceBusyWindow` with the raw periods. Deviation from the task text: the
      `external_id` hash is derived in `DBStore.ReplaceBusyWindow` (task 4.4), not here —
      it is purely a storage-layer concern for the SQL unique constraint, and keeping
      `BusyPeriod`/`Store` free of it keeps the domain worker's tests (4.6) about sync
      behavior rather than hashing.
- [x] 4.4 Implemented `Store` as a new adapter, `busysync.DBStore` (mirroring
      `calsync.DBStore`'s own shape — a thin wrapper over `*db.Queries` plus the pool it
      needs for one transaction — rather than extending `mentorship.QueriesRepository`,
      which serves a different concern: profiles, bookings, reviews): `ListConnections`
      over `ListMentorBusySyncConnections`, `ReplaceBusyWindow` running
      `DeleteMentorBusyIntervalsInWindow` then one `UpsertMentorBusyInterval` per period
      inside a transaction (with `externalID`, the hash from task 4.3's note),
      `SetNeedsReconsent` reusing `SetGmailStatus`.
- [x] 4.5 `FreeBusyReader` HTTP implementation: `POST
      https://www.googleapis.com/calendar/v3/freeBusy` with `timeMin`/`timeMax`/
      `items: [{id: "primary"}]`, parsing `calendars.primary.busy[]` into `BusyPeriod`.
      A non-2xx response wraps as `gmailsync.APIError` (mirroring `calsync.APIReader`'s
      style) so `gmailsync.RevokedGrant` can classify it.
- [x] 4.6 Unit tests (`worker_test.go`): periods round-trip into `ReplaceBusyWindow` with
      the right window bound; a revocation-shaped reader error calls `SetNeedsReconsent`
      and does NOT call `ReplaceBusyWindow`; a non-revocation error does not call
      `SetNeedsReconsent`; one failing connection among several does not stop the others
      (the spec's own scenario), and the run's returned error names the failure count.
      Freebusy HTTP reader tested against `httptest` (`freebusyapi_test.go`, mirroring
      `calsync.calendarapi_test.go`'s rewrite-transport pattern): the request itself
      (POST, primary calendar, timeMin/timeMax) and a normal response parse correctly; a
      403 wraps as `gmailsync.APIError` and `RevokedGrant` reports true. Integration
      tests (`dbstore_integration_test.go`, real Postgres via testcontainers):
      `ListConnections` requires the opt-in flag AND the scope AND `connected` status AND
      an approved profile all at once — missing any one excludes the mentor, including
      the unrelated-grant case (scope without the flag) this whole feature exists to get
      right; `ReplaceBusyWindow` replaces rather than merges (a stale interval inside the
      window is gone after a resync with different periods) and a re-sync of the same
      period updates rather than duplicates the row.

## 5. `cmd/mentor-busy-sync`

- [x] 5.1 New `cmd/mentor-busy-sync/main.go` following this repo's cron-worker
      conventions (`internal/platform/worker`'s `Main`/`Bootstrap` — see its AGENTS.md):
      needs `DATABASE_URL` and the Google OAuth client config `gmailsync.Connector`
      needs; wires `busysync.NewWorker` with `busysync.ReaderFactoryFor` (a Google-backed
      `ReaderFactory`, mirroring `calsync.ReaderFactoryFor`) and runs `RunOnce`. Mirrors
      `cmd/cal-sync/main.go` line for line. Also added `mentor-busy-sync` to
      `internal/platform/arch/layering/blocks.go`'s `engage` list (as
      `mentorship/busysync`, the `auth/oauth` naming convention for a sub-package) and to
      `.gitignore` — both required by `internal/platform/arch/layering`'s and
      `internal/platform/arch`'s own tests.
- [x] 5.2 `deploy/systemd/freehire-mentor-busy-sync.service` and `.timer` (a `Type=oneshot`
      unit + timer, hourly like `freehire-cal-sync.timer` and offset from it and
      `gmail-sync`'s). Documented in `AGENTS.md`'s worker table (a new bullet beside
      `mentorship-remind`) and in `internal/engage/mentorship/AGENTS.md`'s "How it works"
      section that this unit needs hand-provisioning on the host and is not touched by
      `release.sh`.

## 6. Frontend

- [ ] 6.1 Add a "Mentor calendar sync" card to `IntegrationsView.svelte`, beside the
      existing Google cards — its own status line, its own Connect link
      (`/api/v1/me/mentor-busy-sync/connect`), its own `mentor_busy_error`/
      `mentor_busy=connected` verdict handling, read from
      `GmailStatus.mentor_busy_sync_connected` (added to `$lib/api.ts`).
- [ ] 6.2 Verify via `svelte-check` (0 errors), `eslint` (clean on the touched file), the
      full frontend `vitest run`, and the design-system adoption ratchet (unchanged — the
      card reuses components `IntegrationsView.svelte` already uses).

## 7. Verification

- [ ] 7.1 `gofmt -l .` clean, `go vet ./...` clean, `go test ./...` all green.
- [ ] 7.2 `go test -count=1 -tags=integration,llmlive ./internal/platform/arch/...`
      confirms the layering guard passes with the new `internal/engage/mentorship/busysync`
      package importing `internal/application/gmailsync` and `internal/platform/tokencrypt`
      (both legal downward imports — engage is layer 7, application and platform are 6
      and 1).
- [ ] 7.3 `go vet -tags=integration ./...` clean. Run the full tagged integration suite
      for `internal/platform/db`/`internal/api/handler` if any `.sql` query signature
      changed in a way the untagged suite could not already catch.
- [ ] 7.4 `node scripts/check-migrations.mjs`: 0 issues on the new migration.
- [ ] 7.5 `svelte-check`, `eslint`, full frontend `vitest run`, design-system adoption
      ratchet — all clean/unchanged.
- [ ] 7.6 `/code-review` pass on the full diff; fix Critical + Important findings with a
      regression test each, matching how `mentor-google-meet-link`'s own task 9 closed
      out its post-review fixes.
