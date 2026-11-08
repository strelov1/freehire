# job-public-identity Specification

## Purpose

TBD - created by archiving change add-job-public-slug. Update Purpose after archive.

## Requirements

### Requirement: Public slug identifier

Every job SHALL carry a `public_slug`: a URL-safe, human-readable identifier that
is unique across all jobs and is the only job identifier exposed by the public
API. The internal `BIGINT` `id` SHALL NOT appear in any public response field or
route.

The slug SHALL be composed of the normalized job title, the normalized company
name, and a short code derived from the job's dedup key `(source, external_id)`:
`<title-slug>-<company-slug>-<shortcode>`. Title and company normalization SHALL
reuse the existing slug normalization (the one used for `company_slug`).

The short code SHALL be a deterministic function of `(source, external_id)` only,
so that re-ingesting the same job (which upserts the same row) yields the same
slug. The slug MUST NOT depend on volatile fields such as the description.

#### Scenario: Slug is generated on job write

- **WHEN** a job is upserted with title "Senior Go Developer", company "Acme",
  source "manual", and external_id "42"
- **THEN** the stored `public_slug` equals `senior-go-developer-acme-<shortcode>`
  where `<shortcode>` is the deterministic short code of `("manual", "42")`

#### Scenario: Slug is stable across re-ingest

- **WHEN** the same job (same `source` and `external_id`) is upserted again with
  an edited description but unchanged title and company
- **THEN** the `public_slug` is unchanged

#### Scenario: Slug uniqueness for same title and company

- **WHEN** two distinct jobs share the same title and company but differ in
  `(source, external_id)`
- **THEN** their slugs differ by the short code, and both are stored without a
  uniqueness conflict

#### Scenario: Internal id is not exposed

- **WHEN** a job is returned by any public endpoint
- **THEN** the response identifies the job by `public_slug` and does not include
  the internal numeric `id`

### Requirement: Slug-based public routing

Public job endpoints SHALL address jobs by `public_slug` rather than by the
numeric id. A request for an unknown slug SHALL return 404.

#### Scenario: Fetch a job by slug

- **WHEN** a client requests `GET /api/v1/jobs/:slug` with an existing slug
- **THEN** the response is the matching job under `{"data": ...}`

#### Scenario: Unknown slug

- **WHEN** a client requests `GET /api/v1/jobs/:slug` with a slug that matches no
  job
- **THEN** the response status is 404

#### Scenario: Record a view by slug

- **WHEN** an authenticated client sends `POST /api/v1/jobs/:slug/view`
- **THEN** the slug is resolved to the job's internal id and the view is recorded
  in `user_jobs` for that (user, job)

#### Scenario: Mark applied by slug

- **WHEN** an authenticated client sends `POST /api/v1/jobs/:slug/apply`
- **THEN** the slug is resolved to the job's internal id and `applied_at` is set
  in `user_jobs` for that (user, job)
