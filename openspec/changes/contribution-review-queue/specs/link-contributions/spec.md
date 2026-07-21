## MODIFIED Requirements

### Requirement: Supported-ATS board recognition

The system SHALL accept a link as a board contribution only when its host belongs to a
supported multi-tenant ATS and the URL yields a board slug, determined without any network
request. A link from an unknown host, a single-tenant source, or a URL with no board segment
SHALL NOT be recorded as a board; instead, when it is a well-formed `http(s)` URL it SHALL be
recorded for manual review (see "Record an unrecognized link for manual review"), and only a
value that is not a well-formed `http(s)` URL SHALL be rejected with the "unsupported ATS"
error. Both a vacancy URL and a bare board-listing URL for the same company SHALL yield the
same board.

#### Scenario: Non-URL garbage is rejected

- **WHEN** a user submits a value that is not a well-formed `http(s)` URL
- **THEN** the system responds 422 with an "unsupported ATS" error and records nothing

#### Scenario: Unknown host is recorded for review

- **WHEN** a user submits `https://example.com/careers/123`
- **THEN** no board is derived, so the link is recorded for manual review rather than rejected

#### Scenario: Vacancy URL and board-listing URL yield the same board

- **WHEN** a user submits `https://jobs.ashbyhq.com/blitzy/<uuid>` and another submits `https://jobs.ashbyhq.com/blitzy`
- **THEN** both derive source `ashby`, board `blitzy`, so the second is a duplicate of the first

### Requirement: My contributions view

The system SHALL let an authenticated user list their own contributions, newest first, each
carrying its canonical URL, status, and — for a recognized board — its source and board slug;
a review-queue row carries no source or board. The list SHALL be scoped to the caller and
never reveal another user's contributions.

#### Scenario: User lists their own contributions

- **WHEN** an authenticated user requests their contributions
- **THEN** the response contains only that user's contributions, newest first, each with its status

#### Scenario: A review-queue submission is listed without a board

- **WHEN** an authenticated user who submitted an unrecognized link requests their contributions
- **THEN** that row appears with status `review` and no source or board

## ADDED Requirements

### Requirement: Record an unrecognized link for manual review

When a submitted link is a well-formed `http(s)` URL but yields no supported board, the system
SHALL record it as a review-queue contribution — owner and canonical URL, status `review`, no
source or board — and SHALL NOT award any AI credits. A URL already in the review queue SHALL
be rejected with the "board already contributed" error and SHALL NOT create a second row. Credit
for such a link is granted only later, by hand, once a maintainer confirms the source is
ingestable and promotes the row.

#### Scenario: Unrecognized valid link is recorded without credit

- **WHEN** an authenticated user submits a well-formed link that yields no supported board
- **THEN** a row is recorded with status `review` and no source or board, and the user's AI-credits balance is unchanged

#### Scenario: Duplicate review link is rejected

- **WHEN** a user submits a link already present in the review queue
- **THEN** the system responds 409 and records no second row

#### Scenario: Review row earns no credit at submit time

- **WHEN** a review-queue row is created
- **THEN** no AI-credits reward is applied — credit remains exclusive to recognized novel boards
