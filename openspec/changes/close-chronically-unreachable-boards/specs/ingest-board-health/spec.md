## ADDED Requirements

### Requirement: A board with no success for a long window is classified chronic

The system SHALL classify a board as **chronic** when it has recorded no successful
crawl for at least a configurable chronic window (default 30 days), measured from
`last_success_at`. For a board that has never once succeeded, the window SHALL be
measured from when its health record was first created rather than from a
`last_success_at` that does not exist. A board below the window — including one
currently in an ordinary cooldown from a recent run of failures — SHALL NOT be
classified chronic; chronic status requires the failures to have persisted across
the window, not merely accumulated quickly.

#### Scenario: A board stuck failing past the window is chronic

- **WHEN** a board's `last_success_at` is more than 30 days in the past and every
  crawl since has failed
- **THEN** the board is classified chronic

#### Scenario: A board that recently recovered is not chronic

- **WHEN** a board has been failing for 10 days since its last success
- **THEN** the board is not classified chronic, regardless of its consecutive-failure
  count

#### Scenario: A board that has never succeeded is measured from first sight

- **WHEN** a board's health record has no `last_success_at` and was first created 31
  days ago, failing on every crawl since
- **THEN** the board is classified chronic

### Requirement: The unhealthy-board summary distinguishes chronic boards

The per-run unhealthy-board summary and the operator-facing health rollup SHALL
report chronic boards as a distinct group from boards that are merely cooling down or
accumulating failures below the chronic window, so a curator scanning the summary can
tell "still within a normal backoff" apart from "has not worked in over a month and
needs a decision."

#### Scenario: A chronic board appears in its own group

- **WHEN** a run's unhealthy-board summary includes both a board cooling down from a
  handful of recent failures and a board chronic for 40 days
- **THEN** the summary reports the chronic board separately from the merely-cooling one
