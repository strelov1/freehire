## MODIFIED Requirements

### Requirement: Board health is runtime state only; the board catalog is unchanged

The `board_health` table SHALL hold only runtime state (failure counts, cooldown, last
error, timestamps, last ingested count) and SHALL NOT hold catalog or lifecycle data. The
set of boards to crawl and their schedule SHALL remain sourced from the `boards` table; a
`board_health` row is a sidecar keyed by a board's identity, created lazily and harmless
if its board later leaves the active catalog (retired).

#### Scenario: Retiring a board leaves no orphaned behavior

- **WHEN** a board's status in `boards` becomes `retired`
- **THEN** the run simply stops touching that board; its stale `board_health` row is
  inert (never read for a board that is not eligible to crawl) and changes no scheduling
