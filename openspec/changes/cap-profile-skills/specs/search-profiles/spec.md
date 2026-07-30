## MODIFIED Requirements

### Requirement: Skills validation

A profile's `skills` set SHALL be non-empty after normalization, and neither `skills` nor
`excluded_skills` SHALL exceed 200 entries after normalization.

The bound exists because the set is expanded per element into the coverage verdict's search
filter (one `skills != "<skill>"` AND group each), so a stored list is a multiplier on every
later read of the profile against the index that also serves public search. A set past the
bound SHALL be rejected with `400` and nothing stored, exactly as a specialization set past
its own cap is.

A single skill longer than 64 characters SHALL be dropped from the set rather than rejecting
the save, the same treatment blanks and duplicates receive: a per-value problem does not fail
an otherwise valid save, and no canonical skill the dictionary emits approaches that length.

#### Scenario: Empty skills rejected
- **WHEN** an authenticated user saves a profile whose `skills` are absent, empty, or reduce to empty after trimming
- **THEN** the system responds `400` and stores nothing

#### Scenario: Too many skills rejected
- **WHEN** an authenticated user saves a profile with more than 200 distinct skills, in either the wanted or the avoided set
- **THEN** the system responds `400` and stores nothing

#### Scenario: A skill set at the bound is stored whole
- **WHEN** an authenticated user saves a profile with exactly 200 distinct skills
- **THEN** the profile is stored with all 200

#### Scenario: An over-long value is dropped, not rejected
- **WHEN** an authenticated user saves a profile whose skills include a value longer than 64 characters alongside valid ones
- **THEN** the profile is stored with the over-long value omitted and the valid skills kept
