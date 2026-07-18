## MODIFIED Requirements

### Requirement: Moderator creates a hand-curated vacancy

The system SHALL allow a user with the `moderator` role to create a vacancy through
`POST /api/v1/jobs`. The created job MUST be stored under the manual source identity
(`source = 'manual'`, `external_id = <url>`) and MUST record the creating user in
`created_by`. The request MUST be authenticated by session cookie or API key and then
authorized by role; a non-moderator MUST be rejected.

`url`, `title`, and `company` are required; `location`, `remote`, `description`, and
`posted_at` are optional, as are the structured facets `skills`, `regions`, `cities`,
`work_mode` and the salary fields `salary_min`, `salary_max`, `salary_currency`,
`salary_period`. `url` MUST be a valid `http`/`https` URL. The system SHALL derive geography
(countries/regions/work-mode), skill tags, the public slug, and the company slug from the
supplied fields using the same deterministic dictionaries the ingest pipeline uses. When the
request supplies `skills`, `regions`, `cities`, or `work_mode` explicitly, those values MUST
win over the dictionary derivation for that facet; when a facet is not supplied, derivation
MUST fill it as before. When the request supplies salary, the system MUST store it as an
authoritative manual salary on the job.

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
