## MODIFIED Requirements

### Requirement: Authenticated user submits a vacancy for review

The system SHALL allow any authenticated user to submit a vacancy for moderation through
`POST /api/v1/submissions`. The submission MUST be stored in a staging queue with
`status = 'pending'` and MUST record the submitting user. The submission MUST NOT appear
in any public job surface (list, search, company, sitemap) until a moderator approves it.
The request MUST be authenticated by session cookie or API key; an unauthenticated request
MUST be rejected.

`url`, `title`, and `company` are required; `source`, `location`, `remote`, `description`,
and `posted_at` are optional, as are the structured facets `skills`, `regions`, `cities`,
`work_mode`, and the salary fields `salary_min`, `salary_max`, `salary_currency`,
`salary_period`. `url` MUST be a valid `http`/`https` URL. Submission content MUST be
validated by the same contract a moderator create uses, so an invalid body is rejected
before any write. The stored submission MUST retain the supplied structured facets and
salary so the moderator sees exactly what the submitter entered, and the submission
response MUST echo them back.

#### Scenario: User submits a job

- **WHEN** an authenticated user `POST`s `{ "url": "...", "title": "...", "company": "..." }` to `/api/v1/submissions`
- **THEN** the system stores a `pending` submission owned by that user and responds `201` with `{ "data": <submission> }`

#### Scenario: User submits a job with structured facets

- **WHEN** an authenticated user `POST`s a submission that also includes `skills`, `regions`, `cities`, `work_mode`, and salary fields
- **THEN** the system stores the `pending` submission with those facets and salary retained, and responds `201` with a `data` object echoing them

#### Scenario: Unauthenticated submission is rejected

- **WHEN** a request with no valid cookie or API key `POST`s to `/api/v1/submissions`
- **THEN** the system responds `401` and creates no submission

#### Scenario: Missing required field is rejected

- **WHEN** an authenticated user `POST`s a body missing `url`, `title`, or `company`, or with a non-`http(s)` `url`
- **THEN** the system responds `400` before any database write

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

## ADDED Requirements

### Requirement: The submit surface captures structured facets and a formatted description

The system SHALL present the `/submit` contribution form with inputs for the structured
facets in addition to the base fields: a skills chip input, a region selector, a city input,
a work-mode selector, and salary inputs (min, max, currency, period). These inputs SHALL
reuse the catalogue's shared facet vocabularies (region labels, country/region map, the
work-mode vocabulary, the currency list) so a submitter's choices align with the values the
filter and catalogue use. The description field SHALL use the same rich (markdown) editor the
job tracker uses, and its content SHALL be submitted as HTML so it matches the catalogue's
sanitized-HTML description contract.

#### Scenario: Submitter enters structured facets on the form

- **WHEN** a signed-in user opens `/submit`
- **THEN** the form shows inputs for skills, region, city, work mode, and salary alongside URL/title/company, drawn from the shared facet vocabularies

#### Scenario: Description is authored with the rich editor and sent as HTML

- **WHEN** a submitter writes a description using the form's rich editor and submits
- **THEN** the description is sent to `POST /api/v1/submissions` as HTML consistent with how the catalogue renders descriptions
