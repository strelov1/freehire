# job-authoring Specification

## Purpose
TBD - created by archiving change moderator-create-jobs. Update Purpose after archive.
## Requirements
### Requirement: Moderator creates a hand-curated vacancy

The system SHALL allow a user with the `moderator` role to create a vacancy through
`POST /api/v1/jobs`. The created job MUST be stored under the manual source identity
(`source = 'manual'`, `external_id = <url>`) and MUST record the creating user in
`created_by`. The request MUST be authenticated by session cookie or API key and then
authorized by role; a non-moderator MUST be rejected.

`url`, `title`, and `company` are required; `location`, `remote`, `description`, and
`posted_at` are optional, as are the structured facets `skills`, `regions`, `cities`,
`work_mode` and the salary fields `salary_min`, `salary_max`, `salary_currency`,
`salary_period`. `url` MUST be a valid `http`/`https` URL. The system SHALL derive
geography (countries/regions/work-mode), skill tags, the public slug, and the company
slug from the supplied fields using the same deterministic dictionaries the ingest
pipeline uses. When the request supplies `skills`, `regions`, `cities`, or `work_mode`
explicitly, those values MUST win over the dictionary derivation for that facet; when a
facet is not supplied, derivation MUST fill it as before. When the request supplies
salary, the system MUST store it as an authoritative manual salary on the job.

#### Scenario: Moderator creates a job

- **WHEN** a moderator `POST`s `{ "url": "...", "title": "...", "company": "..." }`
- **THEN** the system stores a job with `source='manual'`, `external_id` equal to the URL, `created_by` set to the moderator, and responds `201` with `{ "data": <job> }`

#### Scenario: Explicit structured facets override derivation

- **WHEN** a moderator `POST`s a create body that supplies explicit `regions`, `cities`, `work_mode`, and `skills`
- **THEN** the created job carries those explicit values for those facets rather than the values the dictionaries would derive from location and description

#### Scenario: Supplied salary is stored as an authoritative manual salary

- **WHEN** a moderator `POST`s a create body that supplies salary fields
- **THEN** the created job carries that salary as its authoritative manual salary

#### Scenario: Non-moderator is rejected

- **WHEN** an authenticated user without the `moderator` role `POST`s to `/api/v1/jobs`
- **THEN** the system responds `403` and creates no job

#### Scenario: Unauthenticated request is rejected

- **WHEN** a request with no valid cookie or API key `POST`s to `/api/v1/jobs`
- **THEN** the system responds `401` and creates no job

#### Scenario: Missing required field is rejected

- **WHEN** a moderator `POST`s a body missing `url`, `title`, or `company`, or with a non-`http(s)` `url`
- **THEN** the system responds `400` before any database write

### Requirement: Re-creating the same URL is idempotent

The system SHALL treat `external_id = <url>` as the dedup key for manual jobs. Re-`POST`ing
the same URL MUST update the existing manual job's content rather than create a duplicate,
and MUST set `updated_by` to the acting moderator. The job's public slug MUST remain stable
across such updates.

#### Scenario: Re-POST updates instead of duplicating

- **WHEN** a moderator `POST`s a URL that already exists as a manual job
- **THEN** the system updates the existing row's content, sets `updated_by`, keeps the same `public_slug`, and does not create a second job

### Requirement: New manual jobs are queued for enrichment

The system SHALL enqueue a newly created manual job for AI enrichment in the same
transaction as the write, identically to every other source. Editing via `PATCH` does
NOT re-enqueue enrichment.

#### Scenario: Create enqueues enrichment

- **WHEN** a moderator creates a new manual job
- **THEN** an enrichment-outbox row is enqueued atomically with the job write

### Requirement: Moderator edits a manual vacancy

The system SHALL allow a moderator to partially update a manual vacancy through
`PATCH /api/v1/jobs/:slug`. Only fields present in the request body are changed; absent
fields are left unchanged. The system MUST set `updated_by` to the acting moderator and
MUST NOT change the job's source identity (`url` / `external_id` / `public_slug`).

When a content field that feeds a deterministic facet changes (location → geography,
description → skills, company → company slug), the system SHALL recompute that facet from
the resulting values so the stored facets stay consistent with the edited content. (AI
enrichment is not re-run — that remains a separate concern.)

#### Scenario: Partial update changes only supplied fields

- **WHEN** a moderator `PATCH`es `/api/v1/jobs/:slug` with `{ "title": "New" }`
- **THEN** the job's title becomes `New`, all other content fields are unchanged, `updated_by` is set, and the response is `200` with `{ "data": <job> }`

#### Scenario: Editing location recomputes geography

- **WHEN** a moderator `PATCH`es a job's `location` to one that resolves to a new country
- **THEN** the job's stored countries/regions facet reflects the new location

#### Scenario: Editing a non-manual job is rejected

- **WHEN** a moderator `PATCH`es the slug of a job whose `source` is not `manual` (e.g. an ATS or Telegram job)
- **THEN** the system responds `404` and changes nothing

#### Scenario: Editing an unknown slug returns not found

- **WHEN** a moderator `PATCH`es a slug that matches no job
- **THEN** the system responds `404`

### Requirement: Authorship audit is internal

The system SHALL store `created_by` and `updated_by` on jobs as authorship audit. These
fields MUST be `NULL` for jobs created by automated sources and MUST NOT be exposed on the
public job wire shape.

#### Scenario: Audit fields are not in the wire response

- **WHEN** any job is returned over the API
- **THEN** the response body contains no `created_by` or `updated_by` field

