## ADDED Requirements

### Requirement: Catalogue scale is served from one periodically recomputed snapshot

The system SHALL compute the public catalogue-scale figures — the open-job
count, the company count, the number of registered ATS platform adapters, and
the number of crawled Telegram channels — as a single `Snapshot` value carrying
the instant it was computed.

The open-job and company counts SHALL be exact counts over the same predicate
the public listings apply (`closed_at IS NULL AND duplicate_of IS NULL AND NOT
is_private`), so the total a surface quotes describes the same set of postings a
visitor can page through. The ATS-platform count SHALL be derived from the
provider registry and the Telegram-channel count from the crawled channel
configuration, not maintained as a literal in either the backend or the
frontend.

Every consumer SHALL read the same snapshot, so two surfaces rendered from the
same snapshot cannot disagree about the size of the catalogue.

#### Scenario: Snapshot counts match the public listing predicate

- **WHEN** the snapshot is computed against a catalogue containing open,
  closed, duplicate-suppressed, and private postings
- **THEN** its open-job count includes only postings that are open, not
  duplicate-suppressed, and not private — the same set `GET /api/v1/jobs`
  paginates

#### Scenario: Platform and channel counts are derived, not hardcoded

- **WHEN** a new source adapter is registered or a new Telegram channel is added
  to the crawled configuration
- **THEN** the next computed snapshot reflects the new count with no change to
  any literal in the backend or the frontend

### Requirement: The exact count never runs on a request path

The system SHALL recompute the snapshot only from the scheduled rollup worker,
never during an HTTP request. A request SHALL NOT trigger a query whose cost
grows with catalogue size.

The recomputed snapshot SHALL be published to a shared cache with a retention
window that outlives the worker's schedule, so a skipped or failed worker run
degrades the figure to a slightly stale exact value rather than to no value.

#### Scenario: Serving a request never recomputes

- **WHEN** any number of clients request a surface that quotes catalogue scale
- **THEN** no exact catalogue-wide count is executed as part of serving those
  requests

#### Scenario: A skipped worker run keeps the last snapshot

- **WHEN** the rollup worker does not run for longer than its normal interval
  but within the cache retention window
- **THEN** consumers continue to receive the last computed snapshot, and its
  computed-at instant reports how stale it is

### Requirement: A cold or unreachable cache degrades, never fails

The system SHALL treat an absent or unreadable cached snapshot as a miss and
fall back to the existing approximate open-job estimate. A cache failure SHALL
NOT fail a request, and SHALL NOT be surfaced to the client as an error.

Consumers SHALL be able to tell an exact snapshot from the degraded fallback, so
a surface can choose to present the figure differently when it is only an
estimate.

#### Scenario: Cache is empty on a cold start

- **WHEN** the cache holds no snapshot and a client requests the jobs list
- **THEN** `meta.total` carries the approximate estimate and the request
  succeeds normally

#### Scenario: Cache backend is unreachable

- **WHEN** the cache backend cannot be reached
- **THEN** the request succeeds with the degraded figure rather than returning
  an error

### Requirement: The snapshot is exposed as a public endpoint

The system SHALL serve the snapshot over an unauthenticated
`GET /api/v1/stats/catalog`, using the single-item envelope `{"data": ...}`. The
response SHALL carry the open-job count, the company count, the ATS-platform
count, the Telegram-channel count, and the instant the snapshot was computed.

#### Scenario: Endpoint returns the whole snapshot

- **WHEN** an anonymous client requests `GET /api/v1/stats/catalog`
- **THEN** the response is `{"data": {...}}` carrying all four figures and the
  computed-at instant

#### Scenario: One request replaces two list reads

- **WHEN** a page needs both the open-job count and the company count
- **THEN** it obtains both from one `GET /api/v1/stats/catalog` response, and
  the two figures come from the same snapshot
