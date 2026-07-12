# gmail-ats-sync Specification

## Purpose
TBD - created by archiving change gmail-inbox. Update Purpose after archive.
## Requirements
### Requirement: ATS-scoped Gmail sync

The system SHALL, for each connected user, read their mail via the Gmail API
restricted to a curated ATS sender-domain list, and MUST NOT ingest non-ATS mail.

#### Scenario: Only ATS mail ingested

- **WHEN** the sync worker runs for a connected user
- **THEN** it queries the Gmail API for mail from the configured ATS domains and stores only those messages

#### Scenario: Non-ATS mail ignored

- **WHEN** a user's mailbox contains mail from non-ATS senders
- **THEN** that mail is never fetched or stored

### Requirement: Full-message persistence

The worker SHALL upsert each ATS message with its headers and full text and HTML
bodies into the `emails` store, keyed uniquely by the Gmail message id so a
re-sync never duplicates.

#### Scenario: New message stored

- **WHEN** the worker fetches an ATS message not yet stored
- **THEN** it inserts an `emails` row with from/subject/received time and the text and HTML bodies

#### Scenario: Re-sync is idempotent

- **WHEN** the worker re-fetches a message already stored
- **THEN** no duplicate row is created (dedup on the Gmail message id)

### Requirement: Incremental sync cursor

The system SHALL track a per-user sync cursor so a subsequent run fetches only
new mail, and MUST recover safely when the cursor is stale or absent.

#### Scenario: Incremental run

- **WHEN** the worker runs after a previous sync
- **THEN** it fetches only mail newer than the stored cursor and advances the cursor

#### Scenario: First run backfills

- **WHEN** the worker runs for a newly connected user with no cursor
- **THEN** it backfills the user's ATS mail history and sets the cursor

### Requirement: Resilient to token and quota failures

The worker SHALL be best-effort per user: an expired/revoked token or a Gmail
API quota error MUST be logged and skip that user without aborting the run.

#### Scenario: Revoked token

- **WHEN** a user's refresh token no longer authorizes access
- **THEN** the worker marks that connection as needing re-consent and continues with other users

