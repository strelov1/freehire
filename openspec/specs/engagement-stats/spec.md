# engagement-stats Specification

## Purpose
TBD - created by archiving change open-engagement-stats. Update Purpose after archive.
## Requirements
### Requirement: Public engagement counts
The system SHALL expose an unauthenticated `GET /api/v1/stats/engagement` endpoint
returning aggregate interaction counts from `user_jobs` — jobs saved, applications
marked, and jobs viewed — computed directly from the table with no rollup.

#### Scenario: Counts returned
- **WHEN** a client requests `GET /api/v1/stats/engagement`
- **THEN** the response is `{"data": {"saved": <n>, "applied": <n>, "viewed": <n>}}` with each count the number of `user_jobs` rows whose respective timestamp is set

#### Scenario: No authentication required
- **WHEN** the endpoint is requested without a session cookie or API key
- **THEN** the request succeeds (a public read, like the other `/stats/*` endpoints)

#### Scenario: Aggregate only, no PII
- **WHEN** the counts are produced
- **THEN** the response contains only integer totals — never any user identifier, job id, or other row-level field

#### Scenario: Empty table
- **WHEN** there are no interactions
- **THEN** the endpoint returns `{"data": {"saved": 0, "applied": 0, "viewed": 0}}` with a 200 status

