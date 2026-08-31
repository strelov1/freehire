## MODIFIED Requirements

### Requirement: Authenticated board contribution

The system SHALL accept a board contribution only from an authenticated user, identified by
session cookie or API key, and SHALL attribute every recorded board to that user.

#### Scenario: Anonymous request is rejected

- **WHEN** an unauthenticated caller posts a link to the contribution endpoint
- **THEN** the system responds 401 and records nothing

#### Scenario: Authenticated request is attributed

- **WHEN** an authenticated user submits a link that passes all checks
- **THEN** the recorded board is owned by that user

### Requirement: Reject a board already in the catalogue

The system SHALL NOT record a contribution for a board it already crawls, because the board
needs no onboarding. The submitted link SHALL still be served by the intake sequence — the
vacancy is imported if it can be read — and the caller SHALL be told which company is already
tracked.

#### Scenario: A board we already crawl is rejected

- **WHEN** a user submits a link for a board that already has jobs in the catalogue
- **THEN** no contribution row is recorded and the answer names the company already being
  crawled

#### Scenario: A vacancy on a crawled board is still imported

- **WHEN** the submitted link is a readable vacancy on an already-crawled board that the
  catalogue does not yet carry
- **THEN** the vacancy is imported and its posting is returned to the caller

### Requirement: Reject a board already contributed

The system SHALL reject a contribution whose board was already recorded (by any user), with a
distinct "board already contributed" error, and SHALL NOT record a second row. The board — not
the vacancy — is the uniqueness key, so any second link to the same company collides.

#### Scenario: A second vacancy on the same board is rejected

- **WHEN** a user submits a link whose board matches an existing contribution
- **THEN** the system responds 409 with a "board already contributed" error

#### Scenario: Concurrent duplicate submissions record at most one

- **WHEN** two requests for the same new board race
- **THEN** exactly one board is recorded; the other receives the "board already contributed" error

### Requirement: Recording a novel board

For a supported, non-duplicate board, the system SHALL record a contribution row — owner,
canonical URL, source, and board slug. Recording SHALL carry no reward, because there is no
longer a currency to award: contributing is rewarded again in the `add-invites` change, as
days of Pro. The system SHALL NOT maintain any separate per-user "points" counter.

#### Scenario: Novel board is recorded

- **WHEN** a user submits a supported link for a board we neither crawl nor already hold
- **THEN** a contribution row is recorded and owned by that user

#### Scenario: Recording is idempotent per board

- **WHEN** the same novel board is submitted twice
- **THEN** exactly one contribution row exists for it

## RENAMED Requirements

- FROM: `### Requirement: Recording a novel board and awarding AI credits`
- TO: `### Requirement: Recording a novel board`
