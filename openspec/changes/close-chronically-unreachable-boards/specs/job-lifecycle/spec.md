## ADDED Requirements

### Requirement: Jobs on a chronically unreachable board are closed by a safety-net pass

The system SHALL run a safety-net pass, separate from the per-run unseen sweep, that
closes the open jobs of a board once it has been chronic (see `ingest-board-health`)
for at least a second, longer configurable window (default 60 days total since its
last successful crawl). This pass SHALL close only the jobs belonging to the specific `(provider, board)`
health record that crossed the closure window, never jobs outside it: a provider with
one permanently broken board and many healthy ones only loses the broken board's
jobs. A boardless provider is tracked as a single health record covering its whole
crawl, so for such a provider going chronic does mean its whole catalogue closes —
that is the correct reading of "this board's jobs" when the provider has no finer
board grain, not a broadening of the rule. A
board that succeeds even once resets its chronic state (per `ingest-board-health`)
and SHALL be excluded from this pass on its next run. This safety net exists
specifically for the case the ordinary sweep cannot reach: `shouldSweep`,
`boardQualifies`, and `sweepableCompanies` never let the unseen sweep close a board
the run did not prove it covered, which is correct for a transient failure but leaves
a permanently broken board's jobs open forever without it.

#### Scenario: A board chronic past the closure window has its jobs closed

- **WHEN** the safety-net pass runs and a board has had no successful crawl for 61
  days
- **THEN** that board's open jobs are closed

#### Scenario: A chronic board short of the closure window is left alone

- **WHEN** the safety-net pass runs and a board has had no successful crawl for 40
  days (chronic, but short of the 60-day closure window)
- **THEN** that board's open jobs remain open

#### Scenario: A board that recovers is excluded going forward

- **WHEN** a board that was chronic for 65 days is crawled successfully before the
  safety-net pass reaches it
- **THEN** the pass does not close that board's jobs

#### Scenario: Only the broken board's jobs are closed, not the whole provider

- **WHEN** a multi-board provider has one board chronic past the closure window and
  other boards crawling normally
- **THEN** only the chronic board's open jobs are closed; the other boards' jobs are
  unaffected

#### Scenario: A chronic boardless provider closes its whole catalogue

- **WHEN** a boardless provider's single health record is chronic past the closure
  window
- **THEN** that provider's open jobs are closed, since the provider has no finer
  board grain to scope to

### Requirement: A safety-net close carries its own mechanism label

A job closed by the chronic-board safety net SHALL record a close reason distinct
from the unseen sweep's, the liveness probe's, and the age rule's, consistent with
the existing requirement that every close records the mechanism that wrote it. A job
closed this way that later reopens (its board is re-added and crawled successfully)
SHALL have that record cleared together with `closed_at`, the same as any other
close reason.

#### Scenario: The safety-net close is labeled distinctly

- **WHEN** a job is closed by the chronic-board safety net
- **THEN** its close reason identifies that mechanism, distinct from `unseen sweep`,
  `liveness probe`, or `age rule`

#### Scenario: A safety-net-closed job reopens cleanly

- **WHEN** a job closed by the chronic-board safety net is later re-ingested because
  its board crawls successfully again
- **THEN** both `closed_at` and its close reason are cleared
