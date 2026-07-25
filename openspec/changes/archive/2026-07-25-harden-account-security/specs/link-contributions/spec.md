## ADDED Requirements

### Requirement: Contribution submissions are rate-limited per user

The system SHALL bound how often one user can submit board contributions, because an
unrecognized link makes the server fetch an attacker-chosen URL — so an unbounded endpoint is
an outbound-fetch amplifier and a timing oracle for public hosts.

- The limit SHALL be keyed on the authenticated user, not the client IP, so rotating IPs does
  not lift it.
- A caller over the limit SHALL receive `429` and no fetch SHALL be performed.
- The limit SHALL apply to the HTTP endpoint and SHALL NOT change the Telegram contribution
  path's own behaviour.

#### Scenario: Submissions within the limit are served

- **WHEN** an authenticated user submits fewer contributions than the limit within the window
- **THEN** every submission is processed as it is today

#### Scenario: Over the limit is refused before any fetch

- **WHEN** an authenticated user exceeds the limit within the window
- **THEN** the system responds `429`, performs no outbound fetch, and records nothing

#### Scenario: The limit is per user, not per address

- **WHEN** the same user submits from several client IP addresses within one window
- **THEN** all their submissions count against one shared limit
