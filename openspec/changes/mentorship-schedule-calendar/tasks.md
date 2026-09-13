## 1. Repository: split booked vs. synced-busy

- [ ] 1.1 In `internal/engage/mentorship/booking_repository.go`, extract an unexported
      helper that reads `ListMentorBusyBookings` and `ListMentorBusyIntervals`
      separately and returns them as two `[]Interval` (not unioned).
- [ ] 1.2 Have the existing `ListBusy` call that helper and union the results, keeping
      its exported signature and behavior unchanged.
- [ ] 1.3 Add a new exported repository method (e.g. `ListBusyByKind`) that returns the
      two `[]Interval` separately, for the day-breakdown path.

## 2. Domain: day-breakdown computation

- [ ] 2.1 Add `internal/engage/mentorship/calendar.go` with a `CalendarRequest` /
      `CalendarResult` pair (mirroring `SlotRequest`/`SlotResult` in shape) and a
      `Calendar(req CalendarRequest) (CalendarResult, error)` function.
- [ ] 2.2 Implement the partition: expand the schedule (`expandSchedule`), compute the
      free/bookable slots the same way `Slots()` does (`subtractBusy` + `sliceSlots` +
      the `earliest`/`latest` notice-and-horizon bounds), then label each moment in the
      window `booked` / `busy` / `free` / `closed` by priority (booked over busy over
      free over closed), per design.md's "Label exactly the sliced, grid-aligned slots
      as `free`" decision.
- [ ] 2.3 Unit tests: a day with no availability is entirely `closed`; a booked hour is
      `booked` and not also `busy`/`free`; a synced busy interval is `busy` and carries
      no extra detail; a buffer-widened sliver is `closed`, not `free`; a past interval
      is never `free`; the labeled `free` ranges match what `Slots()` would offer for
      the same inputs (a property/table test comparing the two).

## 3. Service layer

- [ ] 3.1 Add `Service.MyCalendar(ctx, userID int64, month time.Time) (CalendarResult, error)`
      in `internal/engage/mentorship/schedule_service.go`, mirroring `slotsFor`/
      `MyAvailability`: resolve the caller's own profile (`ownProfile`), load rules,
      load booked+busy via `ListBusyByKind`, and call `Calendar(...)`.
- [ ] 3.2 Unit test the service method against a fake repository (see existing
      `fake_repo_test.go`), including the "no profile" refusal already used elsewhere.

## 4. HTTP handler and route

- [ ] 4.1 Add `GetMyCalendar` to `internal/api/handler/mentorship_availability.go`:
      parses an optional `?month=YYYY-MM` (default current month), calls
      `Service.MyCalendar`, and serializes the per-day categorized intervals in the
      mentor's own IANA zone.
- [ ] 4.2 Register `GET /me/mentorship/availability/calendar` behind `mw.key` next to
      the existing `/me/mentorship/availability` routes in `mentorship.go`.
- [ ] 4.3 Handler test: malformed `month` is refused (422); an authenticated mentor gets
      their own breakdown; a signed-out or non-mentor caller is refused — matching the
      spec's two auth scenarios.

## 5. Frontend: API client and data loading

- [ ] 5.1 Add a client function (e.g. `myMentorCalendar(month)`) to `web/src/lib/api.ts`
      calling the new endpoint.
- [ ] 5.2 Wire `web/src/routes/my/mentorship/schedule/+page.server.ts` to load the
      current month's breakdown alongside the existing availability load.

## 6. Frontend: read-only calendar component

- [ ] 6.1 Add `web/src/lib/components/MentorScheduleCalendar.svelte`: a month grid with
      prev/next navigation and `?month=` in the URL, modeled on
      `MentorBookingView.svelte`'s grid mechanics but rendering a compact multi-segment
      status bar per day cell instead of a clickable slot list.
- [ ] 6.2 Clicking a day expands that day's labeled intervals as a simple list below the
      grid (start–end, status), in the mentor's own timezone.
- [ ] 6.3 Fetch subsequent/previous months client-side via the new API client function
      as the mentor navigates, matching `MentorBookingView`'s pattern.
- [ ] 6.4 Render the component above the existing `MentorSessionSettings` +
      `MentorScheduleEditor` on `/my/mentorship/schedule/+page.svelte`; no changes to
      the existing form editor.
- [ ] 6.5 Component test (or Playwright, per this project's extension-verification
      pattern) covering: month navigation, a day showing a mix of statuses, and that no
      control on the calendar attempts to edit availability.

## 7. Tab reorder

- [ ] 7.1 In `web/src/lib/mentorshipTabs.ts`, reorder the tab list so Profile is first,
      ahead of Sessions and Bookings; Schedule stays last. No path or gating changes.
- [ ] 7.2 Verify `web/src/routes/my/mentorship/+layout.svelte` renders the new order
      (it reads the list directly, so this should need no code change there — confirm
      and adjust only if the strip hardcodes an order).

## 8. Verification

- [ ] 8.1 `gofmt -l .`, `go vet ./...`, `go test ./...` clean.
- [ ] 8.2 `go vet -tags=integration ./...` clean (handler test touches integration-tagged
      files per `internal/api/handler`'s convention).
- [ ] 8.3 Manually verify in the browser: `/my/mentorship/profile` tab order, and
      `/my/mentorship/schedule` showing a real mentor's booked/busy/free/closed month —
      including a mentor with busy-sync connected, per this project's UI-change
      verification convention.
