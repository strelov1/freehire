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
`q` SHALL return the unfiltered list.

The endpoint SHALL additionally accept repeatable facet query parameters —
`collections`, `regions`, `countries`, `domains`, `company_type`, and
`company_size` — each filtering against the company's corresponding denormalized
array (`collections` and the derived facet arrays) by **array overlap**: a company
matches a facet when its array shares at least one value with the requested values
(OR within a facet), and a company must match every provided facet (AND across
facets). Facet filters SHALL compose with the `q` name search. An absent facet
parameter SHALL not constrain the list.

When any filter (`q` or a facet) is applied, the list `meta.total` SHALL report
the count of companies matching the full filter combination, so pagination over
the filtered results is correct.

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

#### Scenario: Filtering by a single facet

- **WHEN** a client requests `GET /api/v1/companies?regions=europe`
- **THEN** the response contains only companies whose `regions` array contains
  `europe`, and `meta.total` is the count of such companies

#### Scenario: Multiple values within one facet are OR-ed

- **WHEN** a client requests `GET /api/v1/companies?regions=europe&regions=asia`
- **THEN** the response contains companies whose `regions` overlap
  `{europe, asia}` (in Europe **or** Asia)

#### Scenario: Different facets are AND-ed and compose with search

- **WHEN** a client requests
  `GET /api/v1/companies?collections=yc&company_type=startup&q=lab`
- **THEN** the response contains only companies that are in the `yc` collection
  **and** have `startup` among their `company_types` **and** whose name matches
  `lab`

### Requirement: Company job counts are denormalized and periodically recomputed

The system SHALL store each company's count of open jobs (`closed_at IS NULL`) in
a denormalized `companies.job_count` column, and its derived facet arrays
(`regions`, `countries`, `domains`, `company_types`, `company_sizes`) in
denormalized columns. Both SHALL be maintained by the same periodic recompute (a
scheduled worker), not by a synchronous write on the job ingest/close paths, so
they are eventually consistent with the `jobs` table within the recompute
interval. A company with no open jobs SHALL have `job_count = 0` and empty facet
arrays.

#### Scenario: Recompute reflects only open jobs

- **WHEN** the recompute runs and a company has 3 open jobs and 2 closed jobs
  (`closed_at` set)
- **THEN** that company's `job_count` is set to 3 and its facet arrays reflect
  only the 3 open jobs

#### Scenario: Recompute zeroes a company whose jobs all closed

- **WHEN** every job of a company has been closed since the last recompute and the
  recompute runs again
- **THEN** that company's `job_count` is set to 0 and its facet arrays are emptied

#### Scenario: Counts are eventually consistent

- **WHEN** a new job is ingested for a company between recompute runs
- **THEN** the company's `job_count` and facet arrays do not change until the next
  recompute, which then includes the new job

#### Scenario: Recompute rewrites nothing when already current

- **WHEN** the recompute runs and a company's `job_count` and every facet array
  already equal the freshly computed values
- **THEN** that company's row is not rewritten (the recompute reports it as
  unchanged)

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

### Requirement: Companies carry derived facet arrays aggregated from their open jobs

The system SHALL store, on each `companies` row, a set of denormalized facet
arrays derived from the company's **open** jobs (`closed_at IS NULL`):
`regions`, `countries`, `domains`, `company_types`, and `company_sizes` (each a
`TEXT[]`). Each array SHALL be the **distinct union** of the corresponding value
across the company's open jobs:

- `regions` and `countries` from the top-level `jobs.regions` / `jobs.countries`
  columns.
- `domains`, `company_types`, `company_sizes` from the job's `enrichment` payload
  (`domains` array, `company_type` scalar, `company_size` scalar); an unenriched
  or value-less job contributes nothing, so these arrays are sparse until jobs are
  enriched.

A company with no open jobs SHALL have every facet array empty (`'{}'`). The
arrays SHALL be maintained by the same periodic recompute that maintains
`job_count` (see the recompute requirement), not by a synchronous write on the
ingest/close paths, so they are eventually consistent with `jobs`.

#### Scenario: Region and country unions are derived from open jobs

- **WHEN** the recompute runs for a company whose open jobs have regions
  `{europe}`, `{europe, asia}` and countries `{de}`, `{de, sg}`
- **THEN** that company's `regions` is `{asia, europe}` and `countries` is
  `{de, sg}` (distinct union, closed jobs excluded)

#### Scenario: Enrichment facets are derived from the enrichment payload

- **WHEN** the recompute runs for a company whose open, enriched jobs carry
  `enrichment.domains` `{fintech}` and `{fintech, ecommerce}`,
  `enrichment.company_type` `startup` and `product`, and `enrichment.company_size`
  `11-50`
- **THEN** that company's `domains` is `{ecommerce, fintech}`, `company_types` is
  `{product, startup}`, and `company_sizes` is `{11-50}`

#### Scenario: Unenriched jobs contribute no enrichment facets

- **WHEN** a company's only open job has never been enriched (empty `enrichment`)
- **THEN** that company's `domains`, `company_types`, and `company_sizes` are all
  empty, while `regions`/`countries` still reflect the job's geography columns

#### Scenario: Closing all jobs empties the facet arrays

- **WHEN** every open job of a company is closed and the recompute runs again
- **THEN** that company's facet arrays are all set to empty (`'{}'`)

