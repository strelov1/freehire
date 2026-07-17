## ADDED Requirements

### Requirement: Precomputed facet-distribution snapshot
The system SHALL maintain a precomputed snapshot of the value→count distribution
for a fixed set of job facets — `countries`, `skills`, `seniority`, and
`work_mode` — recomputed by a run-once-and-exit worker so that reads never
trigger a live search-engine facet count.

#### Scenario: Snapshot holds the selected facets
- **WHEN** the rollup worker has run
- **THEN** the snapshot contains a value→count distribution for each of
  `countries`, `skills`, `seniority`, and `work_mode`, and no other facets

#### Scenario: Distribution matches the live catalogue
- **WHEN** the worker recomputes the snapshot
- **THEN** each facet's counts are those the search engine reports for that
  attribute over the full catalogue, using the same facet vocabulary as the live
  filter facets

### Requirement: Atomic recompute with idempotent reruns
The system SHALL recompute the snapshot inside a single transaction that clears
and rebuilds it atomically, so a reader never observes a partial rebuild and
rerunning the worker is safe.

#### Scenario: Reader never sees a partial rebuild
- **WHEN** the worker is midway through rebuilding the snapshot
- **THEN** a concurrent reader still sees the previous complete snapshot until
  the transaction commits

#### Scenario: Recompute is idempotent
- **WHEN** the worker runs twice in succession with no catalogue change between
  runs
- **THEN** the resulting snapshot is identical after each run

#### Scenario: Failed rebuild leaves the prior snapshot intact
- **WHEN** the recompute transaction fails before commit
- **THEN** the snapshot retains its previous contents and the worker exits
  non-zero

### Requirement: Public aggregate facets endpoint
The system SHALL expose a public, unauthenticated `GET /api/v1/stats/facets`
endpoint that returns the snapshot as `{"data": {"facets": {<facet>: {<value>:
<count>}}}}`, keyed by public facet param names, exposing only aggregate counts.

#### Scenario: Endpoint returns the snapshot
- **WHEN** an anonymous client requests `GET /api/v1/stats/facets` after the
  worker has populated the snapshot
- **THEN** the response is 200 with a `data.facets` object holding the
  distributions for `countries`, `skills`, `seniority`, and `work_mode`

#### Scenario: Empty snapshot degrades gracefully
- **WHEN** the snapshot has not yet been populated
- **THEN** the endpoint responds 200 with empty facet maps rather than an error

#### Scenario: Only aggregate counts are exposed
- **WHEN** any client reads the endpoint
- **THEN** the response contains only per-value counts and never any
  record-level or per-user data
