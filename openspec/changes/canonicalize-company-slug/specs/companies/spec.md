## MODIFIED Requirements

### Requirement: Companies are stored as a slug-keyed entity

The system SHALL store companies in a `companies` table identified by a natural
`slug` key derived by `normalize.CompanySlug` from the company name — the
normalized name with one trailing legal form stripped — and resolved through the
company-slug alias registry. The table SHALL NOT use a surrogate id. Each company
SHALL have a display `name`.

A company row is normally created from a job's company name. A company MAY also
exist as a **reference row** created by the company info backfill without any
job referencing it; such a row SHALL be marked `is_reference = true`. Any orphan
cleanup that deletes companies no job references SHALL preserve reference rows.
When a job is later ingested whose normalized name matches a reference row's
`slug`, the existing row SHALL be reused (not duplicated), gaining jobs while
keeping its company-info data.

#### Scenario: Company is created from a job's company name

- **WHEN** a job is ingested with a non-empty company name that has no matching
  company row
- **THEN** the system inserts a `companies` row whose `slug` is the normalized
  name and whose `name` is the display name

#### Scenario: Existing company is reused, not duplicated

- **WHEN** a job is ingested whose normalized company name matches an existing
  `companies.slug`
- **THEN** no duplicate company row is created and the existing row is reused

#### Scenario: A legal-form variant reaches the same company row

- **WHEN** a job is ingested for `RingCentral, Inc.` and a company `ringcentral`
  already exists
- **THEN** the existing `ringcentral` row is reused rather than a second
  `ringcentral-inc` row being created

#### Scenario: Reference company survives orphan cleanup

- **WHEN** the orphan cleanup runs and a company row has no job referencing it
  but is marked `is_reference = true`
- **THEN** the row is not deleted

#### Scenario: A job adopts a reference company

- **WHEN** a job is ingested whose normalized company name matches the `slug` of
  a reference row
- **THEN** the existing row is reused, its job count reflects the new job, and
  its company-info fields are unchanged

## ADDED Requirements

### Requirement: A retired company slug SHALL redirect to its canonical company

The system SHALL respond to `GET /api/v1/companies/:slug` with a 301 to the canonical company's
path when no company row matches the slug but the slug is registered in `company_slug_aliases`.
A slug that matches neither SHALL continue to answer 404. The SSR company route SHALL propagate
that redirect to the browser rather than rendering its 404 page.

This applies to every slug the registry holds, whoever retired it — including the slugs
`cmd/backfill-company-names` re-keys through `RenameSlugCompany`, which today leave dead URLs.

#### Scenario: A merged slug redirects

- **WHEN** `GET /api/v1/companies/ringcentral-inc` is requested after that slug was merged into
  `ringcentral`
- **THEN** the response is a 301 to `/api/v1/companies/ringcentral`

#### Scenario: An unknown slug still 404s

- **WHEN** `GET /api/v1/companies/not-a-real-company` is requested and the slug is in neither
  `companies` nor `company_slug_aliases`
- **THEN** the response is 404

#### Scenario: A live company is not redirected

- **WHEN** a slug exists in `companies` AND in `company_slug_aliases`
- **THEN** the company is served with 200 — an existing row wins over the registry, so a
  re-created company is never shadowed by a stale alias

#### Scenario: The page follows the redirect

- **WHEN** a browser requests `/companies/ringcentral-inc`
- **THEN** the SSR route issues a 301 to `/companies/ringcentral` instead of rendering a 404
