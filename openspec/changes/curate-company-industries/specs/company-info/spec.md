## MODIFIED Requirements

### Requirement: Companies carry authoritative company-info attributes

The system SHALL store authoritative company-info attributes on the `companies`
entity, independent of the job-derived facet arrays: `industries` (text array,
holding only canonical values from the curated industry vocabulary),
`year_founded` (integer, nullable), `employee_count` (integer, nullable),
`hq_country` (ISO 3166-1 alpha-2, nullable), `organization_type` (nullable),
`tagline` (nullable), and a `company_info` JSONB holding lower-coverage extras
(homepage, funding, stock listing, parent company, subsidiaries, activities). A
`company_info_at` timestamp SHALL record when the attributes were last written.

Every writer of `industries` SHALL resolve its values through the curated
dictionary before storing them, and SHALL store nothing for a label the dictionary
does not know.

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

#### Scenario: A stored industry is always canonical

- **WHEN** any writer stores industries for a company
- **THEN** every stored value is a canonical value of the curated vocabulary, and
  labels outside it are absent rather than stored verbatim

## ADDED Requirements

### Requirement: Company-info writes fill gaps rather than overwrite other sources

A writer of company-info attributes SHALL NOT replace a value another source has
already stored. `tagline` SHALL be written only when the stored value is NULL or
empty. `company_info` JSONB SHALL be merged key-wise, with the stored value
winning any key collision. `industries` SHALL be unioned with the stored values,
de-duplicated and sorted.

More than one source now writes these columns, so a wholesale replacement by any
one of them destroys the others' work on its next run.

#### Scenario: An existing tagline survives a later import

- **WHEN** a source imports a company that already has a non-empty tagline
- **THEN** the stored tagline is unchanged and the imported one is discarded

#### Scenario: JSONB keys fill gaps without overwriting

- **WHEN** a source imports `company_info` keys for a company that already stores
  some of them
- **THEN** keys absent from the stored JSONB are added, and keys present in it keep
  their stored values

#### Scenario: Industries accumulate across sources

- **WHEN** a source imports industries for a company that already stores others
- **THEN** the stored value is the sorted, de-duplicated union of both

### Requirement: A run-once worker normalizes and merges company industries

The system SHALL provide a run-once worker that rewrites every company's stored
industries through the curated dictionary, and that optionally merges an external
company dump supplied as a file path argument.

The normalization pass SHALL drop stored values outside the dictionary, making the
column dict-only. Because that is destructive, the worker SHALL be run only after
the affected column has been backed up.

The dump SHALL be matched to companies by both its own record slug and a slug
derived from the record's company name, because dump slugs and our slugs are
derived differently and neither key alone matches enough rows. Where two dump
records collide on one key, the record with more live jobs SHALL win.

The worker SHALL be idempotent: a second run over the same inputs SHALL rewrite
nothing. The worker and its queries SHALL NOT reference the dump's origin.

#### Scenario: Stored values outside the dictionary are dropped

- **WHEN** the normalization pass runs over a company whose stored industries
  include a value the dictionary does not know
- **THEN** that value is removed and the remaining values are stored canonically

#### Scenario: A dump record matches by either key

- **WHEN** a dump record's own slug matches no company but a slug derived from its
  company name does
- **THEN** that company receives the record's industries

#### Scenario: Re-running changes nothing

- **WHEN** the worker runs twice over the same dictionary and the same dump
- **THEN** the second run reports no rewritten rows
