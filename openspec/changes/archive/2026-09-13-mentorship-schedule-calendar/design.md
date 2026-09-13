## Context

The slot engine (`internal/engage/mentorship`, see `mentor-availability` spec) already
computes bookable slots from a mentor's rules, session parameters and busy set
(`Slots()` in `slots.go`, calling the unexported `expandSchedule`/`subtractBusy`/
`sliceSlots` pipeline). The busy set itself unions two sources in
`booking_repository.go`'s `ListBusy`: confirmed bookings and synced Google Calendar
intervals (`mentor-busy-sync`). Today that union is the ONLY thing exposed to a
consumer, and only in its final, collapsed form — a flat list of offerable slots
(`GetMentorSlots`, public) or the raw declared rules with no busy information at all
(`GetMyAvailability`, mentor-only). See proposal.md for why a mentor needs more than
that on `/my/mentorship/schedule`.

## Goals / Non-Goals

**Goals:**
- Give a mentor a single read that categorizes their own time (`booked` / `busy` /
  `free` / `closed`) for a month, computed by the same pipeline the public booking flow
  uses, so the two can never disagree.
- Keep the existing weekly-hours/overrides form as the only way to edit availability.

**Non-Goals:**
- Editing availability by interacting with the calendar (dragging, clicking a cell to
  toggle it). That is a separate, larger feature and was explicitly ruled out for this
  change.
- Changing what `mentor-busy-sync` stores, or exposing calendar event detail (title,
  attendee) to anyone — it was never stored, so there is nothing to expose.
- Changing the public `/mentors/:slug/slots` endpoint or its caching/rate-limiting.

## Decisions

**Reuse the slot engine's internals rather than a parallel implementation.** The new
day-breakdown computation lives in the same package and calls the same unexported
`expandSchedule` / `subtractBusy` / `sliceSlots` helpers `Slots()` already uses, rather
than recomputing schedule expansion or buffer/notice/horizon logic a second time.
*Alternative considered*: let the frontend derive the breakdown client-side from the
existing rules endpoint plus a new raw-busy-intervals endpoint. Rejected — it would
duplicate the buffer-widening, DST and grid-anchoring logic (see `mentor-availability`
spec's daylight-saving scenarios) in TypeScript, which is exactly the "two copies of a
predicate drift" trap this codebase avoids elsewhere.

**Label exactly the sliced, grid-aligned slots as `free`.** The breakdown does not treat
"free" as the raw remainder after subtracting busy time — it treats it as the actual
output of `sliceSlots`, filtered by the same `earliest`/`latest` notice-and-horizon
bounds `Slots()` applies. A sliver too short for a session, or one only closed off by
buffer widening, is labeled `closed`, never `free`. This makes the "breakdown agrees
with what a seeker can book" requirement hold by construction instead of by convention.

**Split booked vs. synced-busy at the repository layer.** `ListBusy` keeps its existing
signature and behavior (a flat union) for the public slot path. A new repository read
returns the two sources separately for the day-breakdown path. *Alternative
considered*: add a `Kind` field to the shared `Interval` type and thread it through
`Slots()`. Rejected — `Interval` is used throughout the public pipeline, where the
distinction is meaningless and, per the `mentor-busy-sync` spec, deliberately invisible;
widening a pervasive type for one caller's concern is the wrong seam.

**The response is in the mentor's own timezone, with no viewer-zone parameter.** Unlike
the public slot endpoint, the only possible reader of this endpoint is the profile
owner, so there is nothing to negotiate. *Alternative considered*: accept `?timezone=`
for parity with the public endpoint. Rejected as unneeded complexity — the existing
weekly-hours editor already assumes the mentor's own zone.

**No caching or rate-limiting on the new endpoint.** `GetMentorSlots` is cached and
rate-limited because it is public and this host serves mostly crawler traffic (see
`mentor-availability` spec). The new endpoint is authenticated, read by exactly one
account, and computed at the same cost as an uncached slot request — plain per-request
computation is enough.

**Calendar UI is a new, purpose-built component**, visually modeled on
`MentorBookingView.svelte`'s month grid (prev/next navigation, day cells, `?month=` in
the URL), but each cell renders a compact multi-segment bar summarizing that day's
categories instead of a clickable list of bookable slots — clicking a day expands the
day's intervals as a simple labeled list below the grid. No new shared component is
extracted from `MentorBookingView` for this change; both components independently
render a month grid, which is a small enough shape that a shared abstraction is not
worth it yet.

**Tab reorder is presentation-only.** The mentorship tab list becomes Profile, Sessions,
Bookings, Schedule. Sessions and Bookings keep their relative order — only Profile
moves to the front, matching it being a new mentor's first task; no route or gating
logic changes.

## Risks / Trade-offs

- A four-category day partition can produce thin `closed` slivers around a buffer-widened
  booking → Mitigation: the API returns raw, non-overlapping intervals; the frontend is
  free to visually merge adjacent same-status fragments when rendering a day cell — that
  is a display concern, not part of the response contract.
- Splitting `ListBusy`'s internals risks regressing the existing public slot path →
  Mitigation: `ListBusy`'s exported behavior and its existing tests are left unchanged;
  the split is additive (a new method), and the existing mentorship test suite
  (`slots_test.go`, `subtract_test.go`, `booking_test.go`, etc.) is the regression guard.
- The tab reorder changes a position mentors are already used to → no mitigation needed;
  it is a deliberate, low-cost UX change with no data or routing impact.

## Migration Plan

No database migration. Both the new endpoint and the new frontend component are
additive, so the backend and frontend deploys have no ordering constraint — an old
frontend simply never calls the new route, and the new frontend degrades to nothing
worse than a loading state if it somehow reached an old backend. Rollback is a plain
revert of the deploy.
