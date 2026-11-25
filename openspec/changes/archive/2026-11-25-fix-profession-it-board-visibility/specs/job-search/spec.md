## ADDED Requirements

### Requirement: Category-unresolved jobs are excluded from the index, with a confirmed-technical carve-out

The system SHALL exclude a job from the searchable index (both the incremental
push and the full rebuild) when its category is unresolved by both the
deterministic title dictionary and enrichment (empty or the catch-all
`"other"`) — this keeps the undifferentiated non-tech bulk a broad ATS crawl
brings in (painters, stockers, drivers, and similarly unfiltered roles) out of
a catalogue no category filter, and often no keyword search, was ever meant to
surface. This exclusion SHALL NOT apply when the job's `is_tech` signal is
confidently `true`: a confirmed-technical posting carries meaningful signal
even without a resolved sub-category, so it is not the undifferentiated bulk
the exclusion targets, and it SHALL remain searchable regardless of whether
its category ever resolves further.

#### Scenario: A job with no resolved category and no tech signal is excluded
- **WHEN** a job's category is empty, its enrichment carries no category or
  `"other"`, and its `is_tech` is unknown or `false`
- **THEN** the job is excluded from the search index

#### Scenario: A confirmed-technical job with no resolved category remains searchable
- **WHEN** a job's category is empty, its enrichment carries no category or
  `"other"`, but its `is_tech` is `true`
- **THEN** the job is included in the search index

#### Scenario: A resolved category always includes the job regardless of is_tech
- **WHEN** a job's category (from the title dictionary or enrichment) is
  resolved to a specific, non-`"other"` value
- **THEN** the job is included in the search index
