## MODIFIED Requirements

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

## ADDED Requirements

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
