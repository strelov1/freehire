# mentor-booking Specification

## Purpose

The life of one session: taking an offered hour, the guarantee that two of them can never
overlap, the confirmation both parties receive with a calendar invitation, cancellation by
either side, the reminders before it, and the single review a seeker may leave afterwards.
The non-overlap guarantee is a database constraint rather than an application re-check,
because a check-then-act race is exactly how one mentor ends up with two people in an hour.
## Requirements
### Requirement: Only an authenticated user may book; anyone may look

The slot listing SHALL be readable without authentication. Creating a booking SHALL
require an authenticated account. A mentor SHALL NOT be able to book their own
sessions.

#### Scenario: An anonymous visitor is invited to sign in

- **WHEN** a signed-out visitor submits a booking
- **THEN** the system refuses with an unauthenticated error
- **AND** no booking row is written

#### Scenario: A mentor cannot book themselves

- **WHEN** a mentor submits a booking for their own profile
- **THEN** the system refuses with a validation error

### Requirement: A booking is refused unless its slot is still offerable

The system SHALL re-derive the requested slot at write time and SHALL refuse a booking
whose start is not an offerable slot — because availability changed, the notice period
has since elapsed, the horizon has passed, the mentor is paused, or the time is no
longer free. The refusal SHALL name the reason.

#### Scenario: A stale page cannot book a withdrawn slot

- **WHEN** a seeker loads slots, the mentor then removes that day's availability, and
  the seeker submits the booking
- **THEN** the system refuses and reports that the slot is no longer available
- **AND** no booking row is written

#### Scenario: A paused mentor takes no bookings

- **WHEN** a seeker submits a booking for a mentor who has since paused
- **THEN** the system refuses

### Requirement: Two confirmed bookings for one mentor can never overlap

The non-overlap of a mentor's confirmed bookings SHALL be guaranteed by a database
constraint over the booking interval, not by an application re-check alone. Two
concurrent requests for the same slot SHALL result in exactly one confirmed booking;
the loser SHALL receive the same "no longer available" outcome as any stale request,
never a server error.

#### Scenario: Concurrent bookings for one slot leave exactly one winner

- **WHEN** two seekers submit a booking for the same mentor and start instant at the
  same moment
- **THEN** exactly one booking is confirmed
- **AND** the other request is refused as unavailable

#### Scenario: A cancelled booking does not block its own time

- **WHEN** a booking at 19:00 is cancelled and another seeker books 19:00
- **THEN** the new booking is confirmed

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

### Requirement: A booking is readable only by its two parties

Every read of one booking — its details, its meeting link, its note — SHALL be authorised
against the caller: the seeker who made it or the mentor it was made with, and nobody
else. A caller who is neither SHALL receive the same answer as one naming a booking that
does not exist.

This is stated separately from the identifier below because they are different defences
and neither substitutes for the other: a random identifier makes bookings unenumerable,
and authorisation makes a leaked or guessed one useless.

#### Scenario: A stranger holding a valid identifier is refused

- **WHEN** an authenticated account that is neither party requests a booking by its
  identifier
- **THEN** the request is refused as not found
- **AND** the response reveals nothing about whether that booking exists

#### Scenario: Either party may read their own session

- **WHEN** the seeker, and separately the mentor, request the booking
- **THEN** each receives it, including the meeting link

### Requirement: A booking identifier is not guessable

A booking SHALL be identified by a random identifier rather than a sequential one,
because a booking is read by two different accounts and a countable identifier would
make any single authorisation slip enumerable.

#### Scenario: Identifiers reveal no ordering

- **WHEN** two bookings are created in sequence
- **THEN** neither identifier can be derived from the other

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

### Requirement: Either party may cancel before the session starts

Both the seeker and the mentor SHALL be able to cancel a confirmed booking at any time
before its start, with an optional reason. Cancellation SHALL record who cancelled and
when, SHALL notify the other party, and SHALL free the time immediately. A booking
that has already started or been cancelled SHALL NOT be cancellable again.

#### Scenario: A cancellation frees the slot and tells the other side

- **WHEN** a mentor cancels a confirmed booking
- **THEN** the booking's status records that the mentor cancelled, with the time
- **AND** the seeker is notified
- **AND** the slot is offerable again

