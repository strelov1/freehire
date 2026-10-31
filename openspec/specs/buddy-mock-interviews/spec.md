# buddy-mock-interviews Specification

## Purpose
Peer mock interviews inside the interview-buddy network: a member publishes the hours they
can host a 60-minute mock, another member books one, both are reminded, and each may leave
a review that is revealed once both have written theirs or the window closes.

## Requirements
### Requirement: Mock settings
An active buddy member SHALL be able to save mock settings: an IANA time zone, a meeting
link starting with `https://`, a non-empty set of topics from `coding`, `system_design`,
`behavioral`, and a pause switch. A member without saved settings, or with the switch
paused, SHALL offer no slots. The settings form SHALL warn that the guest may see the
host's name through the meeting link.

#### Scenario: Settings without a zone are refused
- **WHEN** a member saves settings with an unknown time zone
- **THEN** the system responds with a validation error and stores nothing

#### Scenario: Non-https link is refused
- **WHEN** a member saves a meeting link starting with `http://`
- **THEN** the system responds with a validation error

#### Scenario: Paused member offers nothing
- **WHEN** a member with availability pauses mocks
- **THEN** their slot listing is empty and already booked mocks stay confirmed

### Requirement: Mock availability
A member SHALL publish availability as weekly ranges and dated overrides in their own time
zone, using the same rules as mentor availability: an empty dated override closes its
date, and every time resolves through the member's zone on each date.

#### Scenario: Weekly range across a DST change
- **WHEN** a member in `Europe/Berlin` offers Tuesdays 19:00-21:00 and a viewer lists slots
  across the October clock change
- **THEN** every offered slot starts at 19:00 or 20:00 Berlin wall-clock time on both sides
  of the change

#### Scenario: Closed date
- **WHEN** a member adds an empty override for a date
- **THEN** no slot is offered on that date

### Requirement: Slot listing
An active member SHALL be able to list another active member's open 60-minute slots in the
viewer's own time zone, between 12 hours and 30 days from now. A slot SHALL NOT be offered
while the host is already in a confirmed mock that overlaps it, as host or as guest. A
non-member, or a member blocked by or blocking the host, SHALL get no slots.

#### Scenario: Busy as guest hides the host's slot
- **WHEN** member H is a guest in a mock at 19:00 and offers availability at 19:00
- **THEN** H's 19:00 slot is not listed

#### Scenario: Blocked viewer
- **WHEN** the viewer has blocked the host, or the host has blocked the viewer
- **THEN** the listing responds 404

### Requirement: Booking a mock
An active member SHALL be able to book an offered slot of another active member, choosing
one of the host's topics. The system SHALL re-derive the slot at booking time and refuse
with one "slot unavailable" error when it is no longer offered or was taken concurrently.
A person SHALL never be in two overlapping confirmed mocks, as host or as guest, and the
database SHALL enforce this. A member SHALL NOT book themselves, and SHALL hold at most 3
future confirmed mocks as guest.

#### Scenario: Concurrent bookings of one slot
- **WHEN** two members book the same host slot at the same time
- **THEN** exactly one mock is confirmed and the other request gets "slot unavailable"

#### Scenario: Guest already busy
- **WHEN** a guest who hosts a mock at 19:00 books another member's 19:00 slot
- **THEN** the booking is refused with "slot unavailable"

#### Scenario: Topic not offered
- **WHEN** the guest picks a topic the host does not offer
- **THEN** the booking is refused with a validation error

#### Scenario: Guest limit
- **WHEN** a member with 3 future mocks as guest books a fourth
- **THEN** the booking is refused with a limit error

### Requirement: Meeting link visibility
The host's current meeting link SHALL be returned only to the two participants of a
confirmed mock. It SHALL NOT appear in slot listings, member pages or notifications to
anyone else.

#### Scenario: Link after booking
- **WHEN** a guest books a mock and the host later changes their link
- **THEN** the guest's mock shows the host's new link

#### Scenario: Link not exposed before booking
- **WHEN** a member lists another member's slots
- **THEN** the response carries no meeting link

### Requirement: Cancellation
Either participant SHALL be able to cancel a confirmed mock before it starts. Leaving the
buddy network SHALL cancel the member's future mocks, and a block SHALL cancel the pair's
future mocks, in the same transaction as the leave or the block. A cancelled mock SHALL
free both participants' time.

#### Scenario: Guest cancels
- **WHEN** the guest cancels a future mock
- **THEN** the mock is cancelled, the host is notified, and the slot is offered again

#### Scenario: Started mock cannot be cancelled
- **WHEN** a participant cancels a mock after its start
- **THEN** the system refuses with a validation error

#### Scenario: Block cancels
- **WHEN** one participant blocks the other
- **THEN** every future mock between them is cancelled

### Requirement: Reminders
The system SHALL remind both participants of a confirmed mock 24 hours and 1 hour before
it starts, each reminder at most once. A reminder SHALL be claimed before sending and the
claim released when delivery fails. A mock already started SHALL NOT be reminded about.

#### Scenario: Failed delivery retries
- **WHEN** sending a 1-hour reminder fails
- **THEN** the claim is released and the next run sends it

#### Scenario: Booked inside the window
- **WHEN** a mock is booked 13 hours before its start
- **THEN** no 24-hour reminder is sent and the 1-hour reminder is

### Requirement: Blind two-way review
Each participant of a confirmed mock SHALL be able to rate the other 1-5 with an optional
comment of at most 1000 characters, once, from the mock's end until 7 days after it. A
review SHALL be visible to its subject and on the subject's member page only after the
counterpart has reviewed too or the 7-day window has closed. A member page SHALL show the
count of completed mocks, the average of revealed ratings and the most recent revealed
reviews.

#### Scenario: Hidden until both reviewed
- **WHEN** the guest has reviewed the host and the host has not reviewed the guest
- **THEN** the host cannot see the guest's review and it is not on the host's page

#### Scenario: Revealed after both
- **WHEN** both participants have reviewed
- **THEN** each sees the review about them and both appear on the member pages

#### Scenario: Revealed after the window
- **WHEN** only one participant reviewed and 7 days have passed since the mock ended
- **THEN** that review becomes visible

#### Scenario: Review before the end
- **WHEN** a participant reviews a mock that has not ended
- **THEN** the system refuses with a validation error

### Requirement: Anonymous notifications
Booking, cancellation and reminder notifications SHALL name the other participant by buddy
handle only and SHALL carry no account name, email or message text. Times SHALL be shown
in the recipient's own zone, falling back to UTC with the zone stated.

#### Scenario: Booking email
- **WHEN** a mock is booked
- **THEN** the host's email names the guest's handle, the topic and the time in the host's
  zone, and contains no email address of the guest

