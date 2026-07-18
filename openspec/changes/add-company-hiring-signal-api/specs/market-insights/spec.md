## ADDED Requirements

### Requirement: Company hiring-signal leaderboard

The system SHALL expose a public, unauthenticated `GET /api/v1/insights/companies`
endpoint that returns companies ranked by hiring growth or current open-count,
served from a precomputed per-company scalar (never aggregated per request over the
full catalogue). It SHALL accept `sort` (`growth` = ramping first, `-growth` =
freezing first, `open` = largest first; default `growth`), a `min_open` threshold
(a small positive default) that excludes companies whose current open-count is below
it, and a `limit` capped to a fixed maximum. It SHALL respond with the standard list
envelope `{"data": [...], "meta": {...}}` where each row carries `company_slug`,
`company_name`, `open_now`, `open_prev_30d`, and `growth_30d`. The response SHALL be
aggregate-only — no record-level job fields.

#### Scenario: Top ramping companies

- **WHEN** a client requests `GET /api/v1/insights/companies?sort=growth`
- **THEN** the response is `200` with a `data` array ordered by
  `growth_30d` (= `open_now − open_prev_30d`) descending, each row carrying
  `company_slug`, `company_name`, `open_now`, `open_prev_30d`, `growth_30d`
- **AND** `meta` echoes the applied `sort`, `min_open`, and `limit`

#### Scenario: Freezing companies

- **WHEN** a client requests `GET /api/v1/insights/companies?sort=-growth`
- **THEN** companies are ordered by `growth_30d` ascending (largest declines first)

#### Scenario: min_open excludes small companies

- **WHEN** a client requests `GET /api/v1/insights/companies?min_open=10`
- **THEN** only companies whose `open_now` is at least `10` appear in `data`

#### Scenario: Invalid sort rejected

- **WHEN** a client requests `GET /api/v1/insights/companies?sort=bogus`
- **THEN** the response is `400` and no data is returned

#### Scenario: Limit is capped

- **WHEN** a client requests a `limit` above the endpoint's maximum
- **THEN** the applied limit is clamped to that maximum (or `400`), never unbounded
