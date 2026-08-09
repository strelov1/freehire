## ADDED Requirements

### Requirement: A run verifies its own edits with job_match before reporting

An autopilot run SHALL check its edits against the deterministic `job_match` score before it
reports: it MUST call `job_match` at least once before calling `tailor_report`, and MAY repeat a
short edit-and-check cycle (a few rounds) while `job_match` still shows a closeable `missing_have`
or `missing_gap`. Stopping is the agent's own judgment, not a fixed numeric score threshold —
`job_match`'s Title Match and Seniority Fit categories do not move from text edits alone, so a
fixed bar could stall the run on a number editing can never reach.

#### Scenario: job_match is checked before the report

- **WHEN** an autopilot run is about to finish
- **THEN** it has called `job_match` at least once before calling `tailor_report`

#### Scenario: A closeable gap triggers another edit-and-check round

- **WHEN** `job_match` still shows a `missing_have` or `missing_gap` the run believes it can close
- **THEN** the run makes another edit and calls `job_match` again rather than reporting immediately

#### Scenario: The run does not loop indefinitely

- **WHEN** the run has already checked `job_match` a few times without closing what remained
- **THEN** it stops iterating and calls `tailor_report` rather than continuing indefinitely

## MODIFIED Requirements

### Requirement: The workspace shows the run report beside the fit analysis

The workspace SHALL render the run report in its right-hand panel above the existing fit analysis,
showing each requirement with its outcome, and SHALL offer starting another run and reverting the last
one from that block. Starting a run MUST be offered whether or not a report exists — a CV that has
just been reverted has no report by design, and one whose run stopped early may have none either —
while reverting is offered exactly while a snapshot is held. The report MUST arrive with the CV the workspace already re-reads after a turn,
without an additional poll, and the fit analysis it sits above MUST be recomputed once the run ends,
whether or not the run made any edits.

#### Scenario: The report appears after a run

- **WHEN** an autopilot run finishes
- **THEN** the workspace's Verdict panel shows each requirement with its outcome above the fit analysis

#### Scenario: Another run can be started after an undo

- **WHEN** the last run has been undone and the report cleared
- **THEN** the panel still offers to start a run

#### Scenario: The fit analysis is refreshed, not left alone

- **WHEN** a run finishes
- **THEN** the cached fit analysis shown beneath the report reflects that run's outcome, not the value shown before it started
