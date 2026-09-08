## ADDED Requirements

### Requirement: Availability is a recurring week plus dated overrides

The system SHALL hold a mentor's availability as rows of two kinds in one set: a
**weekly** row naming a weekday and a start and end time, and an **override** row
naming a calendar date and a start and end time. A row SHALL be one kind or the other
and never both, and the schema SHALL enforce that.

#### Scenario: A weekly row repeats

- **WHEN** a mentor states Tuesday 18:00–20:00 as a weekly row
- **AND** slots are requested for a four-week window
- **THEN** every Tuesday in that window offers availability from 18:00 to 20:00 in the
  mentor's timezone

#### Scenario: A row that names both a weekday and a date is refused

- **WHEN** a row carrying both a weekday and a date is written
- **THEN** the database rejects it

### Requirement: An override replaces its whole day

An override row for a date SHALL replace the weekly rows for that date entirely, not
supplement them. An override whose start time equals its end time SHALL mean the day
is closed.

#### Scenario: An override narrows a day rather than adding to it

- **WHEN** a mentor has a weekly Tuesday 18:00–20:00 row
- **AND** adds an override for one Tuesday of 10:00–12:00
- **THEN** that date offers 10:00–12:00 only
- **AND** offers nothing between 18:00 and 20:00

#### Scenario: An empty override closes a day

- **WHEN** a mentor adds an override for a date with equal start and end times
- **THEN** that date offers no slots
- **AND** the weekly rows for other dates are unaffected

### Requirement: Times are stored without a zone; the mentor's zone resolves them

Availability start and end times SHALL be stored as a time of day carrying no zone and
no date. The mentor's profile SHALL carry one IANA timezone, and that zone alone SHALL
resolve a stored time into an instant. A change of the mentor's timezone SHALL move
their future availability accordingly and SHALL NOT move any already-confirmed booking.

#### Scenario: Daylight saving does not shift the stated hour

- **WHEN** a mentor in `Europe/Berlin` states 18:00–20:00 weekly
- **AND** slots are requested for a window spanning the daylight-saving transition
- **THEN** every date in the window offers 18:00–20:00 local to the mentor
- **AND** the corresponding UTC instants differ by one hour either side of the transition

#### Scenario: A local time that does not exist on the transition day

- **WHEN** an availability window covers a local time that the mentor's zone skips on
  the spring transition date
- **THEN** slot generation completes without error
- **AND** every emitted slot is a real instant in that zone

#### Scenario: A zone whose clocks move at midnight

- **WHEN** the mentor's zone moves its clocks at midnight, so that one date's 00:00 does
  not exist — `America/Santiago`, `America/Havana` and `Atlantic/Azores` each do this
  once a year
- **THEN** slot generation over a window spanning that date terminates
- **AND** a dated override for that date resolves onto that date, not the one before it

#### Scenario: A local time that occurs twice on the transition day

- **WHEN** an availability window covers a local time that the mentor's zone repeats on
  the autumn transition date
- **THEN** no absolute instant is offered more than once
- **AND** neither occurrence is withheld — both are real hours the mentor stated
- **AND** consequently two slots may carry the same wall-clock label, distinguished only
  by their UTC offset, which the slot response SHALL carry for every slot

#### Scenario: Changing the timezone leaves booked sessions alone

- **WHEN** a mentor with a confirmed booking changes their profile timezone
- **THEN** the booking's start and end instants are unchanged
- **AND** future availability resolves against the new zone

### Requirement: Slots are computed on read and never stored

The system SHALL compute bookable slots for a requested window at read time from the
availability rows, the mentor's session parameters and the current busy set. It SHALL
NOT persist generated slots. The computation over a window SHALL be a pure function of
its inputs — schedule, parameters, busy intervals and the current instant — so that the
same inputs always yield the same slots.

#### Scenario: The same window computed twice yields the same slots

- **WHEN** slots are computed twice for one mentor and window with unchanged schedule,
  parameters, busy set and clock
- **THEN** both computations return an identical slot list

### Requirement: A slot is offered only if the session fits entirely

A slot SHALL be offered only when the mentor's full session duration fits inside a
free interval. Intervals SHALL be treated as half-open — a slot ending exactly when
another begins SHALL NOT be a conflict.

#### Scenario: A gap shorter than the session offers nothing

- **WHEN** a mentor's session is 60 minutes and a free interval is 45 minutes long
- **THEN** that interval offers no slot

#### Scenario: The slot grid is anchored to the schedule, not to the moment of the request

- **WHEN** the same window is requested at 18:00, at 18:30 and at 18:33, of a mentor
  available from 18:00 with hour-long sessions
- **THEN** every offered slot begins on the mentor's own hourly grid — 18:00, 19:00,
  20:00 — and never at 18:33
- **AND** only the notice period decides which of those slots are withheld

