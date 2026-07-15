## ADDED Requirements

### Requirement: Per-provider health rollup

The system SHALL provide a read-only rollup over `board_health` grouped by
`provider`, exposing for each provider: total boards, healthy boards
(`consecutive_failures = 0`), boards currently in cooldown (`cooldown_until >
now()`), the most recent `last_run_at`, the most recent `last_success_at`, and
the sum of `last_ingested_count` over healthy boards. The rollup SHALL be pure
read: it SHALL NOT modify `board_health` or affect cooldown behavior.

#### Scenario: Rollup aggregates a provider's boards

- **WHEN** a provider has 3 boards recorded in `board_health`, 2 with
  `consecutive_failures = 0` and 1 in active cooldown
- **THEN** the rollup returns one row for that provider with `total_boards = 3`,
  `healthy_boards = 2`, and `cooled_boards = 1`

#### Scenario: Rollup ignores providers with no recorded boards

- **WHEN** a provider exists in the source YAML but has never been crawled (no
  `board_health` row)
- **THEN** the rollup returns no row for that provider

### Requirement: Provider status derivation

The system SHALL derive each provider's status from its rollup via a pure
function using the healthy fraction (`healthy_boards / total_boards`) and success
freshness:

- `operational` WHEN the healthy fraction is at least 0.9 AND a success occurred
  within the freshness window (48 hours).
- `down` WHEN the healthy fraction is at most 0.1, OR no success occurred within
  the freshness window.
- `degraded` in every other case.

The thresholds and freshness window SHALL be named constants.

#### Scenario: All boards healthy and fresh

- **WHEN** a provider has 100 boards, all healthy, with a success 1 hour ago
- **THEN** its derived status is `operational`

#### Scenario: A minority of boards failing

- **WHEN** a provider has 100 boards, 20 failing, with a recent success
- **THEN** its derived status is `degraded`

#### Scenario: Almost all boards failing

- **WHEN** a provider has 100 boards, 95 failing
- **THEN** its derived status is `down`

#### Scenario: Stale despite healthy counts

- **WHEN** a provider's boards all show `consecutive_failures = 0` but the most
  recent `last_success_at` is older than the freshness window
- **THEN** its derived status is `down`

### Requirement: Overall fleet status

The system SHALL derive an overall fleet status as the worst individual provider
status: `down` if any provider is `down`, else `degraded` if any provider is
`degraded`, else `operational`.

#### Scenario: One provider down drags the fleet

- **WHEN** every provider is `operational` except one that is `down`
- **THEN** the overall status is `down`

#### Scenario: Empty fleet

- **WHEN** there are no providers in the rollup
- **THEN** the overall status is `operational`

### Requirement: Public status endpoint

The system SHALL expose `GET /api/v1/status` as a public, unauthenticated
endpoint returning `{ "data": { overall, generated_at, providers[] } }`. Each
provider entry SHALL include only sanitized fields: provider key, derived status,
total/healthy/cooled board counts, last run, last success, and ingested total.
The response SHALL NOT include raw error text (`last_error`) or individual board
identifiers.

#### Scenario: Anonymous request succeeds

- **WHEN** an unauthenticated client requests `GET /api/v1/status`
- **THEN** the response is `200` with `data.overall` and a `data.providers`
  array

#### Scenario: No internal detail leaks

- **WHEN** the response is inspected
- **THEN** it contains no `last_error` field and no board-level identifiers

### Requirement: Public status page

The web app SHALL serve a public `/status` page that renders the overall fleet
status as a banner and a flat list of providers, each showing a status pill,
board counts (total and healthy), and a relative last-run time. The page SHALL be
reachable without authentication.

#### Scenario: Visitor views the status page

- **WHEN** an unauthenticated visitor opens `/status`
- **THEN** they see an overall status banner and one row per provider with its
  status pill and board counts
