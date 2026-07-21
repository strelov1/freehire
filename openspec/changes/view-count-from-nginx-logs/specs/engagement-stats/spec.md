## MODIFIED Requirements

### Requirement: Public engagement counts
The system SHALL expose an unauthenticated `GET /api/v1/stats/engagement` endpoint
returning aggregate interaction counts. `saved` and `applied` are computed
directly from `user_jobs` (rows whose respective timestamp is set). `viewed` is
the total job views across all traffic, computed as `SUM(jobs.view_count)` so it
reflects anonymous, signed-in, and API views (see `view-count-aggregation` and
`job-engagement-counts`). No rollup precomputation is required.

#### Scenario: Counts returned
- **WHEN** a client requests `GET /api/v1/stats/engagement`
- **THEN** the response is `{"data": {"saved": <n>, "applied": <n>, "viewed": <n>}}` where `saved` and `applied` are counts of `user_jobs` rows with the respective timestamp set, and `viewed` is `SUM(jobs.view_count)`

#### Scenario: viewed reflects all traffic
- **WHEN** views have been aggregated from nginx logs into `jobs.view_count`
- **THEN** the `viewed` figure includes those anonymous and API views, not only signed-in interactions

#### Scenario: No authentication required
- **WHEN** the endpoint is requested without a session cookie or API key
- **THEN** the request succeeds (a public read, like the other `/stats/*` endpoints)

#### Scenario: Aggregate only, no PII
- **WHEN** the counts are produced
- **THEN** the response contains only integer totals — never any user identifier, job id, or other row-level field

#### Scenario: Empty database
- **WHEN** there are no interactions and no views
- **THEN** the endpoint returns `{"data": {"saved": 0, "applied": 0, "viewed": 0}}` with a 200 status