#### Scenario: Overlapping availability offers each hour once

- **WHEN** a mentor's rules overlap — the same row written twice, or a weekly evening
  crossing a dated override
- **THEN** each bookable hour is offered exactly once
- **AND** the slots are still in ascending order

#### Scenario: Back-to-back sessions do not conflict

- **WHEN** a mentor with a 60-minute session and no buffers has a confirmed booking
  from 18:00 to 19:00
- **THEN** the 19:00–20:00 slot is still offered

### Requirement: Busy time and buffers are subtracted before slicing

The system SHALL subtract from a mentor's availability every confirmed booking and
every interval in the mentor's busy set, each widened by the mentor's buffers. The
buffer after an existing engagement and the buffer before a prospective one SHALL both
apply, so the gap the system requires between two sessions is their sum.

#### Scenario: Buffers on both sides add up

- **WHEN** a mentor has a 15-minute after-buffer and a 10-minute before-buffer
- **AND** holds a confirmed booking ending at 19:00
- **THEN** no slot is offered starting before 19:25

#### Scenario: A cancelled booking frees its time

- **WHEN** a confirmed booking at 19:00 is cancelled
- **THEN** the 19:00 slot is offered again

#### Scenario: A busy interval blocks a slot without revealing itself

- **WHEN** the mentor's busy set holds an interval overlapping an available hour
- **THEN** that hour offers no slot
- **AND** the response carries no title, attendee, or description for the busy interval

### Requirement: Minimum notice and booking horizon bound the window

The system SHALL offer no slot beginning sooner than the mentor's minimum notice from
now, and no slot beginning later than the mentor's booking horizon from now. A
requested window wider than the horizon SHALL be clamped rather than refused.

#### Scenario: A slot inside the notice period is withheld

- **WHEN** a mentor's minimum notice is 120 minutes and an available slot starts in
  30 minutes
- **THEN** that slot is not offered

#### Scenario: A window beyond the horizon is clamped

- **WHEN** a mentor's horizon is 30 days and a visitor requests a 90-day window
- **THEN** the response carries slots for the first 30 days only
- **AND** the request succeeds rather than erroring

### Requirement: Slots are returned in the viewer's timezone

The slot endpoint SHALL accept the viewer's IANA timezone and SHALL express every
returned slot in it, alongside the absolute instant. An absent or unrecognised
timezone SHALL fall back to UTC rather than to the mentor's zone, and the response
SHALL say which zone it used.

#### Scenario: A viewer in another zone sees local times

- **WHEN** a viewer in `Asia/Tokyo` requests the slots of a mentor in `Europe/Berlin`
- **THEN** each slot carries the Tokyo wall-clock time and the same absolute instant

#### Scenario: An unrecognised timezone falls back and says so

- **WHEN** a viewer supplies a timezone identifier no zone database carries
- **THEN** the response is computed in UTC
- **AND** the response names UTC as the zone it used

### Requirement: Slot computation is cached and rate-limited

Because the slot endpoint is public and this host serves mostly crawler traffic, the
system SHALL cache a computed window and SHALL rate-limit the endpoint. The cache key
SHALL include the mentor, the window and the viewer's timezone.

Staleness SHALL be bounded by a short expiry rather than by explicit invalidation, and
the booking path SHALL NOT read the cache. A booking would otherwise have to invalidate
every cached window overlapping it in every viewer's timezone — a set the writer cannot
enumerate — and the shared cache interface offers no deletion. The consequence is stated
rather than hidden: a taken hour MAY be offered for up to the expiry, and the seeker who
takes it receives the ordinary "no longer available" refusal.

#### Scenario: A booked slot may be offered briefly, and refuses when taken

- **WHEN** a slot window has been cached
- **AND** a booking is confirmed inside it
- **THEN** a request for that window MAY still offer the booked slot until the entry
  expires
- **AND** an attempt to book it is refused as unavailable, because the booking path
  re-derives from the live schedule rather than reading the cache

#### Scenario: Two viewers in different zones do not share a cache entry

- **WHEN** one viewer requests a window as `Asia/Tokyo` and another requests the same
  window as `Europe/Berlin`
- **THEN** each is served slots expressed in their own zone

#### Scenario: An unreachable cache degrades rather than fails

- **WHEN** the cache is unavailable, whether reading or writing
- **THEN** slots are computed directly and the response is correct

#### Scenario: Pausing takes effect before the cache expires

- **WHEN** a mentor's window has been cached and the mentor then pauses
- **THEN** the next request for that window answers as though the mentor does not exist
- **AND** it does not serve the cached slots

The publication check SHALL therefore run BEFORE the cache is consulted. The cache exists
to save the slot computation, not the profile read — and a pause button that appears to do
nothing for a minute is used at exactly the moment that matters.
