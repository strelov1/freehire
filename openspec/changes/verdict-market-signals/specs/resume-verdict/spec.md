## ADDED Requirements

### Requirement: Transferable/adjacent skill status

The verdict SHALL classify a top-20 role skill the CV does not hold exactly (neither
`strong` nor `hidden`) as `adjacent` when the CV holds a skill listed as adjacent to
it in a curated adjacency dictionary; otherwise it remains `missing`. Classification
precedence SHALL be `strong` → `hidden` → `adjacent` → `missing`. An `adjacent`
skill SHALL NOT count toward must-have coverage or stack-match (the exact skill is
absent) — it is surfaced as a close, reframe-able gap rather than a hard miss. The
classification SHALL be deterministic and require no LLM.

#### Scenario: Adjacent when a close skill is held
- **WHEN** the role wants "rest-apis", the CV lacks it but lists "fastapi", and the adjacency dictionary maps rest-apis → {fastapi}
- **THEN** the skill's status is `adjacent`

#### Scenario: Missing when no close skill is held
- **WHEN** the role wants "rust", the CV holds no skill adjacent to rust
- **THEN** the skill's status is `missing`

#### Scenario: Adjacent does not inflate coverage
- **WHEN** a must-have role skill is `adjacent` (not strong or hidden)
- **THEN** it does not count toward `must_have_covered` or `stack_match_percent`

### Requirement: Typed, actionable per-skill advice

Each non-`strong` skill status SHALL carry a concrete, status-specific advice line:
`adjacent` SHALL name the close skill the candidate holds and prompt reframing or
ramp-up; `hidden` SHALL prompt surfacing the skill in the Skills section; `missing`
SHALL prompt learning it and evidencing it. `strong` SHALL carry no advice. Advice
SHALL be deterministic templates.

#### Scenario: Adjacent advice names the held skill
- **WHEN** "rest-apis" is `adjacent` because the CV lists "fastapi"
- **THEN** its advice references "fastapi" as the close skill to reframe around

### Requirement: Skill-bundle rows on the verdict

The verdict SHALL include the CV's coverage of the curated skill bundles (see the
skill-bundles capability) so the candidate sees which market skill *combinations*
they cover, not only isolated skills.

#### Scenario: Bundle coverage carried on the verdict
- **WHEN** the CV covers the `genai-core` and `cloud-ops` bundles but not `web-stack`
- **THEN** the verdict's bundle rows report genai-core and cloud-ops as covered and web-stack as not covered
