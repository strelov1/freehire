# job-report Specification

## Purpose
TBD - created by archiving change report-job. Update Purpose after archive.
## Requirements
### Requirement: Authenticated user reports a problem with a vacancy

The system SHALL allow any authenticated user to report a live vacancy through
`POST /api/v1/jobs/:slug/reports`. The job is resolved from its public slug; a slug that
matches no job MUST be rejected with `404`. The report MUST be stored in a staging queue
with `status = 'pending'` and MUST record the reporting user. A report MUST NOT change the
reported job or any public read surface (list, search, company, sitemap) on its own. The
request MUST be authenticated by session cookie or API key; an unauthenticated request
MUST be rejected with `401`.

`reason` and `details` are required; `contact_telegram` is optional. `reason` MUST be one
of the controlled values `no_response`, `not_relevant`, `spam`, `fraud`, `other`; any other
value MUST be rejected before any database write. `details` MUST be non-empty after
trimming whitespace.

#### Scenario: User files a report

- **WHEN** an authenticated user `POST`s `{ "reason": "fraud", "details": "asks for payment" }` to `/api/v1/jobs/<slug>/reports`
- **THEN** the system stores a `pending` report owned by that user against the resolved job and responds `201` with `{ "data": <report> }`

#### Scenario: Unauthenticated report is rejected

- **WHEN** a request with no valid cookie or API key `POST`s to `/api/v1/jobs/<slug>/reports`
- **THEN** the system responds `401` and creates no report

#### Scenario: Unknown job slug is rejected

- **WHEN** an authenticated user `POST`s a report to a slug that matches no job
- **THEN** the system responds `404` and creates no report

#### Scenario: Invalid reason or empty details is rejected

- **WHEN** an authenticated user `POST`s a body whose `reason` is outside the controlled vocabulary, or whose `details` is missing or blank
- **THEN** the system responds `400` before any database write

### Requirement: At most one open report per user and job

The system SHALL treat `(reporting user, job)` as a uniqueness key among `pending`
reports: while a user's report of a job is awaiting review, a second report of the same job
by the same user MUST be rejected with `409`. A different user MAY report the same job at
any time. Once a user's report is resolved or dismissed it no longer blocks that user from
reporting the job again.

#### Scenario: Duplicate open report by the same user is rejected

- **WHEN** a user `POST`s a report for a job for which that user already has a `pending` report
- **THEN** the system responds `409` and creates no second report

#### Scenario: A different user reporting the same job is allowed

- **WHEN** a user `POST`s a report for a job that another user has already reported
- **THEN** the system stores a new `pending` report and responds `201`

#### Scenario: Reporting again after a decision is allowed

- **WHEN** a user `POST`s a report for a job whose only prior report by that user was resolved or dismissed
- **THEN** the system stores a new `pending` report and responds `201`

### Requirement: Moderator reviews the pending report queue

The system SHALL let a `moderator` read all pending reports through `GET /api/v1/reports`,
including the reporter's email and the reported job's slug and title so the moderator can
judge the report. The endpoint MUST be authorized by role; a non-moderator MUST be
rejected with `403`.

#### Scenario: Moderator lists pending reports

- **WHEN** a moderator `GET`s `/api/v1/reports`
- **THEN** the system responds `200` with every `pending` report, each including the reporter's email and the reported job's slug and title

#### Scenario: Non-moderator is forbidden from the queue

- **WHEN** an authenticated non-moderator `GET`s `/api/v1/reports`
- **THEN** the system responds `403`

### Requirement: Moderator resolves a report

The system SHALL let a `moderator` resolve a pending report through
`POST /api/v1/reports/:id/resolve`. The report MUST be marked `resolved`, recording the
reviewing moderator. When the request asks to close the reported job, the system MUST close
that job through the existing soft-close lifecycle (`closed_at`), so the job leaves the
list/search surfaces while its detail page and history survive. Resolving a report that is
not `pending` MUST be rejected with `409`.

The request MAY carry a moderator `note`, which MUST be stored on the report as its review
reason. When the request asks to notify the reporter, the system MUST mail the reporter the
decision after the report is marked, and the response MUST report whether that mail was
delivered. A mail failure MUST NOT unwind the decision.

#### Scenario: Resolving and closing the job

- **WHEN** a moderator `POST`s `/api/v1/reports/:id/resolve` with `{ "close_job": true }` for a pending report
- **THEN** the system marks the report `resolved` with the moderator recorded, soft-closes the reported job, and responds `200`

#### Scenario: Resolving without closing the job

- **WHEN** a moderator `POST`s `/api/v1/reports/:id/resolve` with `{ "close_job": false }` (or no flag) for a pending report
- **THEN** the system marks the report `resolved`, leaves the job open, and responds `200`

#### Scenario: Resolving an already-decided report is rejected

