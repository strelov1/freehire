# site-health-status Specification

## Purpose

Reports whether the site/API itself is functioning — independent of the
ingest fleet's own status — from signal the serving process already has,
with no external dependency and no new storage.

## Requirements

### Requirement: Rolling request-error window

The system SHALL maintain an in-process, per-minute-bucketed count of total
HTTP responses and 5xx responses, fed by every response the process sends
(both the normal-completion path and the error-handler path). Given a
trailing duration, it SHALL report the fraction of responses that were 5xx
within that duration, and the total response count the fraction was computed
from. Buckets older than the requested duration SHALL be excluded from both
figures. A window with no recorded responses SHALL report a rate of 0 and a
total of 0, never a division error.

#### Scenario: Rate reflects only responses within the window

- **WHEN** one 5xx response was recorded 10 minutes ago and three 2xx
  responses were recorded just now, and the fraction is requested for a
  5-minute trailing window
- **THEN** the total is 3 and the error fraction is 0 (the old 5xx falls
  outside the window)

#### Scenario: Mixed responses within the window

- **WHEN** 6 responses in the trailing window were 2xx and 2 were 5xx
- **THEN** the reported total is 8 and the error fraction is 0.25

#### Scenario: No traffic yet

- **WHEN** no response has been recorded at all
- **THEN** the reported total is 0 and the error fraction is 0

### Requirement: Site status derivation

The system SHALL derive the site's own status — `operational`, `degraded`,
or `down` — from a live database-availability check and the rolling
request-error window, via a pure function:

- `down` WHEN the database check fails, regardless of the error window.
- `operational` WHEN the database check succeeds AND the window's total
  response count is below a minimum-traffic threshold (too little data to
  trust an error fraction).
- `down` WHEN the database check succeeds but the window's error fraction is
  at or above a "down" threshold.
- `degraded` WHEN the database check succeeds and the error fraction exceeds
  a lower "degraded" threshold but is below the "down" threshold.
- `operational` otherwise.

The thresholds SHALL be named constants.

#### Scenario: Database unreachable

- **WHEN** the database check fails
- **THEN** the derived site status is `down`, regardless of the error window

#### Scenario: Healthy database, negligible errors

- **WHEN** the database check succeeds and the error fraction is 0 over a
  window with enough traffic to trust
- **THEN** the derived site status is `operational`

#### Scenario: Too little traffic to trust the error fraction

- **WHEN** the database check succeeds but the window's total response count
  is below the minimum-traffic threshold, even if every one of those few
  responses was a 5xx
- **THEN** the derived site status is `operational`

#### Scenario: Elevated but moderate error rate

- **WHEN** the database check succeeds, there is enough traffic to trust the
  window, and the error fraction is above the degraded threshold but below
  the down threshold
- **THEN** the derived site status is `degraded`

#### Scenario: Database up but most requests failing

- **WHEN** the database check succeeds but the error fraction is at or above
  the down threshold
- **THEN** the derived site status is `down`

### Requirement: Site status does not depend on the ingest-fleet rollup succeeding

The system SHALL compute and return the site status from the database check
and the request-error window alone. When the database is unreachable, the
system SHALL NOT attempt the ingest-fleet rollup query first and propagate
its failure instead of reporting the site status — the database check SHALL
run first, and a failed database check SHALL short-circuit straight to a
response carrying the site status, without depending on the rollup query at
all.

#### Scenario: Database unreachable short-circuits before the rollup query

- **WHEN** the database is unreachable
- **THEN** the site status is still computed (as `down`) and returned,
  without the request also depending on the ingest-fleet rollup query
  succeeding, and the response's ingest-fleet fields report the same "no
  data" value an empty rollup already uses rather than asserting the fleet
  itself is down
