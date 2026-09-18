## MODIFIED Requirements

### Requirement: Company-info writes fill gaps rather than overwrite other sources

A writer of company-info attributes SHALL NOT replace a value another source has
already stored. `tagline` SHALL be written only when the stored value is NULL or
empty. `company_info` JSONB SHALL be merged key-wise, with the stored value
winning any key collision. `industries` SHALL be unioned with the stored values,
de-duplicated and sorted.

More than one source now writes these columns, so a wholesale replacement by any
one of them destroys the others' work on its next run.

The one exception is a verified employer editing their own company's curated fields
(`tagline`, `company_info`, `industries`, `year_founded`, `employee_count`, `hq_country`,
`subindustry`, see `employer-account`): those writes SHALL be authoritative and SHALL
overwrite any existing stored value, because the employer is the company itself and is a
more authoritative source than an imported or inferred one.

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

#### Scenario: A verified employer's edit overwrites an existing curated value

- **WHEN** an active employer edits their own company's `tagline`, `company_info` key,
  `industries`, `year_founded`, `employee_count`, `hq_country`, or `subindustry`, and a
  value was already stored for it by another source
- **THEN** the stored value is replaced with the employer's, not merged with or protected
  from it

## ADDED Requirements

### Requirement: A curated field asserted by a verified employer is protected from importer overwrite

Once a company has an active employer account, any run-once or periodic company-info
importer (for example the YC-directory import) SHALL NOT overwrite that company's
`year_founded`, `employee_count`, `hq_country`, or `subindustry`, even when its own writer
would ordinarily replace those fields unconditionally. This protection applies in addition
to, not in place of, the existing fill-gap protection already in force for `tagline` and
`company_info`.

#### Scenario: A YC-directory import does not clobber an employer-asserted founding year

- **WHEN** the YC-directory import processes a company that has an active employer account
  and a stored `year_founded` the employer set
- **THEN** the company's `year_founded` is left unchanged by that import run

#### Scenario: A company with no employer account is unaffected

- **WHEN** the YC-directory import processes a company with no employer account
- **THEN** its `year_founded`, `employee_count`, `hq_country`, and `subindustry` are
  overwritten as before