- **WHEN** a moderator `POST`s `resolve` for a report whose status is already `resolved` or `dismissed`
- **THEN** the system responds `409` and changes nothing

#### Scenario: Non-moderator cannot resolve

- **WHEN** an authenticated non-moderator `POST`s `resolve`
- **THEN** the system responds `403` and changes nothing

#### Scenario: The moderator note is stored as the review reason

- **WHEN** a moderator `POST`s `resolve` with `{ "note": "Fixed — the job is now marked Hybrid" }`
- **THEN** the system stores that text as the report's review reason

### Requirement: Moderator dismisses a report

The system SHALL let a `moderator` dismiss a pending report through
`POST /api/v1/reports/:id/dismiss`, with an optional reason. The report MUST be marked
`dismissed`, recording the reviewing moderator and the reason. The reported job MUST NOT
change. Dismissing a report that is not `pending` MUST be rejected with `409`.

When the request asks to notify the reporter, the system MUST mail the reporter the decision
after the report is marked, quoting the reason, and the response MUST report whether that
mail was delivered. A mail failure MUST NOT unwind the decision.

#### Scenario: Dismissing records the reason

- **WHEN** a moderator `POST`s `/api/v1/reports/:id/dismiss` with `{ "reason": "not a real issue" }` for a pending report
- **THEN** the system marks the report `dismissed` with the moderator and reason recorded, leaves the job unchanged, and responds `200`

#### Scenario: Dismissing an already-decided report is rejected

- **WHEN** a moderator `POST`s `dismiss` for a report whose status is not `pending`
- **THEN** the system responds `409` and changes nothing

### Requirement: Reporter identity is internal

The system SHALL store the reporting user id on each report as ownership. This id MUST NOT
be exposed on the wire shape returned to the reporter. The moderator queue MAY surface the
reporter's email for review.

#### Scenario: Reporter id is not in the create response

- **WHEN** a user files a report and reads the response
- **THEN** the response body contains no raw reporter user id field

### Requirement: The reporter is emailed the moderator's decision

The system SHALL email the reporter when a moderator decides their report and the decision
asks to notify them. The message MUST identify the reported job, state the outcome, and
quote the moderator's note when one was written; a blank note MUST degrade to a generic
statement rather than an empty quotation. The three outcomes — resolved with the job closed,
resolved with the job left open, and dismissed — MUST be distinguishable to the recipient.

Moderator- and reporter-authored text carried into the message MUST be escaped, so neither
can inject markup into the mail.

The notice MUST be sent only after the decision is recorded, so a message never claims an
outcome the system did not store.

#### Scenario: A resolved report notifies the reporter with the moderator's note

- **WHEN** a moderator resolves a report asking to notify the reporter, with a note
- **THEN** the system mails the reporter a message identifying the job, stating that the report was acted on, and quoting the note

#### Scenario: A dismissed report notifies the reporter

- **WHEN** a moderator dismisses a report asking to notify the reporter, with a reason
- **THEN** the system mails the reporter a message identifying the job, stating that nothing changed, and quoting the reason

#### Scenario: A closed job is distinguishable from a job left open

- **WHEN** a moderator resolves a report that also closes the job, asking to notify the reporter
- **THEN** the mailed message states that the vacancy was removed from the listings

#### Scenario: A blank note does not produce an empty quotation

- **WHEN** a moderator decides a report asking to notify the reporter, writing no note
- **THEN** the mailed message states the outcome in generic terms and quotes nothing

### Requirement: A decision may close a report without notifying the reporter

The system SHALL let a moderator record a decision without mailing the reporter, so reports
that warrant no reply — spam, duplicates, tests — close quietly. When the request does not
ask to notify, no message MUST be sent, and the decision MUST be recorded exactly as it is
when a notice is sent.

#### Scenario: A decision that opts out sends nothing

- **WHEN** a moderator resolves or dismisses a report without asking to notify the reporter
- **THEN** the system records the decision and sends no message

### Requirement: A failed notice never unwinds the decision

The system SHALL keep the moderator's decision when the notice cannot be delivered — an
unconfigured or failing mail transport MUST NOT leave the report pending, because a
moderation queue that stalls on an email outage is worse than an unsent notice. The decision
response MUST carry whether the reporter was notified, so the moderator can follow up by
hand instead of assuming a reply was sent.

#### Scenario: A mail failure leaves the report decided

- **WHEN** a moderator decides a report asking to notify the reporter and the mail transport fails
- **THEN** the system keeps the report decided and responds `200` reporting that the reporter was not notified

#### Scenario: An unconfigured mail transport is not an error

- **WHEN** a moderator decides a report asking to notify the reporter in an environment with no mail transport configured
- **THEN** the system records the decision and responds `200` reporting that the reporter was not notified

#### Scenario: A delivered notice is reported as such

- **WHEN** a moderator decides a report asking to notify the reporter and the mail is accepted by the transport
- **THEN** the response reports that the reporter was notified
