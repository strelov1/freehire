## ADDED Requirements

### Requirement: The explaining page previews the signal with the components that render it

The `/features/ghost-jobs` landing SHALL illustrate the signal by rendering the same components the
product renders, fed illustrative payloads, and MUST NOT reproduce their markup as a copy.

A copy is stale the moment the component is redesigned, and the page then describes an interface
that no longer exists — to a reader who has come to it precisely because they did not understand
what they saw. Screenshots fail the same way, and additionally freeze one theme and one employer's
name.

#### Scenario: The preview follows a redesign

- **WHEN** the job page's presentation of the signal changes shape
- **THEN** the landing's preview changes with it, because it renders the same component rather than a copy of its markup

#### Scenario: The preview carries the caveat

- **WHEN** a reader reaches the landing's preview of the signal
- **THEN** the observations-not-accusations caveat is stated on the page beside it

## MODIFIED Requirements

### Requirement: The interface states criteria, never an accusation

The web SPA SHALL present the signal as a hedged statement — wording that the job may be inactive,
never that it is a ghost or that the employer is not hiring — accompanied by the number of criteria
that fired out of the total.

In a list card the presentation SHALL be a text chip carrying that wording and scale. On the job page
it SHALL instead be a single row: a segmented gauge, the hedged wording, the scale, and a disclosure
control that expands the full checklist in place. The checklist SHALL NOT be expanded by default, and
the job page SHALL carry the signal exactly once — the row replaces both the card-style chip and the
separate always-open panel.

The expanded checklist SHALL account for every criterion the classifier weighs. Each criterion that
fired SHALL be listed, carrying the facts behind it where the payload supplies any — for a criterion
whose firing IS the fact the interface SHALL add nothing further.

The criteria that did not fire SHALL be named together in a single line, rather than one line each,
and that line SHALL claim only that they were not OBSERVED. It MUST NOT state that there is no data
on them: the payload reports which criteria fired and never distinguishes a criterion checked and
found clear from one with nothing to check, so `evergreen_posting` — derived from a class computed
for every job — is never a criterion the system lacks data on. This is the same constraint that
governs the gauge's uncoloured segments, and prose beside the gauge must not concede what the colour
withholds.

The checklist SHALL carry a link to the page explaining the signal, which states the standing caveat
that these are observations about the posting rather than a claim about the employer.

The gauge SHALL have one segment per criterion the classifier weighs and SHALL colour exactly the
number that fired, in a tone that escalates with that number. An uncoloured segment SHALL carry no
claim about the criterion behind it: the served payload distinguishes a criterion that fired from one
that did not, and never a criterion checked and found clear from one with no data at all, so the
gauge MUST NOT render an uncoloured segment in a tone that reads as reassurance.

The word *ghost* SHALL remain internal to the codebase — package name, API field, criterion codes —
and MUST NOT appear in the interface. "Open 240 days, found only on an aggregator" is a claim about
observable facts; "ghost job" is a claim about an employer's intent, which the system cannot
observe.

Where the ghost signal is present it SHALL supersede the reality badge, which would otherwise
restate the `evergreen_posting` criterion as a second, louder badge for the same fact. Where the
ghost level is `none`, the reality badge renders unchanged.

#### Scenario: The chip is hedged and carries the scale

- **WHEN** a job reaches level `possible` with two of four criteria
- **THEN** the card shows a hedged chip and a two-of-four scale, and no accusatory wording

#### Scenario: The job page states the signal once, as a gauge row

- **WHEN** a user opens a job page at level `possible` with two of four criteria
- **THEN** one row renders with two of four segments coloured, the hedged wording and the scale, and no separate panel repeating the same signal

#### Scenario: The checklist is disclosed on demand

- **WHEN** a user opens a job page carrying the signal
- **THEN** the criteria are collapsed behind a disclosure control, and activating it accounts for every criterion and links to the explanation

#### Scenario: The checklist explains what did not fire

- **WHEN** a user expands the checklist on a job whose outcome criteria did not fire
- **THEN** one line names those criteria as not observed, so the reader can see why the level is not higher

#### Scenario: The summary does not claim an absence of data

- **WHEN** the criteria that did not fire include one the system evaluates for every job
- **THEN** the line says they were not observed, and does not say that there is no data on them

#### Scenario: The caveat is reachable from the checklist

- **WHEN** a user expands the checklist
- **THEN** it links to the page explaining the signal, where the observations-not-accusations caveat and the limits of each criterion are stated

#### Scenario: An uncoloured segment claims nothing

- **WHEN** a job fires two of four criteria and the other two did not fire
- **THEN** the two uncoloured segments render in a neutral tone that does not read as those criteria having been checked and cleared

#### Scenario: The tone escalates with the count

- **WHEN** one job fires one criterion and another fires four
- **THEN** the four-of-four gauge renders in a stronger tone than the one-of-four gauge

#### Scenario: The ghost chip replaces the reality chip

- **WHEN** a job is both `likely-evergreen` and at ghost level `possible`
- **THEN** only the ghost signal renders, and the evergreen fact appears inside its checklist

#### Scenario: Reality is unaffected where ghost is silent

- **WHEN** a job is `likely-evergreen` and its ghost level is `none`
- **THEN** the reality badge renders exactly as before
