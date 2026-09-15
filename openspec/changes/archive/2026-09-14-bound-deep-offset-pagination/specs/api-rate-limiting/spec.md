## ADDED Requirements

### Requirement: Bounded pagination depth on every list endpoint

Every paginated list endpoint SHALL refuse a request whose `offset + limit` reaches past a
single shared window, whichever store answers the endpoint. The refusal SHALL be `400` and
SHALL NOT be a clamped `200`.

A per-caller rate limit cannot substitute for this bound. A rate limit caps requests per
minute; an endpoint whose `OFFSET` the caller chooses has a cost per request the caller also
chooses, so a caller obeying its budget in full can still exhaust the connection pool. The
two are separate defences and both are required.

#### Scenario: A request past the window is refused

- **WHEN** a caller requests a list endpoint with `offset + limit` greater than the shared
  pagination window
- **THEN** the endpoint answers `400` with an error naming the pagination as too deep, and
  no database query for that page is issued

#### Scenario: The last page inside the window still serves

- **WHEN** a caller requests a list endpoint with `offset + limit` exactly equal to the
  shared pagination window
- **THEN** the endpoint answers `200` with that page

#### Scenario: A refused page is never silently substituted

- **WHEN** a caller requests a page past the window
- **THEN** the response is an error, not a successful page carrying rows from a different
  offset — a walker must be able to tell "no such page" from "here is a page"

#### Scenario: The window is one number for every store

- **WHEN** the same offset and limit are requested from a list served by Postgres and from a
  list served by the search index
- **THEN** both apply the same window, so neither store's lists can drift to a different
  depth than the other's

#### Scenario: An out-of-range offset does not become a server error

- **WHEN** a caller supplies an offset larger than a 32-bit integer can hold
- **THEN** the endpoint answers `400` for being too deep rather than `500` from a wrapped
  negative bind parameter

### Requirement: Every public list endpoint carries a rate limiter

Every unauthenticated list endpoint SHALL mount a rate limiter. An endpoint registered
without one is a defect regardless of how cheap its query is.

#### Scenario: The public company-feedback list is throttled

- **WHEN** an anonymous caller exceeds the public-read budget on `GET /api/v1/companies/:slug/feedback`
- **THEN** the request is refused with `429`, as it is on the other public reads

### Requirement: A query may not hold a pooled connection without bound

The API server's database pool SHALL impose a server-side statement timeout, so a single
slow query cannot hold a pooled connection indefinitely and starve every other request.

This is a backstop, not the primary bound: it exists for the endpoint whose cost nobody
anticipated. It applies to the request-serving pool only — the cron workers share the same
package and some legitimately run for hours.

#### Scenario: A runaway query is cut, not left to hold its connection

- **WHEN** a query issued on the API server's pool runs past the configured statement timeout
- **THEN** Postgres cancels it and the connection returns to the pool, and the request
  answers an error rather than hanging

#### Scenario: A cron worker's pool is not bounded by the server's timeout

- **WHEN** a batch worker opens a pool through the shared database package without asking
  for a statement timeout
- **THEN** its queries carry no statement timeout, so a multi-hour backfill is not cut
