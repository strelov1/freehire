# company-info Specification

## Purpose
TBD - created by archiving change company-info-backfill. Update Purpose after archive.
## Requirements
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

### Requirement: A run-once worker fills tagline gaps by matching companies to Wikipedia

The system SHALL provide a run-once host worker that, for every company whose
stored `tagline` is NULL or empty, looks up the company by its display name
against Wikipedia and, when a match passes the confidence gate defined below,
writes a `tagline` and a `company_info` summary field derived from that
match's own summary text, and sets `company_info_at`.

A company that already has a non-empty `tagline` (written by this worker or
any other company-info source) SHALL NOT be looked up or modified.

The worker SHALL NOT read or write `job_count`, `collections`, or any
job-derived facet column (`company_types`, `company_sizes`, `countries`,
`domains`, `regions`).

#### Scenario: A company with no tagline receives one from a confident match

- **WHEN** the worker processes a company with an empty `tagline` and its name
  resolves to a Wikipedia article confidently typed as a company or
  organization
- **THEN** the company's `tagline` and `company_info` summary field are set
  from that article, and `company_info_at` is updated

#### Scenario: A company that already has a tagline is left untouched

- **WHEN** the worker encounters a company whose `tagline` is already
  non-empty
- **THEN** it performs no lookup and makes no write for that company

#### Scenario: Job-derived facets are never touched

- **WHEN** the worker fills a company's `tagline`
- **THEN** the company's `job_count`, `collections`, and job-derived facet
  columns are unchanged

### Requirement: A match is accepted only when confidently typed as a company or organization

A candidate Wikipedia/Wikidata match SHALL be accepted only when the matched
entity's own type classification places it within a curated set of
business/organization types (for example: company, corporation, public
company, business, bank, or a narrower subtype of one of these). A match
whose type falls outside that set, or whose type cannot be determined, SHALL
be rejected, and the worker SHALL make no write for that company.

The type check SHALL NOT rely on keyword-matching the entity's
natural-language description or summary text, because that approach both
rejects valid matches whose description omits a recognized keyword and
accepts invalid matches whose description happens to contain one.

#### Scenario: A same-named person, place, or concept is rejected

- **WHEN** a company's name resolves to a Wikipedia article whose subject is
  typed as a person, a place, a military unit, an abstract concept, or a
  disambiguation page
- **THEN** the worker rejects the match and writes nothing for that company

#### Scenario: A valid company match is accepted regardless of description wording

- **WHEN** a company's name resolves to an article typed as a business or
  organization, even if its description text uses wording outside any fixed
  keyword list (for example "defense contractor" or "electrical contractor")
- **THEN** the worker accepts the match and fills the company's `tagline`

### Requirement: The backfill is idempotent and safe to interrupt or re-run

Re-running the worker over the same company data and the same Wikipedia
content SHALL produce the same stored `tagline`/`company_info` values, with
no duplicate rows and no changes to companies already filled by this or any
other source. Stopping the worker partway through a run SHALL leave already
-written companies unaffected and lose no completed work.

#### Scenario: Re-running after a full pass changes nothing

- **WHEN** the worker is run a second time after a first run has already
  filled the eligible companies it could confidently match
- **THEN** the second run performs no lookups against companies that already
  have a `tagline` and writes nothing

#### Scenario: An interrupted run resumes without redoing completed writes

- **WHEN** the worker is stopped after filling some companies and started
  again
- **THEN** it does not re-write the companies already filled, and continues
  with the companies still missing a `tagline`
