## MODIFIED Requirements

### Requirement: The workspace surfaces the delta without being asked

The tailoring workspace SHALL request the delta when it opens and again after an autopilot run
completes — the two moments the document is most likely to have changed — and display the overall
change, the per-category breakdown, and the regression warning when there is one. The candidate
SHALL NOT have to trigger a check to be told their CV got worse.

The delta SHALL be surfaced in the workspace's **Score** tab, which carries the readability of the
document and the last run's log and nothing else. It MUST NOT share a tab with the job-anchored
match score: the two answer different questions against different baselines, and stacking them
under one heading is what made the previous Verdict tab unreadable.

Each category row SHALL be expandable to the line items behind its score — the individual checks,
their pass/warn/fail status and the points each one carries. The score already computes them; a
row that reports only a number tells the candidate what changed and never why.

An unavailable delta SHALL render as an absence, not as an error state.

#### Scenario: Opening the workspace shows the delta

- **WHEN** the tailoring workspace opens for a tailored CV
- **THEN** the delta is requested and its overall change and per-category breakdown are displayed

#### Scenario: A completed run refreshes the delta

- **WHEN** an autopilot run finishes
- **THEN** the delta is requested again and the displayed values reflect the run's edits

#### Scenario: The delta lives in the Score tab

- **WHEN** the user opens the Score tab
- **THEN** the ATS-readability delta and the last autopilot run's log are shown, and the job-anchored match score is not

#### Scenario: A category expands to the checks behind it

- **WHEN** the user expands a category row
- **THEN** the line items behind its score are shown, each with its status and its points

#### Scenario: An unavailable delta shows nothing rather than an error

- **WHEN** the delta is reported unavailable
- **THEN** the workspace displays no delta and no error state
