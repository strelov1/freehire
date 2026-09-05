## ADDED Requirements

### Requirement: An employment's period is a structured date, not free text
Each employment's start and end period boundary SHALL be stored as a structured
year/(optional month) value rather than a free-form string, plus the existing
`is_current` flag for an ongoing role (unchanged — an ongoing role has no end
date, and `is_current` is never inferred or overwritten by import once a role
has ended). Listing an owner's employments SHALL sort chronologically using
these structured values directly (a native database ordering), not by
re-sorting a lexicographically-ordered free-text column in application code
afterward.

#### Scenario: Employments with mixed date precision sort chronologically
- **WHEN** an owner's employments include one with only a year ("2024") and
  others with a month and year ("October 2018", "Mar 2021")
- **THEN** listing the owner's employments orders them by actual chronological
  date, not by the lexicographic order of their original text

#### Scenario: A year-only employment keeps its precision
- **WHEN** a candidate records an employment with only a start year, no month
- **THEN** the stored period carries that year with no month, and it is not
  displayed or exported as if a specific month were known

#### Scenario: An ongoing employment has no end date
- **WHEN** a candidate marks an employment as current (`is_current`)
- **THEN** its end period is absent, and listing treats it as ongoing for
  sorting purposes without requiring a fabricated end date
