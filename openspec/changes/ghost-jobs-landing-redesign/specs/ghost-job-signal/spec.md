## MODIFIED Requirements

### Requirement: The explaining page previews the signal with the components that render it

The `/features/ghost-jobs` landing SHALL illustrate the signal by rendering the same components the
product renders, fed illustrative payloads, and MUST NOT reproduce their markup as a copy.

A copy is stale the moment the component is redesigned, and the page then describes an interface
that no longer exists — to a reader who has come to it precisely because they did not understand
what they saw. Screenshots fail the same way, and additionally freeze one theme and one employer's
name.

The preview SHALL be operable rather than fixed: the reader SHALL be able to select which criteria
have fired and how many distinct people have contributed outcome evidence, and the components SHALL
re-render from a payload assembled from that selection. The contributor control SHALL offer only the
values that mean something relative to the gate — none, one, and enough — because a free count
communicates nothing the gate does not already decide.

To drive that preview the frontend SHALL hold the level rule as a tested function rather than as
prose, deriving the level from the criteria that fired and the number of contributors, and that
function SHALL live in the same module that already declares itself the mirror of the classifier's
constants. The page today asserts the rule — that structural evidence alone cannot reach the higher
level — in a sentence no test can check, so the claim can silently outlive the thresholds it
describes. A mirrored rule can drift from the classifier, but the constants it depends on were
already mirrored in the frontend; expressing the rule as a function makes the drift detectable
instead of invisible, and gathers a mirror that was spread across two modules into one.

#### Scenario: The preview follows a redesign

- **WHEN** the job page's presentation of the signal changes shape
- **THEN** the landing's preview changes with it, because it renders the same component rather than a copy of its markup

#### Scenario: The preview carries the caveat

- **WHEN** a reader reaches the landing's preview of the signal
- **THEN** the observations-not-accusations caveat is stated on the page beside it

#### Scenario: The reader drives the preview

- **WHEN** a reader selects a different set of fired criteria in the preview
- **THEN** the same chip, gauge and checklist the product renders update to the level and scale that
  selection produces

#### Scenario: The ceiling is discovered, not asserted

- **WHEN** a reader selects both structural criteria and no contributors
- **THEN** the preview holds at the lower level, and no control offered by the preview raises it to
  the higher one

#### Scenario: A single contributor yields no count

- **WHEN** a reader sets the preview to one contributor
- **THEN** the rendered row carries no contributor count, matching the payload the product serves
  below the anonymity gate

#### Scenario: The level rule is pinned by test

- **WHEN** the frontend's level rule is exercised across the combinations of convergence and
  contributor gates
- **THEN** each combination yields the level the classifier defines, and no set of structural-only
  criteria yields the higher level
