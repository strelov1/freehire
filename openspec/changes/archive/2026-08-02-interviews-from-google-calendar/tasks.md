## 1. Schema

- [x] 1.1 Write `migrations/0070_interview_schedule.sql` — `emails.ical_uid text`,
      `gmail_connections.scopes text[] NOT NULL DEFAULT '{}'`, and
      `application_interviews (id, user_id, application_id, job_id, ical_uid, starts_at,
      ends_at, title, join_url, status, source, created_at, updated_at)` with
      `UNIQUE (user_id, ical_uid)` and the per-user index the cascade needs. Comment each
      column with the reasoning the spec carries, in the manner of 0062 and 0064.
      Confirm 0069 is the newest applied on production before choosing the number.
- [x] 1.2 Add the queries in `internal/db/queries/` and run `make sqlc`: upsert a matched
      meeting keyed on `(user_id, ical_uid)`, mark one cancelled, list a caller's meetings
      in a range, resolve an application by an email's `ical_uid`, and read/write
      `gmail_connections.scopes`.

## 2. The invitation's UID

- [x] 2.1 RED: a unit test over the MIME/part extraction proving a `text/calendar` part's
      `UID` is captured, that a message without one yields empty, and that a malformed
      part does not fail the message.
- [x] 2.2 Capture the UID in `internal/gmailsync` (the API reader's parts) and in
      `internal/mailingest` (the MIME walk), and persist it through the existing upserts.
      `UpsertExternalEmail` must keep refreshing content columns only.
- [x] 2.3 Integration test: an ingested invitation carrying an ICS part stores its UID;
      re-ingesting the same message does not change it.

## 3. The calendar grant

- [x] 3.1 Extend `internal/gmailsync/connector.go` with a calendar consent URL and scope
      set, leaving the Gmail consent's scopes untouched. Unit test: the mail consent URL
      requests no calendar scope, and the calendar one does.
- [x] 3.2 Record granted scopes on exchange; add the connect/callback routes for the
      calendar alongside the Gmail pair, cookie-only for the same reason (it redirects a
      browser to Google).
- [x] 3.3 Handler tests: connecting the calendar leaves an existing mailbox connected, and
      a caller with no Google grant can start the calendar flow.

## 4. Matching

- [x] 4.1 Add `internal/calmatch` with the tier vocabulary, mirroring `internal/mailmatch`:
      `TierUID` links, everything else suggests. Pure — no database access.
- [x] 4.2 Unit tests: a UID match links; a title naming a tracked employer suggests and
      does not link; an organiser domain never links on its own; a UID belonging to
      another user's mail does not link.

## 5. The sync worker

- [x] 5.1 Add `internal/calsync` mirroring `gmailsync`: `Connection`, a narrow `Store`,
      `CalendarReader` behind a `ReaderFactory`, and `Worker.RunOnce` over the ±90-day
      window.
- [x] 5.2 RED first, and this is the privacy invariant: a fake reader returning personal
      events plus one matched interview must leave the store holding exactly one row.
      Assert the store, not the log.
- [x] 5.3 A rejected token marks that connection `needs_reconsent` and the run continues;
      a connection whose recorded scopes lack the calendar is skipped without an API call.
- [x] 5.4 Reschedule updates in place; cancellation marks; a re-run changes nothing
      (idempotent on `(user_id, ical_uid)`).
- [x] 5.5 Add `cmd/cal-sync` with `worker.Bootstrap`, reporting failure through its exit
      code the way `cmd/classify-mail` now does.

## 6. The ledger and the reader

- [x] 6.1 Add `appevent.KindInterviewScheduled` to the vocabulary and its tests; the
      source is the calendar grant, and the pin test in `appevent` must get its verdict.
- [x] 6.2 Record the event once, dated at the observation, inside the statement that
      upserts the meeting — the same discipline that keeps `MarkJobApplied` and its event
      from drifting. Integration test: a reschedule leaves exactly one event.
- [x] 6.3 Serve scheduled meetings to the SPA: extend the timeline read, or add a sibling
      read, so the calendar receives both layers in the one range request the day panel
      depends on.

## 7. The view

- [x] 7.1 Render a scheduled meeting distinctly from a recorded event, on the day it is
      due in the reader's timezone, using a design-system token — `check:tokens` counts
      raw palette utilities per file.
- [x] 7.2 The day panel entry carries the time, the title, the application, and the
      joining link when there is one; a cancelled meeting is shown as cancelled.
- [x] 7.3 A month holding only a future interview must not show the empty-month message.
      Extend the model's own tests for it.
- [x] 7.4 The connect surface says plainly that the grant is limited to test accounts
      until Google verification.

## 8. Verify

- [x] 8.1 `go build ./...`, `go vet ./...`, `gofmt -l .`, `go test ./...`, and
      `go test -tags=integration ./...` — both runs.
- [x] 8.2 `web/`: `pnpm run check`, `pnpm run build`, `pnpm exec vitest run`, and
      `design-system`'s `check:tokens` and `check:adoption`, recording an adoption
      improvement with `--update` if one lands.
- [x] 8.3 Visual check against a running stack seeded with a matched meeting, a cancelled
      one, and a personal event that must appear nowhere.
