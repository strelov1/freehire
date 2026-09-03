## MODIFIED Requirements

### Requirement: Recording a novel board and awarding AI credits

For a supported, non-duplicate board, the system SHALL insert a `boards` row — owner
(`submitted_by`), canonical URL, source, board slug, and surface — at status `pending`,
and SHALL award the owner the configured AI-credits contribution reward, idempotently
keyed by the inserted row's id so retries never double-credit. A `pending` row is crawled
starting with the provider's next scheduled run — recording a board is what makes it
eligible to crawl, not a separate, later onboarding step. The reward banks above the
monthly grant and does not expire. The system SHALL NOT maintain any separate per-user
"points" counter.

#### Scenario: Novel board is recorded and rewarded

- **WHEN** a user submits a supported link for a board we neither crawl nor already hold
- **THEN** a `boards` row is inserted at status `pending`, the user's AI-credits balance
  increases by the contribution reward, and the board is included in that provider's next
  scheduled `cmd/ingest` run

#### Scenario: Reward is idempotent per contribution

- **WHEN** the reward for an already-recorded board is applied again (retry)
- **THEN** the AI-credits balance is unchanged — the reward is credited at most once per
  inserted row

### Requirement: My contributions view

The system SHALL let an authenticated user list their own contributions, newest first,
each carrying its canonical URL, status, the surface it was submitted from, and — for a
recognized board — its source and board slug; a review-queue row carries no source or
board. Status SHALL be one of `pending` (recorded, not yet proven by a crawl), `active`
(proven by a successful crawl), `rejected` (failed validation), or `review` (an
unclassified URL, not yet a board — see "Record an unrecognized link for manual review").
The list SHALL be scoped to the caller and never reveal another user's contributions.

#### Scenario: User lists their own contributions

- **WHEN** an authenticated user requests their contributions
- **THEN** the response contains only that user's contributions, newest first, each with
  its status and originating surface

#### Scenario: A review-queue submission is listed without a board

- **WHEN** an authenticated user who submitted an unrecognized link requests their
  contributions
- **THEN** that row appears with status `review` and no source or board

#### Scenario: A recorded board's status reflects its crawl, not manual review

- **WHEN** a user's recorded board has not yet had a successful crawl
- **THEN** it is listed at status `pending`
- **AND WHEN** that board's first crawl succeeds
- **THEN** it is listed at status `active`, with no manual step in between
