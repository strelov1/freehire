## ADDED Requirements

### Requirement: A settled outcome for an application nobody answered

The vocabulary SHALL carry a terminal stage `expired`, labelled `Expired`, meaning the
application ended without an answer — the employer never replied, or the posting went away.

It SHALL be settable only by the candidate, through the same stage-setting surfaces as every
other stage. No worker, schedule or threshold SHALL move an application into it: silence is
already reported by the silence state, which measures how long an employer has been quiet,
while `expired` records the candidate's conclusion that no answer is coming. One is computed
and reversible by the passage of a reply; the other is a decision.

Being terminal, `expired` SHALL have no forward rank and no silence threshold: an application
does not pass through it on the way anywhere, and a settled application waits on nobody. The
existing rule that automatic advancement never enters or leaves a terminal stage therefore
applies to it unchanged, so mail classified after the candidate marked an application expired
cannot resurrect it.

#### Scenario: The candidate marks an unanswered application expired

- **WHEN** the candidate sets stage `expired` on an application
- **THEN** the application is settled, shows in the `Closed` group labelled `Expired`, and
  reports no silence state

#### Scenario: Nothing sets it automatically

- **WHEN** an application passes any silence threshold, however long ago it was sent
- **THEN** its stage is unchanged, and only its silence state reports the delay

#### Scenario: A late reply cannot reopen it

- **WHEN** employer mail arrives for an application already at stage `expired`
- **THEN** the automatic advance declines, exactly as it does for the other terminal stages

## MODIFIED Requirements

### Requirement: One owner for the stage vocabulary, its labels and its groups

The system SHALL define the application-stage vocabulary, the human label of each stage, and
the group each stage belongs to in exactly one place — `internal/userjob` — beside the tables
that already key on that vocabulary (`activeRank`, `terminalStages`, `silenceThresholds`). No
other package, and no frontend module, SHALL hold its own copy of the stage labels or of the
stage→group mapping.

The groups SHALL be, in pipeline order: `applied` (stages `applied`, `screening`, `responded`),
`interview` (`interview`), `offer` (`offer`), and `closed` (`accepted`, `rejected`,
`withdrawn`, `expired`).

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

### Requirement: A settled application names its outcome wherever it is shown

The board SHALL collapse the terminal stages into a single `Closed` column, and every card in
that column SHALL carry its own stage as a label, so the coarse column and the precise stage
are legible together rather than competing. Moving a card into `Closed` SHALL require the
candidate to choose which terminal stage applies, because the group does not determine it.

The drawer's stage selector SHALL present its options grouped by the same four groups, so
`Closed` reads as a heading over `Accepted`, `Rejected`, `Withdrawn` and `Expired` rather than
as a fifth state.

#### Scenario: A rejected card in the Closed column

- **WHEN** an application at stage `rejected` is shown on the board
- **THEN** it sits in the `Closed` column and the card carries the label `Rejected`

#### Scenario: An expired card names its own outcome

- **WHEN** an application at stage `expired` is shown on the board
- **THEN** it sits in the `Closed` column and the card carries the label `Expired`, so it does
  not read as a rejection

#### Scenario: Dropping into Closed asks for the outcome

- **WHEN** the candidate drags a card into the `Closed` column
- **THEN** they are asked which terminal stage applies, and declining to choose reverts the move

#### Scenario: The selector groups its options

- **WHEN** the candidate opens the stage selector
- **THEN** the options appear under their group headings, in pipeline order
