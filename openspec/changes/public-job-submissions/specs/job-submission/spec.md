## ADDED Requirements

### Requirement: Authenticated user submits a vacancy for review

The system SHALL allow any authenticated user to submit a vacancy for moderation through
`POST /api/v1/submissions`. The submission MUST be stored in a staging queue with
`status = 'pending'` and MUST record the submitting user. The submission MUST NOT appear
in any public job surface (list, search, company, sitemap) until a moderator approves it.
The request MUST be authenticated by session cookie or API key; an unauthenticated request
MUST be rejected.

`url`, `title`, and `company` are required; `source`, `location`, `remote`, `description`,
and `posted_at` are optional. `url` MUST be a valid `http`/`https` URL. Submission content
MUST be validated by the same contract a moderator create uses, so an invalid body is
rejected before any write.

#### Scenario: User submits a job

- **WHEN** an authenticated user `POST`s `{ "url": "...", "title": "...", "company": "..." }` to `/api/v1/submissions`
- **THEN** the system stores a `pending` submission owned by that user and responds `201` with `{ "data": <submission> }`

#### Scenario: Unauthenticated submission is rejected

- **WHEN** a request with no valid cookie or API key `POST`s to `/api/v1/submissions`
- **THEN** the system responds `401` and creates no submission

#### Scenario: Missing required field is rejected

- **WHEN** an authenticated user `POST`s a body missing `url`, `title`, or `company`, or with a non-`http(s)` `url`
- **THEN** the system responds `400` before any database write

### Requirement: At most one pending submission per URL

The system SHALL treat the URL as a uniqueness key among `pending` submissions: while a
submission for a URL is awaiting review, a second submission of the same URL MUST be
rejected. Once a submission is approved or rejected it no longer blocks resubmission of
that URL.

#### Scenario: Duplicate pending submission is rejected

- **WHEN** a user `POST`s a URL that already has a `pending` submission
- **THEN** the system responds `409` and creates no second submission

#### Scenario: Resubmitting a decided URL is allowed

- **WHEN** a user `POST`s a URL whose only prior submission was rejected
- **THEN** the system stores a new `pending` submission and responds `201`

### Requirement: User sees the status of their own submissions

The system SHALL let an authenticated user read their own submissions through
`GET /api/v1/me/submissions`, each carrying its `status` and, when rejected, the review
reason. A user MUST see only their own submissions, never another user's.

#### Scenario: Listing own submissions

- **WHEN** an authenticated user `GET`s `/api/v1/me/submissions`
- **THEN** the system responds `200` with that user's submissions, each including `status` and `review_reason`, and no submissions belonging to other users

### Requirement: Moderator reviews the pending queue

The system SHALL let a `moderator` read all pending submissions through
`GET /api/v1/submissions`, including the submitter's email so the moderator can judge
provenance. The endpoint MUST be authorized by role; a non-moderator MUST be rejected.

#### Scenario: Moderator lists pending submissions

- **WHEN** a moderator `GET`s `/api/v1/submissions`
- **THEN** the system responds `200` with every `pending` submission, each including the submitter's email

#### Scenario: Non-moderator is forbidden from the queue

- **WHEN** an authenticated non-moderator `GET`s `/api/v1/submissions`
- **THEN** the system responds `403`

### Requirement: Moderator approves a submission into a live vacancy

The system SHALL let a `moderator` approve a pending submission through
`POST /api/v1/submissions/:id/approve`. Approval MUST mint a live vacancy from the
submission's fields using the existing moderator-create use case — so geography, skills,
slugs, dedup (`source`/`external_id = url`), and the enrichment enqueue are derived
identically to any moderator-authored job. The minted job's `created_by` MUST be the
**submitter**. The submission MUST then be marked `approved`, recording the reviewing
moderator and the minted job's id. Approving a submission that is not `pending` MUST be
rejected.

#### Scenario: Approving mints a job and marks the submission

- **WHEN** a moderator `POST`s `/api/v1/submissions/:id/approve` for a pending submission
- **THEN** the system creates a live vacancy whose `created_by` is the submitter, marks the submission `approved` with `reviewed_by` set to the moderator and `job_id` set to the new job, and responds `200`

#### Scenario: Approving an already-decided submission is rejected

- **WHEN** a moderator `POST`s `approve` for a submission whose status is already `approved` or `rejected`
- **THEN** the system responds `409` and changes nothing

#### Scenario: Non-moderator cannot approve

- **WHEN** an authenticated non-moderator `POST`s `approve`
- **THEN** the system responds `403` and changes nothing

### Requirement: Moderator rejects a submission

The system SHALL let a `moderator` reject a pending submission through
`POST /api/v1/submissions/:id/reject`, with an optional reason. The submission MUST be
marked `rejected`, recording the reviewing moderator and the reason. No job is created.
Rejecting a submission that is not `pending` MUST be rejected.

#### Scenario: Rejecting records the reason

- **WHEN** a moderator `POST`s `/api/v1/submissions/:id/reject` with `{ "reason": "duplicate" }` for a pending submission
- **THEN** the system marks the submission `rejected` with `reviewed_by` set and `review_reason` = "duplicate", creates no job, and responds `200`

#### Scenario: Rejecting an already-decided submission is rejected

- **WHEN** a moderator `POST`s `reject` for a submission whose status is not `pending`
- **THEN** the system responds `409` and changes nothing

### Requirement: Submitter identity is internal

The system SHALL store the submitting user id on each submission as ownership, used to
scope `GET /me/submissions`. This id MUST NOT be exposed on the wire shape returned to the
submitter. The moderator queue MAY surface the submitter's email for review.

#### Scenario: Submitter id is not in the self response

- **WHEN** a user reads their own submissions
- **THEN** the response body contains no raw submitter user id field
