## ADDED Requirements

### Requirement: A mentor may state their seniority level

A profile MAY carry a seniority level, drawn from the platform's existing closed
seniority vocabulary. A submission or edit naming a value outside that vocabulary SHALL
be refused. A profile that leaves it unset SHALL still be created or updated normally,
and SHALL simply not match a directory search narrowed by seniority.

#### Scenario: A profile states its seniority

- **WHEN** a mentor submits or edits a profile with a seniority value from the platform's
  seniority vocabulary
- **THEN** the profile is created or updated with that value

#### Scenario: An unrecognised seniority value is refused

- **WHEN** a mentor submits or edits a profile with a seniority value outside the
  platform's seniority vocabulary
- **THEN** the system refuses with a validation error
- **AND** no profile is created or changed

#### Scenario: Seniority is optional

- **WHEN** a mentor submits a profile with no seniority stated
- **THEN** the profile is created normally
- **AND** it does not match a directory search narrowed by seniority

## MODIFIED Requirements

### Requirement: The public directory lists approved mentors and can be narrowed

The system SHALL serve a public directory of approved, unpaused mentor profiles, and
SHALL allow it to be narrowed by company, by topic, by language, by seniority, by a
free-text match against a mentor's name or headline, and to mentors with no reviews yet.
Following the project's dropped-filter rule, the directory SHALL report any query
parameter it did not read in `meta.ignored_params`, and SHALL omit that key when there
are none.

#### Scenario: The directory excludes profiles that are not publishable

- **WHEN** a visitor lists the mentor directory
- **THEN** only profiles whose status is `approved` and which are not paused appear

#### Scenario: An unrecognised filter is reported, not silently ignored

- **WHEN** a visitor requests the directory with a parameter the endpoint does not read
- **THEN** the results are unnarrowed by it
- **AND** `meta.ignored_params` names that parameter

#### Scenario: A free-text search matches name or headline

- **WHEN** a visitor narrows the directory with a text query
- **THEN** only mentors whose name or headline contains that text, case-insensitively,
  appear

#### Scenario: Narrowing by seniority

- **WHEN** a visitor narrows the directory by a seniority value
- **THEN** only mentors who stated that exact seniority appear
- **AND** a mentor who left seniority unset does not appear

#### Scenario: Narrowing to mentors with no reviews yet

- **WHEN** a visitor narrows the directory to mentors with no reviews yet
- **THEN** only mentors whose review count is zero appear
