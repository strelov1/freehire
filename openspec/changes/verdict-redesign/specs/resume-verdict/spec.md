## ADDED Requirements

### Requirement: Market-anchored role-skill breakdown

The verdict SHALL include a breakdown of the selected role's top 20 in-demand
skills (the role's `skills` facet, ranked by vacancy frequency). For each skill it
SHALL report: `name`, `market_frequency` = round(vacancies listing it / role total
× 100), a `must_have` flag (true when `market_frequency` ≥ a configured threshold),
a `status`, and an `advice` line. `status` SHALL be derived from the CV's parsed
skill sets (see cv-section-parsing): `strong` when the skill is in `declared`,
`hidden` when it is in `body` but not `declared`, `missing` when it is in neither.
`advice` SHALL be a deterministic status-specific line (empty for `strong`). Every
number SHALL come from live market data and the CV text — never from an LLM.

#### Scenario: Strong when declared in the Skills section
- **WHEN** a role's top skill "go" is present in the CV's `declared` set
- **THEN** its row has `status` = `strong` and no advice

#### Scenario: Hidden when used in experience but not declared
- **WHEN** a role's top skill "kafka" is in the CV's `body` but not `declared`
- **THEN** its row has `status` = `hidden` with advice to surface it in the Skills section

#### Scenario: Missing when absent from the CV
- **WHEN** a role's top skill "rust" is in neither `declared` nor `body`
- **THEN** its row has `status` = `missing` with advice to gain and evidence it

#### Scenario: Must-have flagged by market frequency
- **WHEN** "python" appears in 62% of the role's open vacancies and the threshold is 50%
- **THEN** its row has `must_have` = true

#### Scenario: Rare skill is not must-have
- **WHEN** "cobol" appears in 3% of the role's open vacancies and the threshold is 50%
- **THEN** its row has `must_have` = false

### Requirement: Market-anchored headline stats

The verdict SHALL report three additional headline stats alongside the existing
vacancy coverage: `must_have_total` and `must_have_covered` (of the role's
must-have skills, how many the CV demonstrably holds — `strong` or `hidden`);
`stack_match_percent` = round((`strong` + `hidden` among the top 20) / 20 × 100);
and `coherence_percent` = round(|`declared` ∩ `body`| / |`declared`| × 100), which
is 0 when `declared` is empty. These SHALL be computed deterministically from the
role facets and the CV's parsed skill sets.

#### Scenario: Must-have coverage counts strong and hidden
- **WHEN** the role has 7 must-have skills and the CV holds 6 of them as `strong` or `hidden`
- **THEN** `must_have_total` = 7 and `must_have_covered` = 6

#### Scenario: Stack match is top-20 breadth
- **WHEN** 15 of the role's top 20 skills are `strong` or `hidden` in the CV
- **THEN** `stack_match_percent` = 75

#### Scenario: Coherence penalizes unbacked declared skills
- **WHEN** the CV declares 10 skills and only 7 of them also appear in the body
- **THEN** `coherence_percent` = 70

#### Scenario: Coherence with no declared skills
- **WHEN** the CV has no Skills section (`declared` is empty)
- **THEN** `coherence_percent` = 0
