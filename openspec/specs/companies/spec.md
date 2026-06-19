# companies

## Purpose

Store companies as a first-class, slug-keyed entity linked from jobs, so the API
can serve a company catalog and a company-detail view (company + its jobs)
without joining the `jobs` table on the hot read paths.
## Requirements
### Requirement: Companies are stored as a slug-keyed entity

The system SHALL store companies in a `companies` table identified by a natural
`slug` key derived by normalizing the company name. The table SHALL NOT use a
surrogate id. Each company SHALL have a display `name`.

#### Scenario: Company is created from a job's company name

- **WHEN** a job is ingested with a non-empty company name that has no matching
  company row
- **THEN** the system inserts a `companies` row whose `slug` is the normalized
  name and whose `name` is the display name

#### Scenario: Existing company is reused, not duplicated

- **WHEN** a job is ingested whose normalized company name matches an existing
  `companies.slug`
- **THEN** no duplicate company row is created and the existing row is reused

### Requirement: Jobs link to a company via a denormalized key

The system SHALL store `company_slug` on each job as the normalized link key,
kept alongside the existing `company` display name. Jobs with an empty company
name SHALL have an empty `company_slug` and SHALL NOT create a company.

#### Scenario: Job carries both display name and link key

- **WHEN** a job with company name "Yandex LLC" is ingested
- **THEN** the job's `company` is the display name and its `company_slug` is the
  normalized key, and a matching `companies` row exists with that `slug`

#### Scenario: Job with no company

- **WHEN** a job is ingested with an empty company name
- **THEN** the job is stored with an empty `company_slug` and no company row is
  created

### Requirement: Company list is served without joining jobs

The system SHALL expose `GET /api/v1/companies` returning companies read from the
`companies` table. Each company's job count SHALL be read from the denormalized
`companies.job_count` column (open jobs only), not computed at query time, so the
read path performs no join to the `jobs` table. The list SHALL be ordered by
`job_count` descending, then `name` ascending, so the most active companies
surface first.

The endpoint SHALL accept an optional `q` query parameter that filters companies
by a case-insensitive substring match on the company `name`. An absent or empty
`q` SHALL return the unfiltered list. When `q` is non-empty, the list `meta.total`
SHALL report the count of companies matching `q`, so pagination over the filtered
results is correct.

#### Scenario: Listing companies most-active first

- **WHEN** a client requests `GET /api/v1/companies`
- **THEN** the response contains companies under `data` with list `meta`,
  ordered by `job_count` descending (ties broken by `name`), each carrying its
  denormalized `job_count`

#### Scenario: Searching companies by name

- **WHEN** a client requests `GET /api/v1/companies?q=acme`
- **THEN** the response contains only companies whose name matches `acme`
  case-insensitively, ordered by `job_count` descending, and `meta.total` is the
  count of matching companies

#### Scenario: Empty query returns the full list

- **WHEN** a client requests `GET /api/v1/companies?q=` (empty or absent)
- **THEN** the response is the unfiltered company list, identical to omitting the
  parameter

### Requirement: Company job counts are denormalized and periodically recomputed

The system SHALL store each company's count of open jobs (`closed_at IS NULL`) in
a denormalized `companies.job_count` column. The column SHALL be maintained by a
periodic recompute (a scheduled worker), not by a synchronous write on the job
ingest/close paths, so it is eventually consistent with the `jobs` table within
the recompute interval. A company with no open jobs SHALL have `job_count = 0`.

#### Scenario: Recompute reflects only open jobs

- **WHEN** the recompute runs and a company has 3 open jobs and 2 closed jobs
  (`closed_at` set)
- **THEN** that company's `job_count` is set to 3

#### Scenario: Recompute zeroes a company whose jobs all closed

- **WHEN** every job of a company has been closed since the last recompute and the
  recompute runs again
- **THEN** that company's `job_count` is set to 0

#### Scenario: Counts are eventually consistent

- **WHEN** a new job is ingested for a company between recompute runs
- **THEN** the company's `job_count` does not change until the next recompute,
  which then includes the new job

### Requirement: Company detail returns the company with its jobs

The system SHALL expose `GET /api/v1/companies/:slug` returning the company and
its **open** jobs (`closed_at IS NULL`). The company SHALL be read from
`companies` and its jobs from a single-table filter on `jobs.company_slug` —
without a SQL join between the two tables.

#### Scenario: Existing company

- **WHEN** a client requests `GET /api/v1/companies/:slug` for an existing slug
- **THEN** the response contains the company and its open jobs ordered like the
  main jobs listing

#### Scenario: Unknown company

- **WHEN** a client requests `GET /api/v1/companies/:slug` for a slug with no
  company row
- **THEN** the system responds with HTTP 404

#### Scenario: Closed job leaves the company page

- **WHEN** a company's job is closed
- **THEN** the company detail no longer lists it

