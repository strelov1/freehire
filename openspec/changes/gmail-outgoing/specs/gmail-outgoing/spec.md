## ADDED Requirements

### Requirement: Capture outgoing hiring messages
The system SHALL store sent Gmail messages only in known hiring threads, with direction and recipient headers, using existing read-only authorization.

#### Scenario: Reply from an alias
- **WHEN** a known hiring thread contains a message labelled SENT from a different address
- **THEN** it is stored as outgoing with its To and Cc headers

#### Scenario: Unrelated outgoing message
- **WHEN** a sent message belongs to an unknown thread
- **THEN** its body is not fetched or stored

### Requirement: Discover outgoing updates independently
The system SHALL backfill known threads once and discover new sent replies without a new incoming anchor. Failed passes MUST preserve retry coverage and repeated passes MUST NOT duplicate messages.

#### Scenario: Reply to an old thread
- **WHEN** a candidate sends a reply in a stored thread after its incoming synchronization watermark
- **THEN** the next successful pass stores the reply even with no new incoming mail

#### Scenario: Transient thread failure
- **WHEN** a thread or message cannot be fetched
- **THEN** the pass reports failure and retains its previous cursors for retry

### Requirement: Separate candidate mail from employer decisions
Outgoing messages MUST NOT be classified as employer signals, count as employer replies, advance stages, or increase unread counts. Inbox readers SHALL expose their direction and recipients.

#### Scenario: Candidate confirms interview
- **WHEN** the candidate sends an interview confirmation
- **THEN** it is displayed as sent and does not enqueue classification or create an employer event
