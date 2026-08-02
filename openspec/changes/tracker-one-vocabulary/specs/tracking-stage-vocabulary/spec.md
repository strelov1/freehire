## ADDED Requirements

### Requirement: One owner for the stage vocabulary, its labels and its groups

The system SHALL define the application-stage vocabulary, the human label of each stage, and
the group each stage belongs to in exactly one place — `internal/userjob` — beside the tables
that already key on that vocabulary (`activeRank`, `terminalStages`, `silenceThresholds`). No
other package, and no frontend module, SHALL hold its own copy of the stage labels or of the
stage→group mapping.

The groups SHALL be, in pipeline order: `applied` (stages `applied`, `screening`, `responded`),
`interview` (`interview`), `offer` (`offer`), and `closed` (`accepted`, `rejected`,
`withdrawn`).

#### Scenario: Every stage belongs to exactly one group

- **WHEN** the stage vocabulary is enumerated against the group table
- **THEN** every stage appears in exactly one group, and every stage named by a group exists in
  the vocabulary

#### Scenario: A new stage without a group fails the build

- **WHEN** a stage is added to the vocabulary and not placed in a group
- **THEN** the binding test fails and names the missing stage, rather than the stage rendering
  as an unlabelled or invisible column

#### Scenario: Labels are defined once

- **WHEN** any surface renders a stage name to a person
- **THEN** the text comes from the single label table, so the same stage cannot read differently
  on two screens

### Requirement: The vocabulary reaches the frontend by generation, not by retyping

The system SHALL emit the stage labels and the stage→group membership into the generated
frontend contracts alongside the existing `STAGE_VALUES`, and the SPA SHALL read them from
there. The board's columns, the pipeline funnel's bands, the drawer's stage selector and the
marketing funnel on the home page SHALL all derive from the generated tables.

A type-level check SHALL assert that the generated group table covers every generated stage
value, so an omission is a failure of the required `pnpm run check` gate rather than a blank
chip at runtime.

#### Scenario: A label added in Go reaches the SPA

- **WHEN** a stage label is changed in `internal/userjob` and the contracts are regenerated
- **THEN** every SPA surface renders the new label without a second edit

#### Scenario: An uncovered stage is a type error

- **WHEN** the generated stage values contain a stage absent from the generated group table
- **THEN** `pnpm run check` fails

#### Scenario: No surface keeps a private copy

- **WHEN** the frontend is searched for stage labels or column membership
- **THEN** the only definitions are the generated ones

### Requirement: A settled application names its outcome wherever it is shown

The board SHALL collapse the three terminal stages into a single `Closed` column, and every
card in that column SHALL carry its own stage as a label, so the coarse column and the precise
stage are legible together rather than competing. Moving a card into `Closed` SHALL require the
candidate to choose which terminal stage applies, because the group does not determine it.

The drawer's stage selector SHALL present its options grouped by the same four groups, so
`Closed` reads as a heading over `Accepted`, `Rejected` and `Withdrawn` rather than as a fifth
state.

#### Scenario: A rejected card in the Closed column

- **WHEN** an application at stage `rejected` is shown on the board
- **THEN** it sits in the `Closed` column and the card carries the label `Rejected`

#### Scenario: Dropping into Closed asks for the outcome

- **WHEN** the candidate drags a card into the `Closed` column
- **THEN** they are asked which of `Accepted`, `Rejected` or `Withdrawn` applies, and declining
  to choose reverts the move

#### Scenario: The selector groups its options

- **WHEN** the candidate opens the stage selector
- **THEN** the options appear under their group headings, in pipeline order
