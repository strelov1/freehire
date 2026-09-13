## 1. Repository: split booked vs. synced-busy

- [x] 1.1 In `internal/engage/mentorship/booking_repository.go`, extract an unexported
      helper that reads `ListMentorBusyBookings` and `ListMentorBusyIntervals`
      separately and returns them as two `[]Interval` (not unioned).
- [x] 1.2 Have the existing `ListBusy` call that helper and union the results, keeping
      its exported signature and behavior unchanged.
- [x] 1.3 Add a new exported repository method (e.g. `ListBusyByKind`) that returns the
      two `[]Interval` separately, for the day-breakdown path.

## 2. Domain: day-breakdown computation

- [x] 2.1 Add `internal/engage/mentorship/calendar.go` with a `CalendarRequest` /
      `CalendarResult` pair (mirroring `SlotRequest`/`SlotResult` in shape) and a
      `Calendar(req CalendarRequest) (CalendarResult, error)` function.
- [x] 2.2 Implement the partition: expand the schedule (`expandSchedule`), compute the
      free/bookable slots the same way `Slots()` does (`subtractBusy` + `sliceSlots` +
      the `earliest`/`latest` notice-and-horizon bounds), then label each moment in the
      window `booked` / `busy` / `free` / `closed` by priority (booked over busy over
      free over closed), per design.md's "Label exactly the sliced, grid-aligned slots
      as `free`" decision.
- [x] 2.3 Unit tests: a day with no availability is entirely `closed`; a booked hour is
      `booked` and not also `busy`/`free`; a synced busy interval is `busy` and carries
      no extra detail; a buffer-widened sliver is `closed`, not `free`; a past interval
      is never `free`; the labeled `free` ranges match what `Slots()` would offer for
      the same inputs (a property/table test comparing the two).

## 3. Service layer

- [x] 3.1 Add `Service.MyCalendar(...)` in `internal/engage/mentorship/schedule_service.go`,
      mirroring `slotsFor`/`MyAvailability`: resolve the caller's own profile
      (`ownProfile`), load rules, load booked+busy via `ListBusyByKind`, and call
      `Calendar(...)`. Signature ended up `(ctx, userID int64, year int, month
      time.Month)` rather than the `month time.Time` sketched above — explicit
      year+month avoids "which day of a time.Time counts" ambiguity at the call site.
- [x] 3.2 Unit test the service method against a fake repository (see existing
      `fake_repo_test.go`), including the "no profile" refusal already used elsewhere.

## 4. HTTP handler and route

- [x] 4.1 Add `GetMyCalendar` to `internal/api/handler/mentorship_availability.go`:
      parses an optional `?month=YYYY-MM` (default current month), calls
      `Service.MyCalendar`, and serializes the per-day categorized intervals in the
      mentor's own IANA zone.
- [x] 4.2 Register `GET /me/mentorship/availability/calendar` behind `mw.key` next to
      the existing `/me/mentorship/availability` routes in `mentorship.go`.
- [x] 4.3 Handler test: malformed `month` is refused (422); an authenticated mentor gets
      their own breakdown; a signed-out or non-mentor caller is refused — matching the
      spec's two auth scenarios.

## 5. Frontend: API client and data loading

- [x] 5.1 Add a client function (e.g. `myMentorCalendar(month)`) to `web/src/lib/api.ts`
      calling the new endpoint.
- [x] 5.2 Wire `web/src/routes/my/mentorship/schedule/+page.server.ts` to load the
      current month's breakdown alongside the existing availability load.

## 6. Frontend: read-only calendar component

- [x] 6.1 Add `web/src/lib/components/MentorScheduleCalendar.svelte`: a month grid with
      prev/next navigation, modeled on `MentorBookingView.svelte`'s grid mechanics but
      rendering a compact multi-status indicator per day cell instead of a clickable
      slot list. (No `?month=` in the URL: unlike the public booking page, there is no
      sign-in redirect to survive, so the month is kept in local component state —
      YAGNI over copying the pattern verbatim.)
- [x] 6.2 Clicking a day expands that day's labeled intervals as a simple list below the
      grid (start–end, status), in the mentor's own timezone.
- [x] 6.3 Fetch subsequent/previous months client-side via the new API client function
      as the mentor navigates, matching `MentorBookingView`'s pattern.
- [x] 6.4 Render the component above the existing `MentorSessionSettings` +
      `MentorScheduleEditor` on `/my/mentorship/schedule/+page.svelte`; no changes to
      the existing form editor.
- [x] 6.5 Live-verified via a throwaway Playwright script against a real backend +
      Postgres (seeded mentor with a confirmed booking, a synced busy interval, and a
      weekly rule): month grid renders with per-day status dots, clicking a day shows
      the correct labeled intervals (including a multi-day `closed` span rendered with
      full date+time, not a confusing "00:00–00:00"), month navigation re-fetches, and
      no control on the calendar attempts to edit availability. Screenshots reviewed.
      No permanent component test file — this codebase has none for the sibling
      `MentorBookingView`/`MentorScheduleEditor` components either; coverage lives in
      `mentorship.ts`'s pure-logic unit tests instead.

## 7. Tab reorder

- [x] 7.1 In `web/src/lib/mentorshipTabs.ts`, reorder the tab list so Profile is first,
      ahead of Sessions and Bookings; Schedule stays last. No path or gating changes.
- [x] 7.2 Verify `web/src/routes/my/mentorship/+layout.svelte` renders the new order
      (it reads the list directly, so this should need no code change there — confirm
      and adjust only if the strip hardcodes an order). Confirmed: `tabs` is derived by
      mapping over `MENTORSHIP_TABS` in array order, no change needed.

## 8. Verification

- [x] 8.1 `gofmt -l .`, `go vet ./...`, `go test ./...` clean. Note: `go test ./...`
      shows one pre-existing failure, `cmd/billing-sync`'s
      `TestTheStoreProviderAloneKeepsTheWorkerRunning`, in a package this change never
      touches (unrelated `identity`-layer billing worker) — confirmed pre-existing, not
      a regression from this change.
- [x] 8.2 `go vet -tags=integration ./...` clean (handler test touches integration-tagged
      files per `internal/api/handler`'s convention).
- [x] 8.3 Manually verified in the browser: `/my/mentorship/profile` tab order (Profile
      first), and `/my/mentorship/schedule` showing a real mentor's
      booked/busy/free/closed month, including a synced-busy interval standing in for a
      connected calendar. See 6.5.
