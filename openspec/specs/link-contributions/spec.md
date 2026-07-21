# link-contributions Specification

## Purpose
TBD - created by archiving change add-link-contributions. Update Purpose after archive.
## Requirements
### Requirement: Authenticated board contribution

The system SHALL accept a board contribution only from an authenticated user, identified by
session cookie or API key, and SHALL attribute every recorded board and awarded AI-credits
reward to that user.

#### Scenario: Anonymous request is rejected

- **WHEN** an unauthenticated caller posts a link to the contribution endpoint
- **THEN** the system responds 401 and records nothing

#### Scenario: Authenticated request is attributed

- **WHEN** an authenticated user submits a link that passes all checks
- **THEN** the recorded board is owned by that user and the AI-credits reward is credited to that user

### Requirement: Supported-ATS board recognition

The system SHALL accept a link only when its host belongs to a supported multi-tenant ATS and
the URL yields a board slug, determined without any network request. A link from an unknown
host, a single-tenant source, or a URL with no board segment SHALL be rejected with a distinct
"unsupported ATS" error and SHALL NOT be recorded. Both a vacancy URL and a bare board-listing
URL for the same company SHALL yield the same board.

#### Scenario: Unsupported host is rejected

- **WHEN** a user submits `https://example.com/careers/123`
- **THEN** the system responds 422 with an "unsupported ATS" error and records nothing

#### Scenario: Single-tenant source is rejected

- **WHEN** a user submits a single-tenant aggregator link (e.g. `https://geekjob.ru/vacancy/6a1e`)
- **THEN** the system responds 422 — it is not a per-company board

#### Scenario: Vacancy URL and board-listing URL yield the same board

- **WHEN** a user submits `https://jobs.ashbyhq.com/blitzy/<uuid>` and another submits `https://jobs.ashbyhq.com/blitzy`
- **THEN** both derive source `ashby`, board `blitzy`, so the second is a duplicate of the first

### Requirement: URL canonicalization

The system SHALL canonicalize a submitted link — stripping the query string, fragment, a
trailing `/apply` segment, and any trailing slash — and store the canonical form.

#### Scenario: Tracking tails are stripped

- **WHEN** a user submits `https://job-boards.greenhouse.io/acme/jobs/456?utm_source=telegram#top`
- **THEN** the stored URL is `https://job-boards.greenhouse.io/acme/jobs/456` and the board is `acme`

### Requirement: Reject a board already in the catalogue

The system SHALL reject a contribution whose board is already crawled — any job exists whose
identity is under that board namespace — with a distinct "board already in catalogue" error,
and SHALL NOT record it or award AI credits.

#### Scenario: A board we already crawl is rejected

- **WHEN** a user submits a link for a board that already has jobs in the catalogue
- **THEN** the system responds 409 with a "board already in catalogue" error and awards no credits

### Requirement: Reject a board already contributed

The system SHALL reject a contribution whose board was already recorded (by any user), with a
distinct "board already contributed" error, and SHALL NOT record a second row or award AI
credits. The board — not the vacancy — is the uniqueness key, so any second link to the same
company collides.

#### Scenario: A second vacancy on the same board is rejected

- **WHEN** a user submits a link whose board matches an existing contribution
- **THEN** the system responds 409 with a "board already contributed" error and awards no credits

#### Scenario: Concurrent duplicate submissions credit at most one

- **WHEN** two requests for the same new board race
- **THEN** exactly one board is recorded and exactly one AI-credits reward is awarded; the other receives the "board already contributed" error

### Requirement: Recording a novel board and awarding AI credits

For a supported, non-duplicate board, the system SHALL record a contribution row — owner,
canonical URL, source, and board slug — and SHALL award the owner the configured AI-credits
contribution reward, idempotently keyed by the contribution id so retries never double-credit.
The reward banks above the monthly grant and does not expire. The system SHALL NOT maintain any
separate per-user "points" counter.

#### Scenario: Novel board is recorded and rewarded

- **WHEN** a user submits a supported link for a board we neither crawl nor already hold
- **THEN** a contribution row is recorded and the user's AI-credits balance increases by the contribution reward

#### Scenario: Reward is idempotent per contribution

- **WHEN** the reward for an already-recorded contribution is applied again (retry)
- **THEN** the AI-credits balance is unchanged — the reward is credited at most once per contribution

### Requirement: My contributions view

The system SHALL let an authenticated user list their own contributions, newest first, each
carrying its canonical URL, source, and board; the list SHALL be scoped to the caller and never
reveal another user's contributions.

#### Scenario: User lists their own contributions

- **WHEN** an authenticated user requests their contributions
- **THEN** the response contains only that user's boards, newest first

### Requirement: Contribute a board from Telegram

The system SHALL let a user who has linked their Telegram chat contribute a board by sending a
board link to the bot: the webhook resolves the chat to its user and runs the same contribution
flow, replying with the outcome. A message with no link SHALL draw no reply; a link from a chat
not linked to any user SHALL prompt the user to link their account first.

#### Scenario: Linked user's board link is recorded and rewarded

- **WHEN** a linked user sends a supported board link to the bot chat
- **THEN** the board is recorded, the user's AI-credits reward is credited, and the bot replies confirming the new board

#### Scenario: Second link on the same board earns no reward

- **WHEN** a linked user sends another link for a board they already contributed
- **THEN** no AI credits are credited and the bot replies that the board was already contributed

#### Scenario: Ordinary chatter is ignored

- **WHEN** a linked user sends a message with no link
- **THEN** the bot does not reply

#### Scenario: Unlinked chat is prompted to link

- **WHEN** a board link arrives from a chat not linked to any user
- **THEN** the bot replies prompting the user to link their account on the site first

