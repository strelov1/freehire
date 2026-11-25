# job-submission Specification

## Purpose
TBD - created by archiving change public-job-submissions. Update Purpose after archive.
## Requirements
### Requirement: Authenticated user submits a vacancy for review

The system SHALL allow any authenticated user to submit a vacancy for moderation through
`POST /api/v1/submissions`. The submission MUST be stored in a staging queue with
`status = 'pending'` and MUST record the submitting user. The submission MUST NOT appear
in any public job surface (list, search, company, sitemap) until a moderator approves it.
The request MUST be authenticated by session cookie or API key; an unauthenticated request
MUST be rejected.

`url`, `title`, and `company` are required; `source`, `location`, `remote`, `description`,
and `posted_at` are optional, as are the structured facets `skills`, `regions`, `cities`,
`work_mode`, `employment_type`, `seniority`, and the salary fields `salary_min`,
`salary_max`, `salary_currency`, `salary_period`. `url` MUST be a valid `http`/`https`
URL. `employment_type` and `seniority` MUST be validated against the same controlled
vocabularies (`vocab.EmploymentTypeValues`, `vocab.SeniorityValues`) the catalogue's
filters use; a value outside the vocabulary MUST be dropped (not rejected), degrading to
dictionary derivation exactly as an unrecognized `work_mode` already does. Submission
content MUST be validated by the same contract a moderator create uses, so an invalid
body is rejected before any write. The stored submission MUST retain the supplied
structured facets and salary so the moderator sees exactly what the submitter entered,
and the submission response MUST echo them back.

#### Scenario: User submits a job

- **WHEN** an authenticated user `POST`s `{ "url": "...", "title": "...", "company": "..." }` to `/api/v1/submissions`
- **THEN** the system stores a `pending` submission owned by that user and responds `201` with `{ "data": <submission> }`

#### Scenario: User submits a job with structured facets

- **WHEN** an authenticated user `POST`s a submission that also includes `skills`, `regions`, `cities`, `work_mode`, `employment_type`, `seniority`, and salary fields
- **THEN** the system stores the `pending` submission with those facets and salary retained, and responds `201` with a `data` object echoing them

#### Scenario: An out-of-vocabulary employment type or seniority is dropped, not rejected

- **WHEN** an authenticated user `POST`s a submission whose `employment_type` or `seniority` value is not in the controlled vocabulary
- **THEN** the system stores the `pending` submission with that field left empty (falling back to dictionary derivation on approval) and still responds `201`

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
identically to any moderator-authored job. Any structured facets the submission carries
(`skills`, `regions`, `cities`, `work_mode`) MUST be applied to the minted job as explicit
overrides that win over dictionary derivation, and any salary the submission carries MUST be
applied to the minted job as an authoritative manual salary. The minted job's `created_by`
MUST be the **submitter**. The submission MUST then be marked `approved`, recording the
reviewing moderator and the minted job's id. Approving a submission that is not `pending`
MUST be rejected.

#### Scenario: Approving mints a job and marks the submission

- **WHEN** a moderator `POST`s `/api/v1/submissions/:id/approve` for a pending submission
- **THEN** the system creates a live vacancy whose `created_by` is the submitter, marks the submission `approved` with `reviewed_by` set to the moderator and `job_id` set to the new job, and responds `200`

#### Scenario: Approving carries the submission's structured facets and salary onto the job

- **WHEN** a moderator approves a pending submission that carries explicit `skills`, `regions`, `cities`, `work_mode`, and salary
- **THEN** the minted job's geography/work-mode/skills reflect those explicit values (winning over what the dictionaries would derive) and the job carries the submission's salary as its authoritative manual salary

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

### Requirement: Submitter identity is internal

The system SHALL store the submitting user id on each submission as ownership, used to
scope `GET /me/submissions`. This id MUST NOT be exposed on the wire shape returned to the
submitter. The moderator queue MAY surface the submitter's email for review.

