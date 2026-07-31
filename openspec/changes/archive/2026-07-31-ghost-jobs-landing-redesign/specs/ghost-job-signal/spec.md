## MODIFIED Requirements

### Requirement: The explaining page previews the signal with the components that render it

The `/features/ghost-jobs` landing SHALL illustrate the signal by rendering the same components the
product renders, fed illustrative payloads, and MUST NOT reproduce their markup as a copy.

A copy is stale the moment the component is redesigned, and the page then describes an interface
that no longer exists — to a reader who has come to it precisely because they did not understand
what they saw. Screenshots fail the same way, and additionally freeze one theme and one employer's
name.

The page SHALL additionally state the level rule as a diagram rather than as prose: which
combinations of the two gates produce which wording, and that the strongest wording sits in exactly
one of them. Each cell's wording SHALL be derived from the rule rather than written into the
diagram, so the picture cannot caption a level the classifier stopped producing.

To derive it the frontend SHALL hold the level rule as a tested function, taking the criteria that
fired and the number of contributors, and that function SHALL live in the same module that already
declares itself the mirror of the classifier's constants. The page previously asserted the rule —
that structural evidence alone cannot reach the higher level — in a sentence no test could check, so
the claim could silently outlive the thresholds it describes. A mirrored rule can drift from the
classifier, but the constants it depends on were already mirrored in the frontend; expressing the
rule as a function makes the drift detectable instead of invisible, and gathers a mirror that was
spread across two modules into one.

#### Scenario: The preview follows a redesign

- **WHEN** the job page's presentation of the signal changes shape
- **THEN** the landing's preview changes with it, because it renders the same component rather than a copy of its markup

#### Scenario: The preview carries the caveat

- **WHEN** a reader reaches the landing's preview of the signal
- **THEN** the observations-not-accusations caveat is stated on the page beside it

#### Scenario: The ceiling is shown, not asserted

- **WHEN** a reader looks at the diagram of the two gates
- **THEN** the cell reached by posting shape alone carries the lower wording, and the strongest
  wording appears only in the cell that also required people who applied

#### Scenario: The diagram cannot caption a level the rule stopped producing

- **WHEN** the level rule changes which combination yields which wording
- **THEN** the diagram's cells change with it, because each is derived from the rule rather than
  written into the picture

#### Scenario: The level rule is pinned by test

- **WHEN** the frontend's level rule is exercised across the combinations of convergence and
  contributor gates
- **THEN** each combination yields the level the classifier defines, and no set of structural-only
  criteria yields the higher level
