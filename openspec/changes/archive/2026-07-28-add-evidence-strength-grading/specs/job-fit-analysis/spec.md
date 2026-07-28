## MODIFIED Requirements

### Requirement: ATS-style requirement match (Stage 1)

The first stage SHALL extract the vacancy's explicit requirements together with its role-title and
seniority signals, classify each requirement against the CV text as one of `covered`,
`synonym-only`, `missing-but-have`, or `missing-gap`, carrying a required/preferred priority, and —
for the two positive statuses (`covered`, `synonym-only`) — grade the strength of the cited evidence
as one of `metric` (an accomplishment with a number, scale, or measured outcome), `scope` (breadth
of work: teams, systems, regions), `responsibility` (clear ownership with tools or methods), or
`keyword` (the term is present but the surrounding evidence is a bare mention or duty-only). This
requirement-match table MUST be included in the served analysis and MUST NOT fabricate a skill the
CV does not evidence — a genuine gap is reported as `missing-gap`, never hidden. All model output
MUST be sanitized to the controlled vocabulary: an unknown or absent strength on a positive status
coerces to `keyword`, and the two `missing-*` statuses carry no strength.

#### Scenario: Requirement present only under a synonym

- **WHEN** the vacancy requires a skill the CV evidences under a different but equivalent term
- **THEN** that requirement is classified `synonym-only`, not `missing`

#### Scenario: Genuine gap is reported honestly

- **WHEN** the vacancy requires a skill absent from the CV with no close equivalent held
- **THEN** that requirement is classified `missing-gap` and is never presented as covered

#### Scenario: Covered requirement graded by evidence strength

- **WHEN** the CV evidences a covered requirement with a measured accomplishment (a number, scale, or outcome)
- **THEN** that requirement's `evidence_strength` is `metric`, and a covered requirement whose CV evidence is only a bare mention is graded `keyword`

#### Scenario: Out-of-vocabulary or missing strength is coerced

- **WHEN** the model returns an unrecognised or empty `evidence_strength` on a `covered` or `synonym-only` requirement
- **THEN** the served requirement's strength is coerced to `keyword`, and any strength on a `missing-*` requirement is dropped

### Requirement: Adversarial audit (Stage 3)

The final stage SHALL challenge the recruiter verdict — flagging inflated dimension scores,
strengths not supported by the CV evidence, and gaps that were glossed over — and return a
corrected verdict that the served analysis is built from. The audit MUST treat weak evidence on a
**required** requirement as weak support: a `synonym-only` match, or a `covered` match graded
`keyword`, MUST NOT by itself sustain a high `skills_coverage` score. If the audit stage fails or
does not parse, the system MUST fall back to the un-audited recruiter verdict rather than error the
request.

#### Scenario: Audit prunes an unsupported strength

- **WHEN** the recruiter stage lists a strength the CV does not actually evidence
- **THEN** the audit removes or downgrades it and the served analysis reflects the corrected verdict

#### Scenario: Audit demotes keyword-only coverage of a required skill

- **WHEN** a required requirement is `covered` only at `keyword` evidence strength (or `synonym-only`)
- **THEN** the audit does not let that match alone sustain a high `skills_coverage` score

#### Scenario: Audit stage fails

- **WHEN** the adversarial audit call fails or returns unparseable output
- **THEN** the system serves the recruiter-stage verdict and still responds `200`
