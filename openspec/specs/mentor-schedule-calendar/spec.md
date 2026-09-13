# mentor-schedule-calendar Specification

## Purpose

Lets a mentor see their own resolved calendar — booked sessions, synced-busy time, free
bookable time and closed time — as one categorized month view, computed the same way the
public slot engine decides what a seeker may book.

## Requirements

### Requirement: A mentor may read a categorized breakdown of their own month

The system SHALL let an authenticated mentor request their own resolved calendar for a
specified month, and SHALL respond with each day's time partitioned into non-overlapping
intervals, each labeled exactly one of:

- `booked` — covered by a confirmed booking
- `busy` — covered by a synced busy interval from the mentor's connected calendar
- `free` — available and currently offered to a seeker as a bookable slot
- `closed` — outside the mentor's stated availability, or inside it but withheld by a
  buffer, minimum notice, or the booking horizon

The partition SHALL be computed at read time from the mentor's availability rules,
session parameters, and current busy set, and SHALL NOT be persisted. An unspecified
month SHALL default to the current month.

#### Scenario: A day with no configured availability is entirely closed

- **WHEN** a mentor requests the breakdown for a day that has no weekly rule and no
  override
- **THEN** the whole day is returned as a single `closed` interval

#### Scenario: A booked hour is labeled booked

- **WHEN** a mentor has a confirmed booking from 18:00 to 19:00 on a day otherwise free
- **THEN** 18:00–19:00 is labeled `booked`
- **AND** it is not also labeled `busy` or `free`

#### Scenario: A synced busy interval is labeled busy and reveals no detail

- **WHEN** a mentor's connected calendar has synced a busy interval
- **THEN** the corresponding time is labeled `busy` in the breakdown
- **AND** the response carries no title, attendee, or description for it

#### Scenario: A buffer-withheld interval is closed, not free

- **WHEN** a mentor's buffers widen a booked or busy interval into an otherwise
  available hour
- **THEN** the widened portion is labeled `closed`, not `free`

#### Scenario: The breakdown agrees with what a seeker can book

- **WHEN** a mentor's own breakdown for a window labels an interval `free`
- **THEN** a seeker requesting slots for the same mentor and window is offered a
  bookable slot covering that interval

#### Scenario: A past interval is never labeled free

- **WHEN** a requested month includes a date or time before the current instant
- **THEN** no interval before the current instant is labeled `free`

### Requirement: Only the mentor can read their own breakdown

The system SHALL require the caller to be the authenticated account owning the mentor
profile being read. A request for another account's breakdown, or an unauthenticated
request, SHALL be refused rather than answered.

#### Scenario: An unauthenticated request is refused

- **WHEN** a signed-out visitor requests a mentor's own-calendar breakdown
- **THEN** the request is refused

#### Scenario: A signed-in caller cannot read another mentor's breakdown

- **WHEN** a signed-in account without a mentor profile, or with a different mentor
  profile, requests another mentor's own-calendar breakdown
- **THEN** the request is refused
