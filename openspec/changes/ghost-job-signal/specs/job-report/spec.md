## MODIFIED Requirements

### Requirement: Authenticated user reports a problem with a vacancy

The system SHALL allow any authenticated user to report a live vacancy through
`POST /api/v1/jobs/:slug/reports`. The job is resolved from its public slug; a slug that
matches no job MUST be rejected with `404`. The report MUST be stored in a staging queue
with `status = 'pending'` and MUST record the reporting user. A report MUST NOT change the
reported job or any public read surface (list, search, company, sitemap) on its own. The
request MUST be authenticated by session cookie or API key; an unauthenticated request
MUST be rejected with `401`.

`reason` is required and MUST be one of the controlled values `no_response`, `not_relevant`,
`spam`, `fraud`, `other`; any other value MUST be rejected before any database write.
`details` and `contact_telegram` are **optional**. A report carrying only a reason MUST be
accepted.

`details` was previously mandatory. Requiring an explanation of a report whose reason
already says what is wrong buys a moderator little and costs every reporter a paragraph
before they can act — and a mandatory field mostly collects "spam" typed to get past it,
which is worse than nothing because it reads as evidence. The reason vocabulary is the
signal; free text is the elaboration, and elaboration is voluntary. When present, `details`
MUST still be bounded in length.

#### Scenario: User files a report with an explanation

- **WHEN** an authenticated user `POST`s `{ "reason": "fraud", "details": "asks for payment" }` to `/api/v1/jobs/<slug>/reports`
- **THEN** the system stores a `pending` report owned by that user against the resolved job and responds `201` with `{ "data": <report> }`

#### Scenario: A reason alone is a complete report

- **WHEN** an authenticated user `POST`s `{ "reason": "spam" }` with no `details`
- **THEN** the report is stored and the system responds `201`

#### Scenario: Blank details are accepted as absent

- **WHEN** an authenticated user `POST`s `details` consisting only of whitespace
- **THEN** the report is stored with empty details rather than rejected

#### Scenario: Unauthenticated report is rejected

- **WHEN** a request with no valid cookie or API key `POST`s to `/api/v1/jobs/<slug>/reports`
- **THEN** the system responds `401` and creates no report

#### Scenario: Unknown job slug is rejected

- **WHEN** an authenticated user `POST`s a report to a slug that matches no job
- **THEN** the system responds `404` and creates no report

#### Scenario: Invalid reason is rejected

- **WHEN** an authenticated user `POST`s a body whose `reason` is outside the controlled vocabulary
- **THEN** the system responds `400` before any database write

#### Scenario: Over-long details are rejected

- **WHEN** an authenticated user `POST`s `details` beyond the length bound
- **THEN** the system responds `400` before any database write
