## Why

A member can book a mock only from another member's page, and nothing tells them whose
page to open: the mocks page shows the member's own settings and bookings and no one
else. A guest has no way to discover who runs mocks.

## What Changes

- Add `GET /api/v1/me/buddies/mocks/hosts` (cookie auth, active members only): the
  members the viewer could book right now, each with handle, the card summary every
  buddy list shows, topics, the soonest open slot in the viewer's zone, and the
  member-page review aggregate. Optional `topic` filter; soonest slot first.
- Slots come from the existing engine and mock shape (12 h notice, 30-day horizon); the
  candidates, their availability, busy time and review aggregates are read in batched
  statements, and the candidate set is capped at 200 by most recently saved settings.
- The member-page summary query becomes a list query (`ListBuddyMemberMockSummaries`) so
  the listing and the member page share one statement of the reveal-and-block rule.
- The mocks page gains a "Who runs mocks" block and a reworked availability editor.

Not in scope: booking from the listing itself (the Book action opens the member page's
mock tab), and the route is not added to `web/static/openapi.yaml`, which documents no
buddy route.

## Impact

- `internal/engage/buddy` (`mock_hosts.go`, repository), `internal/platform/db/queries/buddy_mocks.sql`,
  `internal/api/handler/buddy_mocks.go`, `web/src/routes/buddies/mocks`.