#### Scenario: A past booking cannot be cancelled

- **WHEN** either party attempts to cancel a booking whose start has passed
- **THEN** the system refuses and the booking's status is unchanged

#### Scenario: A stranger cannot cancel

- **WHEN** an account that is neither the mentor nor the seeker attempts to cancel
- **THEN** the system refuses, and its response does not reveal whether the booking exists

### Requirement: Reminders fire before the session and only for live bookings

The system SHALL remind both parties 24 hours and 1 hour before a confirmed session.
Each reminder SHALL be sent at most once per booking per offset, so that repeating the
worker sends nothing twice. A cancelled booking SHALL receive no further reminders.

Each offset SHALL fire only within a bounded window ending at that offset. A session
booked closer than an offset SHALL receive no reminder for it: there was never that much
notice to give, and sending one anyway would tell somebody their session starts "in 24
hours" when it starts in three.

A reminder whose DELIVERY fails SHALL be retried on a later run rather than counted as
sent. This weakens "at most once" to "at most once per successful delivery, and at least
one attempt per run until one succeeds" — chosen because the two failures are not equal:
a missing reminder costs somebody the session, a duplicate costs them a duplicate.

#### Scenario: A session booked inside an offset gets no reminder for that offset

- **WHEN** a session three hours away is examined for the 24-hour reminder
- **THEN** no 24-hour reminder is sent
- **AND** it still receives its 1-hour reminder when that window arrives

#### Scenario: A failed delivery is retried

- **WHEN** a reminder is claimed and its delivery fails
- **THEN** the claim is released
- **AND** a later run sends it, and does not send it again afterwards

#### Scenario: Re-running the reminder worker sends nothing twice

- **WHEN** the reminder worker runs, sends the 24-hour reminder, and then runs again
  before the next offset is due
- **THEN** no second message is sent for that booking and offset

#### Scenario: Cancelling stops pending reminders

- **WHEN** a booking with an unsent 1-hour reminder is cancelled
- **THEN** that reminder is never sent

#### Scenario: A booking whose window was missed does not fire late

- **WHEN** the worker does not run until after a session has started
- **THEN** no reminder is sent for that session

### Requirement: Booking reminders and confirmations are transactional

Booking confirmations, cancellations and reminders SHALL be delivered regardless of
the account-level notification rule, because they concern a commitment the recipient
made themselves and a second person is holding time for it.

#### Scenario: A user who silenced notifications still hears about their own booking

- **WHEN** a user with their notification rule disabled books a session
- **THEN** they receive the confirmation, the calendar invitation and the reminders

### Requirement: A session becomes completed and may be reviewed once

A confirmed booking whose end has passed and which was not cancelled SHALL be treated
as completed. The seeker SHALL be able to leave one rating and comment per completed
booking, editable afterwards but never duplicated. The mentor's public profile SHALL
show the aggregate rating and the count it rests on.

#### Scenario: Only the seeker of a completed session may review it

- **WHEN** an account that is not that booking's seeker submits a review for it
- **THEN** the system refuses

#### Scenario: A cancelled session cannot be reviewed

- **WHEN** the seeker of a cancelled booking submits a review
- **THEN** the system refuses

#### Scenario: A second review replaces the first rather than adding one

- **WHEN** a seeker who has already reviewed a completed booking submits again
- **THEN** the stored review is updated
- **AND** the mentor's review count is unchanged

### Requirement: Each party sees their own sessions

The system SHALL give a seeker a list of the sessions they booked and a mentor a list
of the sessions booked with them, each split into upcoming and past, and each readable
only by that party.

The split SHALL be made against ONE clock for the whole list, so a session starting
between two reads cannot appear in both halves or in neither. A cancelled session SHALL
be past whatever its start time says: nobody is going to it.

#### Scenario: A seeker's list holds only their own bookings

- **WHEN** a signed-in seeker lists their sessions
- **THEN** every entry is a booking they made

#### Scenario: A cancelled future session is past, not upcoming

- **WHEN** a party lists their sessions and one confirmed future session has been
  cancelled
- **THEN** it appears under past
- **AND** the still-confirmed future sessions appear under upcoming

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

