## Purpose

Lets a mentor opt in to Google Calendar write access, purely so a real, working video
link can be minted for each of their sessions automatically — a separate consent from
the read-only Gmail/calendar access this platform already offers a candidate.

## ADDED Requirements

### Requirement: A mentor may connect Google Calendar for write access

The system SHALL let a signed-in mentor grant `calendar.events` through Google's
incremental authorization, layered on the existing Google OAuth client the way the
platform's other incremental grants are. This SHALL be its own consent, requested
separately from — and never inferred from — any read-only Gmail or calendar access the
same account may already have granted for an unrelated purpose. On success the system
MUST store the resulting refresh token, encrypted at rest, and mark the grant as
covering `calendar.events`.

#### Scenario: A mentor connects Calendar for Meet links

- **WHEN** a signed-in mentor starts this connect flow and grants `calendar.events`
- **THEN** the callback exchanges the code, stores the refresh token encrypted, and the
  grant is recorded as covering `calendar.events`

#### Scenario: An unrelated Google connection does not imply this grant

- **WHEN** an account has previously connected Gmail or read-only calendar access for
  another feature, but has never completed this connect flow
- **THEN** the system does not treat that account as having `calendar.events`

### Requirement: The write grant is never inferred as a side effect of a read grant

Broadening an existing read-only calendar grant to include write access, other than
through this connect flow's own explicit consent screen, SHALL NOT occur. A user who
granted read-only calendar access for one purpose SHALL NOT thereby hand write access
to a different feature they never agreed to.

#### Scenario: Connecting read-only calendar access elsewhere does not grant write access here

- **WHEN** an account completes an unrelated read-only calendar connect flow
- **THEN** this platform does not treat the account as having `calendar.events`
  write access

### Requirement: A revoked or failing grant is marked for reconsent, not silently retried forever

The system SHALL detect a revocation-shaped failure from Google the same way it already
does for its other Google grants, and SHALL mark the connection as needing reconsent
rather than leaving it looking healthy while every use of it quietly fails.

#### Scenario: A revoked grant is marked for reconsent

- **WHEN** a call to Google using a mentor's stored `calendar.events` refresh token
  fails in a way that indicates the grant was revoked
- **THEN** the connection is marked as needing reconsent
