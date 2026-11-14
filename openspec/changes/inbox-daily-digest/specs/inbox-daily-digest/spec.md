## ADDED Requirements

### Requirement: Explicit daily inbox consent
The system SHALL default inbox digests off and SHALL respect global notification settings, email-channel choice, activity consent and recipient eligibility. Omitted new fields from older clients MUST preserve the current choice.

#### Scenario: Enable digest
- **WHEN** a candidate enables daily inbox mail
- **THEN** only new incoming mail received after activation becomes eligible at the chosen daily time in the account timezone

### Requirement: Useful conversation summaries
The digest SHALL group new classified hiring mail into outreach, requests and application updates, display at most ten conversations with overflow counts and source links, and exclude outgoing, other, deleted, stale and already delivered messages.

#### Scenario: Candidate has replied
- **WHEN** a selected conversation has a later stored outgoing message
- **THEN** the digest indicates that a later reply is visible without asserting that all requests are resolved

#### Scenario: Empty day
- **WHEN** no eligible messages exist
- **THEN** no email or daily delivery claim is created

### Requirement: Claim before dispatch
The existing notify worker SHALL send inbox digests independently of search availability, claiming each user's local day before sending. Concurrent runs MUST NOT both dispatch that day's digest. Only confirmed deliveries MUST retire messages.

#### Scenario: Repeated pass
- **WHEN** a second pass encounters the same user's claimed local day
- **THEN** it sends no second digest

#### Scenario: Ambiguous send failure
- **WHEN** a dispatch outcome is uncertain
- **THEN** the day stays claimed, delivery reports failure, and unconfirmed messages remain eligible on a later day

### Requirement: Summaries cannot feed themselves
Generated inbox digests MUST carry an explicit product marker and MUST be excluded from both Gmail and hosted ingestion. Ordinary automated employer messages MUST remain eligible.

#### Scenario: Summary arrives in Gmail
- **WHEN** a marked summary is found by the hiring-shaped Gmail query
- **THEN** it is not stored or reclassified as an employer message

#### Scenario: Delayed classification
- **WHEN** an old request is classified after a newer message in its conversation has already been delivered
- **THEN** the old request does not reappear as a new action in a later digest
