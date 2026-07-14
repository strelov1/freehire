## ADDED Requirements

### Requirement: Companies carry a clean YC subindustry classification

The system SHALL store, on each `companies` row, a nullable scalar `subindustry` (`TEXT`)
holding the leaf of the company's YC subindustry path (e.g. `Industrials -> Manufacturing and
Robotics` → `Manufacturing and Robotics`). It SHALL be populated by `cmd/import-yc` from the
directory entry's `subindustry` field and SHALL be distinct from the existing
`companies.industries` array, which continues to hold the flattened, tag-inclusive display bag
unchanged. A company with no YC subindustry (a non-YC company, or a YC entry without one) SHALL
have `subindustry = NULL`. The value SHALL be a clean, human-readable taxonomy leaf, not a free
tag, so it is safe to offer as a filter option.

#### Scenario: Import stores the subindustry leaf

- **WHEN** `cmd/import-yc` imports a directory entry with `subindustry`
  `"Industrials -> Manufacturing and Robotics"`
- **THEN** that company's `companies.subindustry` is `"Manufacturing and Robotics"` and its
  `companies.industries` display array is unchanged

#### Scenario: A non-YC company has no subindustry

- **WHEN** a company has no matching YC directory entry
- **THEN** its `companies.subindustry` is `NULL`

### Requirement: The subindustry facet vocabulary is served dynamically with counts

The system SHALL expose `GET /api/v1/companies/subindustries` returning, under `data`, the
distinct non-`NULL` `companies.subindustry` values each with the count of companies carrying it,
ordered by count descending then value ascending. This serves the searchable option list for the
subindustry facet; the counts are unconditional (they do not reflect other active filters — a
deliberate simplification versus the Meilisearch job facets). Each item SHALL carry `value` and
`count`.

#### Scenario: Listing available subindustries with counts

- **WHEN** a client requests `GET /api/v1/companies/subindustries`
- **THEN** the response lists every distinct non-`NULL` subindustry with its company count,
  most common first

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

The endpoint SHALL additionally accept the repeatable **scalar** facet parameters
`maturity` and `subindustries`, each filtering against a single-valued company
column (`companies.maturity` / `companies.subindustry`) by **membership**: a company
matches when its scalar value is among the requested values (OR within the facet),
and each ANDs with the others and with `q` exactly like the array facets. A company
whose column is `NULL` matches no value for that facet. `maturity` values are
`government`, `startup`, `scaleup`, `enterprise`; `subindustries` values are the
YC subindustry leaves served by `GET /api/v1/companies/subindustries`.

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

#### Scenario: Filtering by the scalar subindustry facet

- **WHEN** a client requests
  `GET /api/v1/companies?subindustries=Payments&subindustries=Diagnostics`
- **THEN** the response contains only companies whose `subindustry` is `Payments`
  **or** `Diagnostics`, excluding any company whose `subindustry` is `NULL`, and
  `meta.total` is the count of such companies
