# Booking links

## Scope
A Pro or Ultra candidate turns on one unguessable link per application; a recruiter opens it,
sees 30-minute slots and books one with a name and an address, no account (GH issue 4915, epic
4912). Layer 7 (`engage`). Tables `booking_links` and `booking_link_bookings` (migration
0287), queries in `internal/platform/db/queries/booking_links.sql`. HTTP is
`internal/api/handler/booking_links.go` (contract `typespec/booking-links.tsp`), the public
page `web/src/routes/booking/[token]`, the candidate's switch in the job drawer's History tab.

## Always true
- **Slots come from `internal/engage/slots` only**, over the package constants (weekdays
  10:00-18:00 in the zone stored on the link, 30 minutes, 12 h notice, 14-day horizon) minus
  the owner's confirmed bookings across all their links and their other confirmed
  application interviews (no end = 30 minutes; `booking_link` ones counted once).
- **A booking re-derives its slot** server-side and takes it only on an exact `Start.Equal`
  match. The EXCLUDE `booking_link_bookings_no_overlap` is what wins a race; its 23P01 (and
  the 40P01 retry that meets it) becomes `ErrSlotUnavailable`. Off-grid, taken, inside the
  notice, past the horizon and a lost race are ONE answer: 409 `slot_unavailable`.
- **The token is never stored.** 32 bytes from `crypto/rand` are the seed; the token is
  HMAC-SHA256(server key, seed) and only SHA-256 of the token is looked up by. The server key
  is `JWT_SECRET`, so the candidate sees the same URL every time and a database dump alone
  books nothing. A changed key re-points `token_hash` the next time the owner reads the link
  (`tokenFor`); URLs shared before the change stop resolving.
- **The public routes never name** the candidate, their address, the application id or other
  bookings. An unknown token and a link whose owner is no longer paid are the same 404.
- **Paid only.** Turning a link on calls `plan.Store.RequirePaid` first (402, nothing
  written); the public read and the booking ask again for the owner.
- **A booking shows on the application** through `UpsertApplicationInterview` in the same
  transaction: `ical_uid = 'booking:<id>'`, `source = 'booking_link'`, `confirmed`, and its
  `interview_scheduled` ledger event with source `system` (an anonymous booker is not the
  employer's pipeline, so it is not trusted for day math).
- The anonymous pair is throttled per IP at the gateway (`freehire-ops`, `/api/v1/booking/*`),
  and `POST .../bookings` refuses an encoded body before reading it (`refuseEncodedBody`).

## Not here yet
Google free/busy (#4916), calendar event or Meet (#4917), a schedule editor (#4918), cancel
(#4919), revoking by stage (#4920), turning a link off.
