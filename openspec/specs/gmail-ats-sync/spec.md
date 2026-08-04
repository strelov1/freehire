# gmail-ats-sync Specification

## Purpose
TBD - created by archiving change gmail-inbox. Update Purpose after archive.
## Requirements
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

### Requirement: Hiring-shaped Gmail sync

The system SHALL, for each connected user, read their mail via the Gmail API restricted to
mail that is hiring-shaped — sent from a curated ATS sender domain, OR carrying one of the
recognised application and interview phrasings — and MUST NOT ingest mail matching neither.

The phrasings SHALL cover the wordings employers actually use rather than one canonical
form of each. Measured against a live mailbox, the sync fetched 431 messages over 120 days
where a hiring-shaped query found 1151: the misses were near misses, an acknowledgement
reading "we've received your … application" where the list knew only "your application at",
and an invitation reading "interview invite" where it knew only "invite you to interview".

The sync SHALL NOT ingest messages the connected account itself sent, and SHOULD NOT fetch
them. The storage guard already exists; the fetch-side exclusion is what stops those
messages consuming the query's results and a body retrieval each.

#### Scenario: Mail from an ATS sender is ingested

- **WHEN** the sync worker runs and the mailbox holds mail from a configured ATS domain
- **THEN** that mail is fetched and stored

#### Scenario: Mail phrased as an application or an interview is ingested

- **WHEN** the mailbox holds a message from a domain on no list whose text carries a
  recognised application or interview phrasing
- **THEN** that mail is fetched and stored

#### Scenario: Mail that is neither is ignored

- **WHEN** the mailbox holds mail from an unrecognised sender with no recognised phrasing
- **THEN** that mail is never fetched or stored

#### Scenario: The candidate's own mail is neither fetched nor stored

- **WHEN** the mailbox holds a message the connected account sent
- **THEN** the query does not ask for it
- **AND** it is not stored, whether or not its text is hiring-shaped

