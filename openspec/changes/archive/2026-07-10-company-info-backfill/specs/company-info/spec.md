## ADDED Requirements

### Requirement: Companies carry authoritative company-info attributes

The system SHALL store authoritative company-info attributes on the `companies`
entity, independent of the job-derived facet arrays: `industries` (text array),
`year_founded` (integer, nullable), `employee_count` (integer, nullable),
`hq_country` (ISO 3166-1 alpha-2, nullable), `organization_type` (nullable),
`tagline` (nullable), and a `company_info` JSONB holding lower-coverage extras
(homepage, funding, stock listing, parent company, subsidiaries, activities). A
`company_info_at` timestamp SHALL record when the attributes were last written.

These attributes SHALL be independent of the job-derived facet columns
(`company_types`, `company_sizes`, `countries`, `domains`, `regions`): the
periodic facet recompute SHALL NOT read or write the company-info attributes, and
the company info backfill SHALL NOT read or write the job-derived facets or
`job_count`.

An attribute that is unknown in the source SHALL be stored as NULL (or omitted
from the JSONB), so "unknown" stays distinguishable from a real value.

#### Scenario: Company-info attributes persist on the company

- **WHEN** a company is enriched with company-info attributes
- **THEN** its `industries`, `year_founded`, `employee_count`, `hq_country`,
  `organization_type`, `tagline`, and `company_info` JSONB are stored and
  `company_info_at` is set

#### Scenario: Facet recompute does not disturb company info

- **WHEN** the periodic company facet recompute runs over a company that has
  company-info attributes
- **THEN** the company-info attributes are unchanged

### Requirement: Company info are loaded by a one-time backfill matched by slug

The system SHALL provide a run-once host worker that streams a local dataset file
(a path passed as an argument) of company company info and applies each record to
the `companies` table, matching by the normalized-name `slug`. For a record whose
slug matches an existing company, the worker SHALL update only that company's
company-info attributes, leaving `job_count`, `collections`, and the job-derived
facets untouched. For a record whose slug matches no company, the worker SHALL
insert a new company row as a reference row (`is_reference = true`) with the
company-info attributes and no jobs. The worker SHALL be idempotent: re-running the
same dataset SHALL produce the same company-info values. The worker and schema
SHALL NOT reference the dataset's origin.

#### Scenario: Existing company is enriched

- **WHEN** the backfill processes a record whose slug matches an existing company
- **THEN** the company's company-info attributes are updated and its `job_count`,
  `collections`, and job-derived facets are unchanged

#### Scenario: Unmatched company is imported as a reference row

- **WHEN** the backfill processes a record whose slug matches no existing company
- **THEN** a new `companies` row is inserted with `is_reference = true`,
  `job_count = 0`, the display name, and the company-info attributes

#### Scenario: Re-running the backfill is idempotent

- **WHEN** the backfill is run twice over the same dataset file
- **THEN** the second run produces the same company-info values with no duplicate
  rows

#### Scenario: Origin is not recorded

- **WHEN** the backfill writes a company's company info
- **THEN** no field, column, or log names the dataset's source
