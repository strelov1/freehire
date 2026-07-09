## ADDED Requirements

### Requirement: Each board crawl records its outcome to persistent health state

The ingest run SHALL record the outcome of every board it crawls into a persistent
`board_health` row keyed by `(provider, board)`. On a **success** it SHALL reset
`consecutive_failures` to 0, clear any `cooldown_until`, and stamp `last_success_at`
and `last_ingested_count`. On a **failure** (an unknown provider or a fetch error) it
SHALL increment `consecutive_failures`, store `last_error` and `last_error_at`, and
stamp `last_run_at`. The row is created on first sight of a board and updated
in place thereafter.

#### Scenario: A successful crawl resets failure state

- **WHEN** a board that previously had `consecutive_failures = 3` and a cooldown is
  crawled successfully
- **THEN** its `board_health` row has `consecutive_failures = 0`, `cooldown_until`
  cleared, and a fresh `last_success_at` and `last_ingested_count`

#### Scenario: A failed crawl accumulates failure state

- **WHEN** a board's fetch errors on a run
- **THEN** its `board_health` row has `consecutive_failures` incremented and records
  the error text and time in `last_error` / `last_error_at`

### Requirement: A failing board is cooled down with a capped, self-healing backoff

The run SHALL set `cooldown_until` on a failing board using an exponential backoff of
its `consecutive_failures`, but SHALL NOT apply any cooldown until a threshold of
consecutive failures is reached (the hourly cron re-run is the natural retry for the
first few). The cooldown SHALL be capped at approximately 24 hours and SHALL never be
permanent — a board always becomes eligible again after its window, and a later
success clears the cooldown (self-heal).

#### Scenario: The first failures do not cool the board down

- **WHEN** a board fails for the first time (below the cooldown threshold)
- **THEN** no `cooldown_until` is set and the board is eligible on the next run

#### Scenario: Repeated failures back the board off, capped

- **WHEN** a board's `consecutive_failures` crosses the threshold and keeps growing
- **THEN** `cooldown_until` grows exponentially with each failure but never exceeds
  the ~24h cap

#### Scenario: A cooldown is always temporary

- **WHEN** a cooled-down board's `cooldown_until` passes
- **THEN** the board is crawled again on the next run, and a success clears the cooldown

### Requirement: A board in cooldown is skipped before its adapter is invoked

The run SHALL skip a board whose `cooldown_until` is in the future, checked BEFORE the
adapter is invoked, so a persistently-failing board does not crawl every run. A
skipped-for-cooldown board SHALL be logged once and SHALL NOT count as a fetch failure
for the run's stats (it is a deliberate skip, not an error).

#### Scenario: A cooled board is not crawled

- **WHEN** the run reaches a board whose `cooldown_until` is in the future
- **THEN** the adapter's Fetch is not called for that board, and the board is counted
  as cooled/skipped rather than failed

#### Scenario: An eligible board crawls normally

- **WHEN** the run reaches a board with no cooldown (or a past `cooldown_until`)
- **THEN** the adapter is invoked and the outcome is recorded as usual

### Requirement: Unhealthy boards are visible without grepping logs

The run SHALL emit a per-run summary of the boards that are currently unhealthy — in
cooldown or with `consecutive_failures > 0` — and the `board_health` table SHALL be
directly queryable so an operator can list failing boards with their last error and
next-eligible time in one query.

#### Scenario: The run summarizes unhealthy boards

- **WHEN** a run finishes with some boards in cooldown or accumulating failures
- **THEN** it logs a summary line naming those boards, distinct from the routine
  per-board logs

### Requirement: Board health is runtime state only; the YAML catalog is unchanged

The `board_health` table SHALL hold only runtime state (failure counts, cooldown,
last error, timestamps, last ingested count) and SHALL NOT hold catalog or cadence
data. The set of boards to crawl and their schedule SHALL remain sourced from the
YAML board files in git; a `board_health` row is a sidecar keyed by a board's
identity, created lazily and harmless if its board later leaves the YAML.

#### Scenario: Removing a board from YAML leaves no orphaned behavior

- **WHEN** a board entry is removed from its YAML file
- **THEN** the run simply stops touching that board; its stale `board_health` row is
  inert (never read for a board that is not in the catalog) and changes no scheduling
