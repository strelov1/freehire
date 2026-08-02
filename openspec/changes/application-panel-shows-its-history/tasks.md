## 1. Read one application's ledger

- [x] 1.1 Write the failing integration test for `ListApplicationEvents`: newest first, retracted
      absent, another caller's events absent, the limit respected, a deleted message leaving the
      event standing without a subject.
- [x] 1.2 Add the query beside `ListApplicationEventsInRange` and run `make sqlc`.
- [x] 1.3 Add `apptimeline.ForApplication` with its own test over the row→event mapping.

## 2. The events reach the panel

- [x] 2.1 Carry the events on `GET /me/tracking/:slug`, bounded by the limit.
- [x] 2.2 Update `web/src/lib/types.ts` for the new field.

## 3. One event vocabulary

- [x] 3.1 Emit `appevent.Kinds` into the generated contracts.
- [x] 3.2 Move the labels and colours out of `TrackingCalendar.svelte` into `web/src/lib/events.ts`,
      with a check that every generated kind has a label. Verify by mutation.
- [x] 3.3 Point `TrackingCalendar.svelte` at the shared module and confirm the calendar is
      unchanged on screen.

## 4. The panel

- [x] 4.1 Delete the header strip from `JobDrawer.svelte`.
- [x] 4.2 Render the history in the Application tab above Stage and Notes, newest first, and
      nothing at all when the ledger is empty.

## 5. Documentation

- [x] 5.1 Update `docs/API.md` and `web/src/lib/docs/api-spec.ts` for the detail response's
      `events`.
- [x] 5.2 Run both Go suites, all four web gates, and the design-system token ratchet.
- [x] 5.3 Offer a `/blog` changelog entry.
