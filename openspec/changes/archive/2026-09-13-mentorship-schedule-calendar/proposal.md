## Why

A mentor configuring their availability on `/my/mentorship/schedule` today only sees the
raw rules they typed in (weekly hours, dated overrides) — there is no visual feedback
showing what that actually resolves to once confirmed bookings and their synced Google
Calendar commitments are subtracted, unlike the cal.com-style experience mentors are used
to elsewhere. Separately, a new mentor's first task after their profile is approved is
filling in the Profile tab, but it currently sits third in the mentorship section's tab
order (Sessions, Bookings, Profile, Schedule) rather than first.

## What Changes

- Reorder the mentorship section's tabs so **Profile** is first, ahead of Sessions and
  Bookings.
- Add a read-only, cal.com-style month calendar to `/my/mentorship/schedule` showing the
  mentor their own resolved time for each day — booked, synced-busy (from Google Calendar
  sync), free, and outside-availability — reusing the visual pattern already built for the
  public booking calendar (`MentorBookingView.svelte`). The existing weekly-hours/overrides
  form remains the only way to edit availability; the calendar is a view, not an editor.
- Add a new authenticated endpoint that returns this categorized day breakdown for a
  requested month, computed the same way the public slot engine computes bookable slots
  (reusing its schedule-expansion and busy-subtraction logic) but exposing the categories
  instead of collapsing them into a single free-slot list.

## Capabilities

### New Capabilities
- `mentor-schedule-calendar`: lets a mentor see their own availability, bookings and
  synced-calendar busy time as a categorized month calendar, computed on read.

### Modified Capabilities
(none — the tab reorder is presentational only and the new capability reuses the existing
`mentor-availability` computation internally without changing its requirements, and reuses
`mentor-busy-sync`'s stored intervals without changing what is synced or how)

## Impact

- **Backend**: `internal/engage/mentorship` (new day-breakdown computation alongside the
  existing `Slots`), `internal/api/handler/mentorship_availability.go` (new authenticated
  route under `/me/mentorship/...`). No schema change — reuses `mentor_busy_intervals` and
  the existing bookings/availability tables.
- **Frontend**: `web/src/routes/my/mentorship/schedule/+page.svelte` and its `+page.server.ts`
  (load the new endpoint), a new calendar component under `web/src/lib/components/`
  modeled on `MentorBookingView.svelte`, the mentorship tab list module (`+layout.svelte` /
  `mentorshipTabs`) for the reorder, and the mentorship API client module.
- No breaking changes; no changes to the public booking flow.
