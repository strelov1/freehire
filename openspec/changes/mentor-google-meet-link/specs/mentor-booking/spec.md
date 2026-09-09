## MODIFIED Requirements

### Requirement: A booking records where the seeker came from

A booking SHALL be able to record the vacancy the seeker was reading, and SHALL
outlive that vacancy — a pruned or closed posting SHALL clear the reference rather
than remove the booking. A booking SHALL carry a note from the seeker, the seeker's
timezone, and a snapshot of the meeting link as it stood when the booking was made.
For a mentor with a connected calendar this snapshot SHALL be the link minted for that
specific booking rather than a copy of the profile's own field, and it MAY be empty
when minting failed.

#### Scenario: A booking survives its vacancy

- **WHEN** a booking references a vacancy and that vacancy row is later deleted
- **THEN** the booking still exists with no vacancy reference

#### Scenario: Changing the meeting link does not rewrite past invitations

- **WHEN** a mentor changes their meeting link after a booking is confirmed
- **THEN** the existing booking keeps the link it was made with

#### Scenario: A booking's own minted link is unaffected by a later booking's link

- **WHEN** a mentor with a connected calendar takes two bookings in sequence
- **THEN** each booking keeps the Meet link minted for it, even if the two differ

### Requirement: Confirmation reaches both parties with a calendar invitation

On confirmation the system SHALL notify both the seeker and the mentor with the
session time expressed in each recipient's own timezone, the meeting link, and an
`.ics` invitation. Delivery SHALL be best-effort: a failure on one channel or to one
party SHALL be logged and SHALL NOT undo the booking. A booking whose meeting link is
empty SHALL still be confirmed and notified, simply without a link to show.

#### Scenario: Both parties receive an invitation in their own zone

- **WHEN** a booking is confirmed between a mentor in `Europe/Berlin` and a seeker in
  `Asia/Tokyo`
- **THEN** each is notified with the session time in their own zone
- **AND** each message carries an `.ics` invitation for the same instant

#### Scenario: A failed delivery does not undo the booking

- **WHEN** the mail transport fails while notifying the mentor
- **THEN** the booking stays confirmed
- **AND** the failure is logged

#### Scenario: A booking with no meeting link is still confirmed and notified

- **WHEN** a booking is confirmed with an empty meeting link
- **THEN** both parties are notified and the `.ics` invitation is still sent
- **AND** neither rendering treats the missing link as an error

## ADDED Requirements

### Requirement: A connected mentor's booking gets an auto-generated meeting link

When a booking is confirmed for a mentor who has connected Google Calendar for write
access, the system SHALL create a calendar event for that session — the seeker as an
attendee — and SHALL use the resulting Google Meet link as that booking's own meeting
link, in place of the mentor's static profile field. A mentor without this connection
SHALL see no change: the booking's meeting link is the profile's own field, exactly as
before.

#### Scenario: A connected mentor's booking gets a real Meet link

- **WHEN** a seeker books a session with a mentor who has connected Google Calendar for
  write access
- **THEN** the confirmed booking's meeting link is a Google Meet link minted for that
  specific session
- **AND** the seeker receives a calendar invitation to it as an attendee

#### Scenario: An unconnected mentor's booking is unaffected

- **WHEN** a seeker books a session with a mentor who has not connected Google Calendar
  for write access
- **THEN** the confirmed booking's meeting link is the mentor's own profile field,
  exactly as before this change

### Requirement: A failed meeting-link generation never blocks the booking

The system SHALL confirm a booking regardless of whether generating its meeting link
succeeded. A failure SHALL be logged, and SHALL leave the booking's meeting link empty
rather than falling back to the mentor's static profile field or refusing the booking.
A failure that indicates the mentor's calendar grant was revoked SHALL mark that grant
as needing reconsent.

#### Scenario: A booking still confirms when link generation fails

- **WHEN** a seeker books a session with a mentor whose connected calendar's grant has
  been revoked
- **THEN** the booking is still confirmed
- **AND** its meeting link is empty
- **AND** the mentor's calendar grant is marked as needing reconsent

#### Scenario: A transient Google failure does not refuse the booking

- **WHEN** the calendar-event creation call fails for a reason other than a revoked
  grant
- **THEN** the booking is still confirmed with an empty meeting link
- **AND** the mentor's calendar grant is left as it was

### Requirement: Cancelling a booking best-effort removes its calendar event

When a cancelled booking has an associated calendar event, the system SHALL attempt to
delete that event. A failure to delete it SHALL be logged and SHALL NOT prevent or
undo the cancellation.

#### Scenario: Cancelling removes the calendar event

- **WHEN** a confirmed booking that created a calendar event is cancelled
- **THEN** the system attempts to delete that calendar event

#### Scenario: A failed deletion does not block the cancellation

- **WHEN** deleting a cancelled booking's calendar event fails
- **THEN** the cancellation still succeeds
- **AND** the failure is logged
