## MODIFIED Requirements

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
`collections`, `regions`, `countries`, `domains`, `company_type`, `company_size`,
`remote_regions`, `yc_batch`, and `yc_status` — each filtering against the
company's corresponding denormalized array by **array overlap**: a company matches
a facet when its array shares at least one value with the requested values (OR
within a facet), and a company must match every provided facet (AND across facets).
The `remote_regions` facet filters the job-derived remote-hiring regions (a subset
of `regions`). The `yc_batch` and `yc_status` facets filter the curated
`companies.yc_batch` / `companies.yc_status` columns loaded from the YC directory
(see the `yc-company-enrichment` capability); a non-YC company has both empty and
matches neither. Facet filters SHALL compose with the `q` name search. An absent
facet parameter SHALL not constrain the list.

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

#### Scenario: Filtering by remote-hiring regions

- **WHEN** a client requests `GET /api/v1/companies?remote_regions=eu`
- **THEN** the response contains only companies whose `remote_regions` array
  contains `eu`, and `meta.total` is the count of such companies

#### Scenario: Filtering by YC batch and status

- **WHEN** a client requests `GET /api/v1/companies?yc_status=Active&yc_batch=Winter%202012`
- **THEN** the response contains only companies whose `yc_status` contains `Active`
  **and** whose `yc_batch` contains `Winter 2012`, and `meta.total` is the count of
  such companies
