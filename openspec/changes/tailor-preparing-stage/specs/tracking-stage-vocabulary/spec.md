## MODIFIED Requirements

### Requirement: One owner for the stage vocabulary, its labels and its groups

The system SHALL define the application-stage vocabulary, the human label of each stage, and
the group each stage belongs to in exactly one place — `internal/userjob` — beside the tables
that already key on that vocabulary (`activeRank`, `terminalStages`, `silenceThresholds`). No
other package, and no frontend module, SHALL hold its own copy of the stage labels or of the
stage→group mapping.

The groups SHALL be, in pipeline order: `preparing` (stage `preparing`), `applied` (stages
`applied`, `screening`, `responded`), `interview` (`interview`), `offer` (`offer`), and
`closed` (`accepted`, `rejected`, `withdrawn`, `expired`).

`preparing` SHALL carry an explicit `activeRank` entry lower than every other active stage
(rank `0`, below `applied`'s `1`), rather than relying on the zero value an unranked map key
would otherwise return — the same failure `TestEveryStageIsRankedOrTerminal` exists to catch
for any stage silently missing from both `activeRank` and `terminalStages`.

`preparing` SHALL carry an entry in `silenceThresholds` — `TestSilenceThresholdsCoverExactlyTheActiveStages`
requires one for every ranked stage — but the threshold is unreachable in practice: a
`preparing` row never carries `applied_at`, and `jobtracking.TrackedJob.Silence()` (a different
package) reports no silence state at all for any row with `applied_at` unset, before it ever
consults the threshold. The vocabulary-level entry exists for the invariant's sake, not because
the value is ever read for this stage.

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

#### Scenario: Preparing sits ahead of Applied in pipeline order

- **WHEN** the groups are enumerated in pipeline order
- **THEN** `preparing` appears before `applied`, and its rank (`0`) is below `applied`'s (`1`)

#### Scenario: Preparing's silence threshold is unreachable, not absent

- **WHEN** `SilenceThresholdDays` is asked whether stage `preparing` accrues silence
- **THEN** it reports a threshold (the invariant every ranked stage carries one), but no
  `preparing` row can ever reach it: `jobtracking.TrackedJob.Silence()` returns no state for any
  row with `applied_at` unset, which is guaranteed for `preparing`

### Requirement: The vocabulary reaches the frontend by generation, not by retyping

The system SHALL emit the stage labels and the stage→group membership into the generated
frontend contracts alongside the existing `STAGE_VALUES`, and the SPA SHALL read them from
there. The board's columns, the pipeline funnel's bands, the drawer's stage selector and the
marketing funnel on the home page SHALL all derive from the generated tables — including the
new `preparing` group, requiring no hand-written second copy on any of those surfaces.

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

#### Scenario: The pipeline funnel renders a fifth band

- **WHEN** the pipeline funnel component reads the generated groups after this change
- **THEN** it renders a `Preparing` band ahead of `Applied` without a hard-coded band count