#### Scenario: Submitter id is not in the self response

- **WHEN** a user reads their own submissions
- **THEN** the response body contains no raw submitter user id field

### Requirement: The submit surface captures structured facets and a formatted description

The system SHALL present the `/submit` contribution form with inputs for the structured
facets in addition to the base fields: a skills chip input, a region selector, a city input,
a work-mode selector, an employment-type selector, a seniority selector, and salary inputs
(min, max, currency, period). These inputs SHALL reuse the catalogue's shared facet
vocabularies (region labels, country/region map, the work-mode vocabulary, the
employment-type vocabulary, the seniority vocabulary, the currency list) so a submitter's
choices align with the values the filter and catalogue use. The description field SHALL
use the same rich (markdown) editor the job tracker uses, and its content SHALL be
submitted as HTML so it matches the catalogue's sanitized-HTML description contract.

The form SHALL offer a Preview view alongside the entry fields, rendering the in-progress
draft (company, title, salary, facets, and the formatted description) the way it will
appear once approved, without making any network request and without any of the
interactive affordances (apply, save, vote, report, discussion) that require a persisted
job.

#### Scenario: Submitter enters structured facets on the form

- **WHEN** a signed-in user opens `/submit`
- **THEN** the form shows inputs for skills, region, city, work mode, employment type, seniority, and salary alongside URL/title/company, drawn from the shared facet vocabularies

#### Scenario: Description is authored with the rich editor and sent as HTML

- **WHEN** a submitter writes a description using the form's rich editor and submits
- **THEN** the description is sent to `POST /api/v1/submissions` as HTML consistent with how the catalogue renders descriptions

#### Scenario: Submitter previews the draft before submitting

- **WHEN** a signed-in user fills in some fields on `/submit` and switches to the Preview view
- **THEN** the system renders those fields in the vacancy's presentational layout (company, title, salary, facets, formatted description) without issuing any network request

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

### Requirement: Submitter can prefill the form from a job URL

The system SHALL let a signed-in user request a prefill of the `/submit` form's fields
from a job URL through `POST /api/v1/submissions/prefill`. The endpoint MUST parse the
URL using the same destination-recognition registry (`internal/linksource`) that backs
the existing paste-a-link contribution flow, MUST NOT persist anything (no job row, no
submission, no dedup check, no credit reward, no enrichment enqueue, no search push),
and MUST require the same authentication `POST /api/v1/submissions` requires. When the
URL is recognized and resolves to a single vacancy, the response MUST include whichever
of `title`, `company`, `location`, `description`, `work_mode`, `employment_type`,
`seniority`, and the resolved `source` key the destination page states. When the URL is
unrecognized, or recognized but not a single-vacancy page, the endpoint MUST respond
`200` with those fields empty rather than an error — prefill is a best-effort aid, and
its absence MUST NOT block manual entry.

#### Scenario: Prefill from a recognized job URL

- **WHEN** a signed-in user `POST`s a URL for a vacancy on a recognized platform to `/api/v1/submissions/prefill`
- **THEN** the system responds `200` with the parsed `title`, `company`, `location`, `description`, and any structured facets the platform stated, and creates no job, submission, or credit reward

#### Scenario: Prefill from an unrecognized or non-vacancy URL degrades silently

- **WHEN** a signed-in user `POST`s a URL that no destination adapter matches, or that matches but is not a single vacancy page
- **THEN** the system responds `200` with empty fields, not an error

#### Scenario: Prefill requires authentication

- **WHEN** a request with no valid cookie or API key `POST`s to `/api/v1/submissions/prefill`
- **THEN** the system responds `401`

#### Scenario: Prefill never overwrites a field the submitter already filled in

- **WHEN** a submitter has already typed a value into a form field and then triggers prefill for a URL that also carries a value for that field
- **THEN** the form keeps the submitter's own value in that field

