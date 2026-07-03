## MODIFIED Requirements

### Requirement: Companies are stored as a slug-keyed entity

The system SHALL store companies in a `companies` table identified by a natural
`slug` key derived by normalizing the company name. The table SHALL NOT use a
surrogate id. Each company SHALL have a display `name`.

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

#### Scenario: Reference company survives orphan cleanup

- **WHEN** the orphan cleanup runs and a company row has no job referencing it
  but is marked `is_reference = true`
- **THEN** the row is not deleted

#### Scenario: A job adopts a reference company

- **WHEN** a job is ingested whose normalized company name matches the `slug` of
  a reference row
- **THEN** the existing row is reused, its job count reflects the new job, and
  its company-info fields are unchanged
