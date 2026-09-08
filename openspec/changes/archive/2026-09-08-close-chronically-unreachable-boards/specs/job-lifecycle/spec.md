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

### Requirement: A region-ambiguous board name is never board-scoped by the safety net

A board name that is registered under more than one region for its provider (e.g. a
per-country board such as Adzuna's `it-jobs`) SHALL NOT be closed by the board-scoped
path of the safety-net pass, even when the specific `(provider, board, region)` health
record that triggered the pass is itself past the closure window. A job's stored
identity carries no region, so a board-scoped close cannot distinguish one region's
postings from another's, and closing by board name alone risks closing a healthy
region's jobs alongside the genuinely chronic one — the same hazard the ordinary
unseen sweep's board scope already refuses for the same reason. Such a board SHALL be
skipped and reported as skipped, not silently dropped, so an operator reading the run
can see that a region-ambiguous board needs a decision the automated pass could not
safely make on its own. A boardless provider's own health record is never
region-ambiguous in this sense, since it already stands for the whole provider with
no board-name collision possible.

#### Scenario: A board chronic in one region but healthy in another is skipped

- **WHEN** the safety-net pass runs and a board name is chronic past the closure
  window under one region but has a recent successful crawl under a different region
  for the same provider
- **THEN** that board's jobs are not closed, and the run reports it as skipped for
  being region-ambiguous

#### Scenario: An unambiguous board name closes normally

- **WHEN** the safety-net pass runs and a chronic board name is registered under only
  one region for its provider
- **THEN** that board's open jobs are closed as usual

### Requirement: A board that recovers before the close reaches it is never closed

The safety-net pass SHALL re-evaluate whether a board is still past the closure window
at the moment it closes (or, in a dry run, counts) that board's jobs, rather than
trusting only the earlier read that selected it as chronic. A board whose crawl
succeeds — moving its recorded evidence of the last successful crawl forward — between
that earlier selection and the close SHALL NOT have its jobs closed, even though the
pass had already decided, based on the now-stale read, that it qualified.

#### Scenario: A board that recovers between selection and close is spared

- **WHEN** the safety-net pass selects a board as chronic, the board's crawl then
  succeeds, and only afterward does the pass reach the point of closing that board's
  jobs
- **THEN** the board's jobs are not closed

#### Scenario: A dry run's count reflects the same re-evaluation

- **WHEN** a dry run counts what it would close for a board that recovered after
  selection but before the count
- **THEN** the reported count for that board is zero, not the number that would have
  matched at selection time

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
