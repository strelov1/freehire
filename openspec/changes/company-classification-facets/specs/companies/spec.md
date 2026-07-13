# companies

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
`remote_regions`, `yc_batch`, `yc_status`, `yc_stage`, and `yc_flags` — each
filtering against the company's corresponding denormalized array by **array
overlap**: a company matches a facet when its array shares at least one value with
the requested values (OR within a facet), and a company must match every provided
facet (AND across facets). The `remote_regions` facet filters the job-derived
remote-hiring regions (a subset of `regions`). The `yc_batch`, `yc_status`,
`yc_stage`, and `yc_flags` facets filter the curated YC-directory columns (see the
`yc-company-enrichment` capability); a non-YC company has them empty and matches
none. Facet filters SHALL compose with the `q` name search. An absent facet
parameter SHALL not constrain the list.

The endpoint SHALL additionally accept the repeatable **scalar** facet parameter
`maturity`, filtering against the company's single-valued `companies.maturity`
column by **membership**: a company matches when its scalar value is among the
requested values (OR within the facet), and this facet ANDs with the others and
with `q` exactly like the array facets. A company whose `maturity` is `NULL`
(unknown) matches no `maturity` filter. `maturity` values are `government`,
`startup`, `scaleup`, `enterprise`.

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

#### Scenario: Filtering by YC facets

- **WHEN** a client requests `GET /api/v1/companies?yc_stage=Growth&yc_flags=top_company`
- **THEN** the response contains only companies whose `yc_stage` contains `Growth`
  **and** whose `yc_flags` contains `top_company`, and `meta.total` is the count of
  such companies

#### Scenario: Filtering by the scalar maturity facet

- **WHEN** a client requests `GET /api/v1/companies?maturity=startup&maturity=scaleup`
- **THEN** the response contains only companies whose `maturity` is `startup` **or**
  `scaleup`, excluding any company whose `maturity` is `NULL`, and `meta.total` is
  the count of such companies

### Requirement: Company job counts are denormalized and periodically recomputed

The system SHALL store each company's count of open jobs (`closed_at IS NULL`) in
a denormalized `companies.job_count` column, and its derived facet arrays
(`regions`, `countries`, `domains`, `company_types`, `company_sizes`,
`remote_regions`) in denormalized columns. Both SHALL be maintained by the same
periodic recompute (a scheduled worker), not by a synchronous write on the job
ingest/close paths, so they are eventually consistent with the `jobs` table within
the recompute interval. A company with no open jobs SHALL have `job_count = 0` and
empty facet arrays. `remote_regions` SHALL be maintained as the distinct union of
`regions` over the company's open jobs with `work_mode = 'remote'`, so it is a
subset of the `regions` array; a company with no open remote job has an empty
`remote_regions`.

The same recompute SHALL additionally maintain one **deterministic, single-valued**
classification column, `companies.maturity`, computed from signals already stored
(`organization_type`, `yc_status`, `employee_count`, `year_founded`, and whether
the company's open jobs come from an exclusively-government `source`). The
derivation SHALL be a pure rule (no LLM), applied in precedence order: `maturity`
is `government` when the company is government-sourced or `organization_type` is
`Government`, else `startup` when it is a YC company or is small and recently
founded, else `enterprise` when its employee count is large, else `scaleup` for
mid-size, else `NULL` (unknown). Where signals are silent, the value SHALL be
`NULL` — an honest abstain, never a fabricated label. This column SHALL be
maintained under the same `IS DISTINCT FROM` change-guard as the other facets, so
an unchanged company is not rewritten.

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

#### Scenario: remote_regions is derived from open remote jobs only

- **WHEN** the recompute runs for a company whose open jobs are one `remote` job in
  `eu` and one `onsite` job in `north_america`
- **THEN** that company's `remote_regions` is `{eu}` (the onsite job's region is
  excluded) while its `regions` is `{eu, north_america}`

#### Scenario: maturity is derived deterministically

- **WHEN** the recompute runs for a YC company with `employee_count = 20`
- **THEN** its `maturity` is `startup`

#### Scenario: Unknown maturity abstains to NULL

- **WHEN** the recompute runs for a company with no government source, no YC
  status, and no `employee_count`
- **THEN** its `maturity` is `NULL` (unknown), not a guessed label
