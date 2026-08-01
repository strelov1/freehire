## ADDED Requirements

### Requirement: Avoiding a skill from the match block

The claim row SHALL offer, beside the action that claims the skill, an action that records the skill
as one the viewer wants to avoid — written to the profile's excluded-skills set. Because the row
carries two answers, it SHALL name the skill rather than pose a yes/no question. Avoiding a skill
SHALL remove it from the profile's skills, mirroring the rule that claiming one removes it from the
excluded set: a skill is never in both lists.

#### Scenario: The row offers both answers

- **WHEN** the viewer presses a Missing or Close chip
- **THEN** the row SHALL name that skill and offer both an action that adds it to the profile's
  skills and an action that adds it to the profile's avoided skills

#### Scenario: Avoiding writes the excluded set

- **WHEN** the viewer avoids `wordpress`
- **THEN** the saved profile SHALL carry `wordpress` among its excluded skills and not among its
  skills

#### Scenario: Avoiding a skill that was somehow held clears the contradiction

- **WHEN** the profile holds `php` among its skills and the viewer avoids `php`
- **THEN** the saved profile SHALL carry `php` only among its excluded skills

### Requirement: The match does not move when a skill is avoided

Avoiding a skill SHALL leave the coverage percentage, the progress bar and the three chip groups
exactly as they were. The server computes the match from the profile's skills alone, so an avoided
skill is still a skill the candidate does not have, and the block MUST NOT imply otherwise by
re-scoring or by dropping the chip from Missing.

#### Scenario: Coverage is unchanged

- **WHEN** the viewer avoids a skill from the Missing group
- **THEN** the coverage percentage and the counts SHALL read exactly what they did before, and the
  skill SHALL remain in the Missing group

#### Scenario: No match request is issued

- **WHEN** an avoid is written successfully
- **THEN** the block SHALL NOT refetch the match, there being nothing in it that could have changed

### Requirement: An avoided skill is marked wherever it appears

A chip naming a skill in the viewer's excluded set SHALL render as avoided — visually distinct from
an ordinary missing skill and marked as such for assistive technology. The marking SHALL be derived
from the profile the block already holds, so it appears on every job asking for that skill without
any additional request.

#### Scenario: The mark survives to another job

- **WHEN** the viewer avoids `wordpress` on one job and opens another job that also asks for
  `wordpress`
- **THEN** that job's `wordpress` chip SHALL render as avoided, with no further request made to
  obtain the avoided set

#### Scenario: The mark is announced, not merely drawn

- **WHEN** a chip renders as avoided
- **THEN** its accessible name SHALL say the skill is one the viewer avoids

### Requirement: An avoided skill can be un-avoided where it was marked

Pressing an avoided chip SHALL open the row offering to claim the skill or to stop avoiding it.
Stopping SHALL remove the skill from the excluded set and leave the skills set untouched, and the
chip SHALL return to an ordinary missing chip.

#### Scenario: The mark is lifted from the block

- **WHEN** the viewer presses an avoided chip and chooses to stop avoiding it
- **THEN** the skill SHALL leave the profile's excluded skills and the chip SHALL render as an
  ordinary missing skill

#### Scenario: An avoided skill can still be claimed

- **WHEN** the viewer presses an avoided chip and chooses to claim it instead
- **THEN** the skill SHALL be added to the profile's skills and removed from its excluded skills,
  and the chip SHALL move to You have

### Requirement: An avoid is confirmed, reversible and rolled back on failure

A successful avoid SHALL be confirmed by a line naming the skill and what happened to it, offering
undo — the same affordance a claim gets, and distinguishable from it in wording. A failed write
SHALL leave the profile and the chip as they were and state the failure.

#### Scenario: Confirmation distinguishes the two writes

- **WHEN** the viewer avoids `wordpress`
- **THEN** the confirmation SHALL say the skill was added to the skills the viewer avoids, not that
  it was added to their profile skills

#### Scenario: Undo lifts the avoid

- **WHEN** the viewer undoes an avoid
- **THEN** the skill SHALL leave the excluded set and the chip SHALL stop rendering as avoided

#### Scenario: A failed avoid changes nothing

- **WHEN** the profile write for an avoided skill fails
- **THEN** the chip SHALL render as it did before, no confirmation SHALL be shown, and an error
  naming the failure SHALL be shown
