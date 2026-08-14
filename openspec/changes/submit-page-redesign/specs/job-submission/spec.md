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

## ADDED Requirements

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
