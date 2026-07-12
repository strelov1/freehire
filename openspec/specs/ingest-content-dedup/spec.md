# ingest-content-dedup Specification

## Purpose
TBD - created by archiving change ingest-content-dedup. Update Purpose after archive.
## Requirements
### Requirement: One canonical job per role cluster

The system SHALL designate exactly one **canonical** open job per role cluster
(`company_slug` + `role_fingerprint`), reusing the existing `role_fingerprint`. The
canonical row is chosen deterministically — the `min(id)` among the cluster's open
rows — so the choice is stable across recomputes. Non-canonical open rows in a cluster
carry a `duplicate_of` reference to their canonical job; the canonical row and any
row with an empty fingerprint carry no reference. Rows are never deleted or
un-inserted, so the job-reality repost/mass-posting counts are unaffected.

#### Scenario: A cluster resolves to one canonical row

- **WHEN** a company has several open jobs sharing one `role_fingerprint`
- **THEN** exactly one (the `min(id)`) is canonical and the rest reference it via
  `duplicate_of`

#### Scenario: Unfingerprinted and singleton rows stay canonical

- **WHEN** an open job has an empty `role_fingerprint`, or is the only open row in its
  cluster
- **THEN** it is canonical (`duplicate_of` is null)

#### Scenario: Canon fails over when the canonical closes

- **WHEN** the canonical row of a cluster is closed and the recompute runs
- **THEN** the new `min(id)` among the remaining open rows becomes canonical

### Requirement: Non-canonical reposts are hidden from catalogue and search

The jobs list and the search index SHALL exclude non-canonical reposts, so a role
cluster appears once. A job addressed directly by its slug is still served (like a
closed job) so existing links do not break.

#### Scenario: List returns one row per cluster

- **WHEN** the jobs list is queried and a cluster has a canonical row plus reposts
- **THEN** only the canonical row is returned

#### Scenario: Search omits reposts

- **WHEN** the search index is built or incrementally pushed
- **THEN** rows with a non-null `duplicate_of` are not indexed

#### Scenario: A repost is still reachable by slug

- **WHEN** a non-canonical repost is requested by its public slug
- **THEN** the detail read still serves it

### Requirement: Enrichment skips non-canonical reposts

The enrichment enqueue SHALL exclude non-canonical reposts, so duplicate postings do
not consume LLM budget; only the canonical row of a cluster is enriched.

#### Scenario: Only the canonical row is enqueued

- **WHEN** the enrichment enqueue runs over open jobs
- **THEN** a job with a non-null `duplicate_of` is not enqueued

### Requirement: The canonical job surfaces its openings count

The canonical job SHALL surface how many open postings its cluster holds (the existing
role-cluster open count), so a collapsed cluster communicates "N openings" rather than
hiding that N postings exist.

#### Scenario: Canon reports its cluster's open count

- **WHEN** the canonical job of a cluster with N open postings is read
- **THEN** its openings count is N

