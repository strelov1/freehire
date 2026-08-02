## ADDED Requirements

### Requirement: The calendar grant is asked for separately

Reading the candidate's calendar SHALL require its own consent, distinct from the Gmail
connection. Connecting a mailbox SHALL NOT request calendar access, and a caller with a
mailbox connected SHALL be able to decline the calendar without losing the mailbox.

The grant SHALL be incremental, so a candidate who accepts both holds one Google grant
covering both scopes, and the connection row SHALL record which scopes it carries. The
worker SHALL read that record rather than assume: a token minted before the calendar
scope existed cannot call the calendar API, and discovering this by a 403 per user per
run is a worse answer than not asking.

Where the OAuth application is not yet verified by Google, the connect surface SHALL say
so plainly. A restricted-scope application in testing admits a fixed roster of accounts,
and a consent screen that simply refuses an unlisted candidate reads as a fault in
freehire.

#### Scenario: Connecting a mailbox asks for no calendar

- **WHEN** a candidate runs the Gmail connect flow
- **THEN** the consent screen requests mail access only

#### Scenario: Declining the calendar leaves the mailbox connected

- **WHEN** a candidate with a connected mailbox declines the calendar consent
- **THEN** the mailbox stays connected and mail continues to sync

#### Scenario: A pre-calendar token is not used against the calendar

- **WHEN** the sync worker meets a connection whose recorded scopes do not include the
  calendar
- **THEN** it skips that user without calling the calendar API

### Requirement: Only a meeting attached to an application is stored

The sync SHALL read its window into memory, attempt to attach each event to one of the
caller's applications, and persist a row only for those it attached. An event it could
not attach SHALL NOT be written to any table, log, or audit trail.

A calendar carries a person's medical appointments, their family, their current
employer's meetings, and interviews with employers they never told us about. The ledger
already holds this line for mail — content-free by construction — and a calendar is the
more invasive of the two. Storing only what matched makes the boundary structural: the
personal event is not in the database to leak, whatever a later query does.

The stored row SHALL carry what the candidate needs to keep the appointment — when it
starts and ends, what it is called, and how to join it — and nothing about any other
event.

#### Scenario: A personal event is not persisted

- **WHEN** the window contains a dentist appointment and an interview
- **THEN** only the interview is written, and no record of the appointment exists
  afterwards

#### Scenario: An interview with an employer the caller never tracked is not persisted

- **WHEN** the window contains a meeting that names a company with no application
- **THEN** nothing is written for it

### Requirement: Only the invitation's own identifier links a meeting automatically

A meeting SHALL be linked to an application automatically only where its `iCalUID`
equals the `UID` of the `text/calendar` part of an email that is already linked to that
application. Any other correspondence — a company name in the title, an organiser's
domain — SHALL produce a suggestion the candidate confirms, never a link.

The UID is the meeting's own identity, carried by both the invitation and the calendar
entry, so the chain closes without inference: the mail is already linked by the
deterministic matcher, and the UID says the calendar entry is that same meeting.

Matching on the organiser's domain is the trap `mailmatch` already names in its own
terms. An ATS schedules from its own domain as readily as a recruiter schedules from the
employer's, so the domain is evidence about who sent the invitation and not about who is
interviewing.

A UID SHALL only link within one caller's own data: an event whose UID matches an email
belonging to another user SHALL NOT be linked.

#### Scenario: The invitation's UID links the meeting

- **WHEN** a calendar event's `iCalUID` matches the UID of an invitation already linked
  to an application
- **THEN** the meeting is attached to that application without asking

#### Scenario: A matching company name only suggests

- **WHEN** an event titled with a tracked employer's name carries no UID we hold
- **THEN** it is offered as a suggestion and is not linked until the candidate confirms

#### Scenario: A UID does not cross between accounts

- **WHEN** an event's UID matches an email belonging to a different user
- **THEN** the meeting is not linked

### Requirement: A meeting's row is mutable, the ledger's record of it is not

The stored meeting SHALL be updated in place when it moves and marked when it is
cancelled, keyed by `(user, iCalUID)` so a re-sync is idempotent.

The ledger SHALL record `interview_scheduled` once, dated by when the scheduling was
observed rather than by when the meeting is. `application_events.occurred_at` means the
moment a thing happened and every day calculation reads it; a row dated in the future
would make the ledger a schedule, and a rescheduled meeting would need an append-only
table to change its mind.

#### Scenario: A rescheduled meeting moves in place

- **WHEN** an interview is moved to another day and the sync runs again
- **THEN** the stored meeting carries the new time, one row still exists for it, and the
  ledger holds exactly one `interview_scheduled`

#### Scenario: A cancelled meeting is marked, not deleted

- **WHEN** an interview is cancelled in the candidate's calendar
- **THEN** the stored meeting is marked cancelled and the `interview_scheduled` event
  stands — the scheduling happened

#### Scenario: The ledger is dated by the observation, not by the meeting

- **WHEN** a meeting three weeks away is first seen
- **THEN** the `interview_scheduled` event is dated at the observation, so no ledger row
  sits in the future

### Requirement: A failing user does not stop the run

The worker SHALL process each connected candidate independently. A token that no longer
works SHALL mark that connection as needing consent again and the run SHALL continue.

#### Scenario: A revoked grant marks one connection and continues

- **WHEN** one candidate's refresh token is rejected and others are healthy
- **THEN** that connection is marked as needing consent, the remaining candidates sync,
  and the run reports failure through its exit code
