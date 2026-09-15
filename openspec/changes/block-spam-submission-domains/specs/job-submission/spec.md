## ADDED Requirements

### Requirement: Submission from a blocked domain is refused

The system SHALL maintain a blocklist of hosts and, before storing a submission, check
the submitted URL's host against it. A host matches when equal case-insensitively to a
blocklist entry after a leading `www.` is stripped from both sides. A matching
submission MUST be refused and no `job_submissions` row created; a non-matching
submission is unaffected. The blocklist check applies only to
`POST /api/v1/submissions`; it MUST NOT be applied to a moderator's own hand-authored
vacancy create.

#### Scenario: Submission of a blocked host is refused

- **WHEN** an authenticated user `POST`s a submission whose URL's host is on the
  blocklist
- **THEN** the system responds `403` and creates no submission

#### Scenario: A `www.` prefix does not evade or over-match the blocklist

- **WHEN** `gridnaut.site` is on the blocklist and a user submits a URL whose host is
  `www.gridnaut.site`
- **THEN** the system responds `403` and creates no submission

#### Scenario: Non-blocked host is unaffected

- **WHEN** an authenticated user `POST`s a submission whose URL's host is not on the
  blocklist
- **THEN** the system stores the submission as `pending`, unchanged from today

#### Scenario: Moderator create bypasses the blocklist

- **WHEN** a moderator creates a hand-authored vacancy whose URL's host is on the
  blocklist
- **THEN** the system creates the vacancy normally; the blocklist is not consulted

## MODIFIED Requirements

### Requirement: Moderator rejects a submission

The system SHALL let a `moderator` reject a pending submission through
`POST /api/v1/submissions/:id/reject`, with an optional reason. The submission MUST be
marked `rejected`, recording the reviewing moderator and the reason. No job is created.
Rejecting a submission that is not `pending` MUST be rejected.

The request MAY also carry `block_domain: true`. When set, the system MUST, in the
same action, add the submission URL's host to the blocklist (recording the rejecting
moderator and the reject reason, when given) so a later submission of that host is
refused per "Submission from a blocked domain is refused". Adding a host already on the
blocklist MUST NOT error — it is a no-op for the blocklist, and the rejection still
proceeds.

#### Scenario: Rejecting records the reason

- **WHEN** a moderator `POST`s `/api/v1/submissions/:id/reject` with `{ "reason": "duplicate" }` for a pending submission
- **THEN** the system marks the submission `rejected` with `reviewed_by` set and `review_reason` = "duplicate", creates no job, and responds `200`

#### Scenario: Rejecting an already-decided submission is rejected

- **WHEN** a moderator `POST`s `reject` for a submission whose status is not `pending`
- **THEN** the system responds `409` and changes nothing

#### Scenario: Rejecting with block_domain also blocks the host

- **WHEN** a moderator `POST`s `reject` with `{ "reason": "referral spam", "block_domain": true }` for a pending submission whose URL host is `gridnaut.site`
- **THEN** the system marks the submission `rejected` as usual and adds `gridnaut.site` to the blocklist, attributed to that moderator with reason "referral spam"

#### Scenario: Blocking an already-blocked domain while rejecting is a no-op for the blocklist

- **WHEN** a moderator rejects a submission with `block_domain: true` whose host is already on the blocklist
- **THEN** the system rejects the submission as usual and the blocklist is unchanged (still exactly one entry for that host)
