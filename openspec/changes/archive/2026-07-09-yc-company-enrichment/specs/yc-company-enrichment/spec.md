## ADDED Requirements

### Requirement: yc-oss entries map to company-info fields

The system SHALL provide a pure mapping from a yc-oss directory entry
(`yc-oss.github.io/api/companies/all.json`) to company-info fields: `one_liner` →
tagline, `long_description` → a `company_info.description`, `industry` plus `tags`
(de-duplicated) → industries, `team_size` → employee count, `launched_at` → founding
year, and `all_locations` → an HQ country resolved via `internal/location` (the
first resolved country, or none). The mapping SHALL also carry `batch` and `status`
through, and place `website`, `stage`, `top_company`, `isHiring`, the YC profile
url, and the logo into `company_info`. An entry with a blank name SHALL be skipped.

#### Scenario: A directory entry maps to the expected fields

- **WHEN** the mapper is given an entry with `name`, `one_liner`, `long_description`,
  `team_size`, `launched_at`, `industry`, `tags`, `all_locations`, `batch`, `status`
- **THEN** it yields a record whose tagline is the `one_liner`, whose industries
  include the `industry` and each `tag` without duplicates, whose employee count is
  `team_size`, whose founding year is derived from `launched_at`, whose HQ country
  is resolved from `all_locations`, and whose `batch`/`status` are carried through

#### Scenario: Missing optional fields become absent, not empty sentinels

- **WHEN** the mapper is given an entry with an empty `long_description`, unknown
  `all_locations`, and zero `team_size`
- **THEN** the record omits the description, leaves HQ country unset, and leaves
  employee count unset (no zero/empty placeholder)

### Requirement: The YC directory importer enriches existing companies and adds the rest

The system SHALL provide a run-once worker (`cmd/import-yc`) that fetches the
yc-oss directory, maps each entry, and upserts by `normalize.Slug(name)`: an
existing company has its company-info columns and `yc_batch`/`yc_status` refreshed,
and an unmatched entry is inserted as a reference row (`is_reference = true`) with
no jobs, so the full YC directory is held. The upsert SHALL NOT modify a company's
`job_count`, `collections`, or job-derived facet arrays. The worker SHALL be
idempotent — re-running rewrites the same values — and SHALL report matched vs
inserted counts.

#### Scenario: Existing company is enriched

- **WHEN** the worker processes an entry whose normalized name matches a company row
- **THEN** that company's tagline, industries, employee count, founding year, HQ
  country, `company_info.description`, `yc_batch`, and `yc_status` are set, and its
  `job_count`/`collections`/job-derived facets are unchanged

#### Scenario: Unmatched entry is inserted as a reference row

- **WHEN** the worker processes an entry whose normalized name matches no company
- **THEN** a `companies` row is inserted with `is_reference = true`, carrying the
  mapped company-info and yc facets, and `job_count = 0`

#### Scenario: Re-running is idempotent

- **WHEN** the worker runs twice over the same directory
- **THEN** the second run writes the same company-info and yc facet values as the first

### Requirement: yc_batch and yc_status are curated facets exempt from the recompute

The system SHALL store `companies.yc_batch` and `companies.yc_status` (each a
`TEXT[]`) as curated facets loaded by the YC importer, holding the company's YC
batch (e.g. `Winter 2012`) and status (e.g. `Active`). These SHALL be filterable by
array overlap on the company list endpoint. The periodic facet recompute
(`RefreshCompanyFacets` / `cmd/recount-companies`) SHALL NOT read or write these
columns, so an imported value survives every recompute. A company never touched by
the importer SHALL have both empty (`'{}'`).

#### Scenario: Recompute leaves the YC facets untouched

- **WHEN** a company has imported `yc_batch` and `yc_status` values and the facet
  recompute runs
- **THEN** both columns are unchanged, regardless of the company's jobs

#### Scenario: A non-YC company has empty YC facets

- **WHEN** a company the importer never matched is read
- **THEN** its `yc_batch` and `yc_status` are both the empty array
