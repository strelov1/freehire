# mentor-busy-sync Specification

## Purpose

Lets a mentor opt in to read-only Google Calendar access purely so their own existing
commitments are read as busy time and kept out of what freehire offers a seeker to book,
without ever exposing what those commitments are.

## Requirements

### Requirement: A mentor may connect Google Calendar for busy-time sync

The system SHALL let a signed-in mentor grant `calendar.readonly` through Google's
incremental authorization, layered on the existing Google OAuth client the way the
platform's other incremental grants are. This SHALL be its own consent, requested
separately from — and never inferred from — any other Google grant the same account may
already hold, including a read-only calendar grant made for an unrelated feature. On
success the system MUST store the resulting refresh token, encrypted at rest, and mark
the grant as covering `calendar.readonly` for this purpose.

#### Scenario: A mentor connects Calendar for busy-time sync

- **WHEN** a signed-in mentor starts this connect flow and grants `calendar.readonly`
- **THEN** the callback exchanges the code, stores the refresh token encrypted, and the
  grant is recorded as covering busy-time sync

#### Scenario: An unrelated Google connection does not imply this grant

- **WHEN** an account has previously connected Gmail, read-only calendar access for
  another feature, or the mentor-calendar write grant, but has never completed this
  connect flow
- **THEN** the system does not treat that account as having opted in to busy-time sync

### Requirement: A connected mentor's busy time is synced periodically

For each mentor whose grant covers busy-time sync, the system SHALL periodically read
their primary Google Calendar's free/busy state over a bounded future window and
reconcile the result into their stored busy intervals: an interval currently busy that
is not yet stored MUST be added, one already stored that is no longer busy MUST be
removed, and one that changed bounds MUST be updated. The system SHALL store only the
start and end of each interval — never a title, attendee, description, or any other
detail of what occupies the time.

#### Scenario: A newly busy interval is added

- **WHEN** a connected mentor's calendar shows a busy interval that is not yet stored
- **THEN** the system stores it, bounds only

#### Scenario: A cleared interval is removed

- **WHEN** a previously synced busy interval is no longer busy on the mentor's calendar
- **THEN** the system removes the stored interval

#### Scenario: No detail of the underlying event is ever stored

- **WHEN** the system reads a mentor's free/busy state
- **THEN** whatever is persisted carries no title, attendee, description, or link

### Requirement: One mentor's sync failure does not affect another's

A sync run SHALL treat each connected mentor independently: a failure reading or
reconciling one mentor's calendar SHALL NOT prevent the run from continuing to the
remaining mentors, and SHALL be reported rather than silently swallowed.

#### Scenario: One failing mentor does not stop the run

- **WHEN** a sync run processes several connected mentors and one call to Google fails
  for a reason other than a revoked grant
- **THEN** the remaining mentors are still synced
- **AND** the run reports that mentor's failure

### Requirement: A revoked grant is marked for reconsent, not silently retried forever

The system SHALL detect a revocation-shaped failure from Google the same way it already
does for its other Google grants, and SHALL mark the connection as needing reconsent
rather than leaving it looking healthy while every sync attempt quietly fails.

#### Scenario: A revoked grant is marked for reconsent

- **WHEN** a sync attempt using a mentor's stored busy-sync refresh token fails in a way
  that indicates the grant was revoked
- **THEN** the connection is marked as needing reconsent

#### Scenario: A revocation is not undone by an unrelated reconnect

- **WHEN** a mentor's grant is marked needing reconsent, and the account later completes
  an unrelated Google connect flow that happens to restore the shared grant to a healthy
  status while still covering `calendar.readonly`
- **THEN** the account is not treated as having opted in to busy-time sync
- **AND** syncing resumes only once the mentor completes this feature's own connect flow
  again

### Requirement: Disconnecting stops future syncs without erasing sync history

A mentor whose grant is disconnected or marked needing reconsent SHALL NOT be synced
again until reconnected. The busy intervals already stored from prior syncs SHALL NOT be
retroactively cleared by the disconnection itself — the slot engine keeps using the last
known state until it is either confirmed stale by a future sync or the mentor's own
availability changes.

#### Scenario: A disconnected mentor is skipped by the next run

- **WHEN** a mentor's busy-sync grant is disconnected or needs reconsent
- **THEN** the next sync run does not attempt to read their calendar

#### Scenario: Existing intervals survive a disconnection

- **WHEN** a mentor with previously synced busy intervals disconnects the grant
- **THEN** those stored intervals are not removed as a result of disconnecting
