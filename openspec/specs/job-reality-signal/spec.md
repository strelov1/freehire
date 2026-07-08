# job-reality-signal Specification

## Purpose
TBD - created by archiving change job-reality-signal. Update Purpose after archive.
## Requirements
### Requirement: Each job carries a deterministic reality classification
The system SHALL classify every job into exactly one reality class — `fresh`, `stale`, or `likely-evergreen` — computed deterministically from ingest history and posting text, with no randomness and no LLM call.

#### Scenario: A newly first-seen posting is fresh
- **WHEN** a job's `created_at` is recent (within the fresh window) and no evergreen signals apply
- **THEN** its reality class is `fresh`

#### Scenario: A long-open posting with no other signals is stale
- **WHEN** a job has been continuously open well beyond the fresh window but no convergence of evergreen signals is present
- **THEN** its reality class is `stale`, not `likely-evergreen`

#### Scenario: The classification is stable for identical input
- **WHEN** the same job row and clock are classified twice
- **THEN** the resulting class and evidence are identical

### Requirement: Likely-evergreen requires convergence, never a single signal
The system SHALL assign `likely-evergreen` only when multiple independent signals converge, and MUST NOT assign it on age alone, so a genuinely hard-to-fill senior role open a long time is not mislabeled.

#### Scenario: Age alone does not trigger the verdict
- **WHEN** a job has been open 240 days but has no reposts, no mass-posting, and no evergreen text markers
- **THEN** its reality class is `stale`, not `likely-evergreen`

#### Scenario: Convergent signals trigger the verdict
- **WHEN** a job is long-open AND its role has been reposted multiple times AND the description carries evergreen text markers
- **THEN** its reality class is `likely-evergreen`

### Requirement: True age is measured from first-seen, not the source posted date
The system SHALL derive a job's age from `created_at` (when freehire first saw the `(source, external_id)`), so a posting whose `posted_at` is refreshed to look new is still measured from when it actually first appeared.

#### Scenario: A reposted-fresh date does not reset perceived age
- **WHEN** a job's `posted_at` is recent but its `created_at` is far older
- **THEN** the reality signal treats the job as old and records fake-freshness evidence

### Requirement: Repost clustering counts distinct postings of one role
The system SHALL count how many distinct `external_id`s share one job's `role_fingerprint` within the same company, and expose that count as repost evidence.

#### Scenario: Repeated reposts under new ids are counted
- **WHEN** a company has published six distinct `external_id`s that share one role fingerprint
- **THEN** each such job's evidence reports a repost count of six

#### Scenario: A unique role has no repost signal
- **WHEN** a job's role fingerprint is shared by no other posting in the company
- **THEN** its repost count is one and contributes no evergreen signal

### Requirement: Evergreen text markers come from a curated dictionary that never guesses
The system SHALL detect evergreen phrasing ("always hiring", "talent community", "building a pipeline", and curated RU equivalents) via a deterministic dictionary, and MUST emit no marker for text it cannot match.

#### Scenario: A known evergreen phrase is detected
- **WHEN** a description contains a curated evergreen phrase
- **THEN** the evergreen-text signal is set

#### Scenario: Unmatched text yields no marker
- **WHEN** a description contains no curated phrase
- **THEN** no evergreen-text signal is emitted (the dictionary never infers)

### Requirement: The reality signal is served dict-only and exposed as a search facet
The system SHALL serve the reality class as a top-level job-view field and index it as a filterable Meilisearch `reality` facet, computed from the deterministic signal alone and never from LLM enrichment.

#### Scenario: The facet is filterable but not hidden by default
- **WHEN** a client requests jobs without a reality filter
- **THEN** all classes are returned, and the client MAY filter by `reality` to include or exclude a class

#### Scenario: Existing jobs are reconciled through reindex
- **WHEN** the reality dictionary or thresholds change
- **THEN** `make reindex` recomputes the class at index time on existing jobs (the class is computed at index, not stored, so no `backfill-derive` step is needed for it)

### Requirement: The job view surfaces facts-backed evidence, not a bare verdict
The system SHALL accompany a non-`fresh` classification with the observable facts that produced it — age in days, repost count, and mass-posting count — so the UI states facts ("open 240 days · reposted 6×") rather than an unsupported accusation.

#### Scenario: Evidence travels with the classification
- **WHEN** a job is classified `likely-evergreen`
- **THEN** its served payload includes the age, repost count, and mass-posting count that justify the class

