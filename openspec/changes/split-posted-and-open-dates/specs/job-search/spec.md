## ADDED Requirements

### Requirement: Search bounds results by how long a posting has been open

`GET /api/v1/jobs/search` SHALL accept an `open_within_days` parameter. When it is a
positive integer `N`, the search SHALL be restricted to jobs whose `created_ts` is at
or after `now - N*86400`, where `created_ts` is the unix-seconds encoding of the
posting's `created_at` (the instant ingest first wrote the row) and `now` is the time
the request is served. When the parameter is absent, empty, zero, negative, or not a
valid integer, it SHALL impose no restriction.

The bound SHALL compose with every other filter, including `posted_within_days`, as a
conjunction: a request carrying both SHALL return only jobs satisfying both.

This bound is distinct from `posted_within_days` on purpose. `posted_within_days`
filters the date the SOURCE states, which some boards rewrite on every crawl;
`open_within_days` filters the date the system observed, which no source can rewrite.

The search document SHALL carry `created_ts` as a filterable numeric attribute, set
for every indexed job. It SHALL NOT appear in the served job payload — `created_at` is
already served as an RFC3339 string, and `created_ts` exists only because Meilisearch
range operators require a number.

#### Scenario: Open-within filter restricts to recently first-seen postings

- **WHEN** a client requests `GET /api/v1/jobs/search?open_within_days=7`
- **THEN** only jobs first recorded within the last 7 days are returned

#### Scenario: A rewritten posting date does not defeat the open-within bound

- **WHEN** a job was first recorded 72 days ago but its source states a posting date of
  today, and a client requests `GET /api/v1/jobs/search?open_within_days=3`
- **THEN** that job is not returned

#### Scenario: The two date bounds compose

- **WHEN** a client requests
  `GET /api/v1/jobs/search?open_within_days=30&posted_within_days=3`
- **THEN** only jobs first recorded within 30 days AND stating a posting date within
  3 days are returned

#### Scenario: Invalid open-within value imposes no restriction

- **WHEN** a client requests `GET /api/v1/jobs/search` with `open_within_days` absent,
  zero, negative, or non-numeric
- **THEN** the result is not restricted by first-seen date

#### Scenario: The parameter is not reported as unknown

- **WHEN** a client requests `GET /api/v1/jobs/search?open_within_days=7`
- **THEN** the response does not list `open_within_days` among the unknown parameters
