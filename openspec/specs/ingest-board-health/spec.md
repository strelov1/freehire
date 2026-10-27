# ingest-board-health Specification

## Purpose
TBD - created by archiving change ingest-board-health. Update Purpose after archive.
## Requirements
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

### Requirement: A board whose listing is unchanged is crawled progressively less often
After each complete, successful crawl the system SHALL compute the board's listing fingerprint: the
sha256 of the board's de-duplicated, sorted posting external ids, counting every posting the listing
named whatever its later outcome. When the fingerprint equals the one stored from the previous complete
successful crawl, the system SHALL increment the board's quiet streak and SHALL set a quiet pause of
4h·2^(streak−1), capped at 72h. When it differs, or no fingerprint is stored, the system SHALL store
the new fingerprint, reset the quiet streak to zero and clear the quiet pause.

#### Scenario: Unchanged listing grows the pause
- **WHEN** a board's complete successful crawl yields the same set of posting ids as its previous one, for the third time in a row
- **THEN** its quiet streak is 3 and its quiet pause ends 16h after that crawl

#### Scenario: The pause is capped
- **WHEN** a board's quiet streak reaches 7
- **THEN** its quiet pause is 72h, not longer

#### Scenario: A posting appearing resets the board
- **WHEN** a board's complete crawl yields one posting id that its previous crawl did not
- **THEN** its quiet streak is reset to 0 and it has no quiet pause

#### Scenario: A posting disappearing resets the board
- **WHEN** a board's complete crawl no longer yields one posting id its previous crawl yielded
- **THEN** its quiet streak is reset to 0 and it has no quiet pause

#### Scenario: Order and duplicates do not count as change
- **WHEN** a board's crawl yields the same posting ids in a different order, or with one id repeated
- **THEN** the fingerprint is unchanged and the crawl counts as quiet

### Requirement: An incomplete crawl leaves the quiet state untouched
A crawl that did not read the board's whole listing SHALL NOT change the board's fingerprint, quiet streak
or quiet pause. That covers a crawl interrupted by the run's deadline, one that failed, one cut short by the
context ending, one where a listed posting could not be read or failed to save, and one where a hydrating
adapter left a NEW posting it listed un-hydrated (detail budget spent or detail failed).

#### Scenario: Deferred new postings
- **WHEN** a hydrating adapter lists three new postings but hydrates only one before its detail budget runs out
- **THEN** the board's quiet state is unchanged

#### Scenario: Interrupted crawl
- **WHEN** a board's crawl is interrupted by the run's crawl deadline
- **THEN** its stored fingerprint, quiet streak and quiet pause are exactly what they were before the run

### Requirement: A board in its quiet pause is skipped
Before invoking a board's adapter the system SHALL skip the board when its quiet pause lies in the future,
exactly as it skips a board in failure cooldown, and SHALL count it in the run's quiet total, separately
from cooled boards. The check SHALL fail open: a health-store error crawls the board.

#### Scenario: Quiet board skipped
- **WHEN** a run reaches a board whose quiet pause ends in the future
- **THEN** the adapter is not invoked and the run's quiet count increases by one

#### Scenario: Health-store error crawls the board
- **WHEN** reading the board's pauses fails
- **THEN** the board is crawled

### Requirement: The quiet pause is independent of the failure cooldown
The quiet pause SHALL be stored separately from the failure cooldown. Clearing cooldowns after a
provider-wide recovery SHALL NOT clear any quiet pause, and a failed crawl SHALL NOT change the quiet state.

#### Scenario: Recovery probe keeps quiet pauses
- **WHEN** a provider's recovery probe succeeds and clears its boards' cooldowns
- **THEN** every quiet pause of that provider's boards is unchanged

### Requirement: Self-closing providers are never paced by quietness
For a provider that closes its own postings from a change stream, the system SHALL NOT compute a
fingerprint or apply a quiet pause, because its emissions are changes, not a listing.

#### Scenario: Self-closing provider crawl
- **WHEN** a self-closing provider's board completes a crawl that emits nothing
- **THEN** its quiet streak stays 0 and it has no quiet pause

### Requirement: A board the run skipped is never swept by the company or source scope
The post-run unseen sweep SHALL exclude every posting of a board the run skipped — in quiet pause or in
failure cooldown — from its company-scoped and source-scoped closes. A skipped board's postings were
not looked at, so their age since last seen says nothing about them. Without this, a quiet pause longer
than the sweep window (48h) would close the live postings of a company whose other board the run did crawl.

#### Scenario: Quiet board of a company the run crawled elsewhere
- **WHEN** a company has two boards on one provider, the run crawls one and skips the other as quiet, and the skipped board's postings were last seen 60h ago
- **THEN** the company-scoped close leaves the skipped board's postings open

#### Scenario: Cooled board under a source-scoped close
- **WHEN** a full-catalogue provider's run skips one of its boards in failure cooldown
- **THEN** the source-scoped close leaves that board's postings open

### Requirement: A quiet board reads as fresh in health rollups
While a board's quiet pause holds, the provider health rollups that back the public status page and the
silence alerts SHALL treat the board as freshly successful. The board skips crawls on purpose for longer
than their 48h freshness window, and the pause is only ever set by a success.

#### Scenario: All of a provider's boards are quiet
- **WHEN** every board of a provider is in its quiet pause and their last success was 60h ago
- **THEN** the status page and the silence alert report the provider as fresh

