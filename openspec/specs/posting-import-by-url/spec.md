# posting-import-by-url Specification

## Purpose
TBD - created by archiving change import-posting-by-url. Update Purpose after archive.
## Requirements
### Requirement: A page already in the catalog is answered without importing

The system SHALL, before any outbound fetch, resolve the requested URL against the catalog
by source identity and then by stored posting URL. When either resolves, the system SHALL
answer with that posting's `public_slug` and status `found`, and SHALL NOT fetch the page
or write any row.

#### Scenario: A carried posting is returned as found

- **WHEN** an authenticated user submits the URL of a page whose posting the catalog
  already carries
- **THEN** the response is 200 with that posting's `public_slug` and status `found`, and no
  new posting is created

### Requirement: A parseable page is imported into the catalog

The system SHALL resolve a URL the catalog does not carry through the link-source
registry — the host-scoped adapters first, the generic `JobPosting` resolver last — and,
when a single vacancy is parsed, write it under the destination's own
`(source, external_id)` through the canonical job write path, enqueuing it for enrichment.
The response SHALL carry the resulting `public_slug` and status `imported`.

#### Scenario: A vacancy page is imported and answered with its slug

- **WHEN** an authenticated user submits the URL of a vacancy the catalog lacks and an
  adapter parses it
- **THEN** the response is 201 with the new posting's `public_slug` and status `imported`,
  and the posting is readable from the catalog by that slug

#### Scenario: A re-submitted import resolves to the same posting

- **WHEN** the same URL is submitted twice
- **THEN** the second call answers with the same `public_slug` and creates no second
  posting

### Requirement: An unparseable page is queued for triage

The system SHALL, when no adapter parses the page, record the link through the contribution
service rather than discarding it, and answer with status `queued` and a null
`public_slug`. A link that is not an `http(s)` URL SHALL be rejected as unprocessable
instead of recorded.

#### Scenario: A page with no vacancy markup is queued

- **WHEN** an authenticated user submits an `http(s)` URL that no adapter can parse into a
  vacancy
- **THEN** the response is 202 with status `queued` and a null `public_slug`, and the link
  is recorded for manual triage

#### Scenario: A non-URL is rejected

- **WHEN** the submitted value is not an `http(s)` URL
- **THEN** the response is 422 and nothing is recorded

### Requirement: Import requests are authenticated and rate limited

The system SHALL require authentication for this endpoint and SHALL count its calls
against the same per-user budget as board contributions, because both make the server
fetch a user-supplied URL. Outbound fetches SHALL use the SSRF-guarded HTTP client.

#### Scenario: An unauthenticated import is refused

- **WHEN** the endpoint is called without credentials
- **THEN** the response is 401 and no fetch is made

#### Scenario: Exceeding the shared budget is refused

- **WHEN** a user's combined import and board-contribution calls exceed the hourly budget
- **THEN** further calls are refused with 429

