## MODIFIED Requirements

### Requirement: Public status endpoint

The system SHALL expose `GET /api/v1/status` as a public, unauthenticated
endpoint returning `{ "data": { overall, generated_at, providers[], site } }`.
Each provider entry SHALL include only sanitized fields: provider key,
derived status, total/healthy/cooled board counts, last run, last success,
and ingested total. The response SHALL NOT include raw error text
(`last_error`) or individual board identifiers. `site` SHALL carry the
site/API's own derived status, database availability, the rolling error
fraction, and the window (in minutes) it was computed over — independent of
`overall`, which reflects the ingest fleet only.

#### Scenario: Anonymous request succeeds

- **WHEN** an unauthenticated client requests `GET /api/v1/status`
- **THEN** the response is `200` with `data.overall`, a `data.providers`
  array, and a `data.site` object carrying the site's own status

#### Scenario: No internal detail leaks

- **WHEN** the response is inspected
- **THEN** it contains no `last_error` field and no board-level identifiers

#### Scenario: Database unreachable

- **WHEN** the database is unreachable
- **THEN** the response is still `200`, with `data.site.status` `"down"`,
  `data.overall` `"operational"` (the same "no data" value an empty rollup
  already reports — a database outage means the ingest fleet's health is
  unknown for this request, not that it has failed) and `data.providers` an
  empty array, rather than an error response with no body

### Requirement: Public status page

The web app SHALL serve a public `/status` page that renders a "Site status"
section (status pill for the site/API itself) above the existing ingest-fleet
section, which renders the overall fleet status as a banner and a flat list
of providers, each showing a status pill, board counts (total and healthy),
and a relative last-run time. The page SHALL be reachable without
authentication. The site-status section and the ingest-fleet section SHALL
be visually distinct, since they report on different things (the site/API
itself vs. the crawl fleet).

#### Scenario: Visitor views the status page

- **WHEN** an unauthenticated visitor opens `/status`
- **THEN** they see a site-status pill, an overall ingest-fleet status
  banner, and one row per provider with its status pill and board counts
