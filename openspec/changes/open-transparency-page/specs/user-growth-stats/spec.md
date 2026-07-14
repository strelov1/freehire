## ADDED Requirements

### Requirement: Public member-growth series
The system SHALL expose an unauthenticated `GET /api/v1/stats/user-growth` endpoint
returning the cumulative count of registered members over time as a per-day series,
computed directly from `users.created_at` with no rollup table or worker.

#### Scenario: Cumulative series returned
- **WHEN** a client requests `GET /api/v1/stats/user-growth`
- **THEN** the response is `{"data": [...]}` where each item is `{ "date": "YYYY-MM-DD", "total": <cumulative member count as of that day> }`
- **AND** `total` is monotonically non-decreasing across the series

#### Scenario: No authentication required
- **WHEN** the endpoint is requested without a session cookie or API key
- **THEN** the request succeeds (it is a public read, like `/stats/jobs-activity`)

#### Scenario: Aggregate only, no PII
- **WHEN** the series is produced
- **THEN** it contains only dates and counts — never any user identifier, email, or other personal field

#### Scenario: Empty catalogue
- **WHEN** there are no registered members
- **THEN** the endpoint returns `{"data": []}` with a 200 status, not an error
