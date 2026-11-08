## ADDED Requirements

### Requirement: Deterministic tech title detection

The system SHALL provide a deterministic, curated dictionary that identifies confidently technical (software/IT) job titles by whole-word match, and MUST NOT guess: a title it cannot confidently place as technical yields no signal. The dictionary MUST only match unambiguous software/IT role terms (e.g. software developer, backend/frontend/fullstack/mobile developer, programmer, devops, sre, data scientist, machine learning engineer, system administrator, cloud/security/qa engineer) and MUST NOT contain generic terms dominated by non-software roles (e.g. bare "engineer" — which also names mechanical/manufacturing/civil/drainage roles — or bare "analyst").

#### Scenario: Confident tech title is detected

- **WHEN** a title contains a curated software/IT role term as a whole word (e.g. "Senior Software Engineer", "Web3 Developer", "System Administrator")
- **THEN** the detector reports the title as technical

#### Scenario: Non-software engineering title is not flagged

- **WHEN** a title names a non-software engineering or non-tech role (e.g. "Senior Mechanical Engineer", "Professional Engineer - Drainage", "Sales Engineer", "Senior Geologist")
- **THEN** the detector reports no tech signal

#### Scenario: Ambiguous substring does not match

- **WHEN** a tech term appears only as a substring of another word or the title carries only a shared term like bare "engineer"
- **THEN** the detector does not flag the title, matching only on word boundaries

## MODIFIED Requirements

### Requirement: Tri-state is_tech derivation

The system SHALL derive a tri-state `is_tech` signal for every job during facet derivation, from the title and the derived category, with technical evidence taking precedence over non-technical evidence. The value MUST be `true` when the derived category is a recognized technical category OR the tech-title detector flags the title, `false` when the derived category is a known non-technical category OR the non-tech detector flags the title, and otherwise unknown (absent). Technical evidence is evaluated first, so a title carrying both signals resolves to `true`. An unknown value MUST NOT be coerced to `true` or `false` — the absence is the honest state used to measure remaining coverage.

#### Scenario: Recognized tech category yields true

- **WHEN** the title resolves to a technical category (e.g. `backend`)
- **THEN** `is_tech` is `true`

#### Scenario: Detector-only tech title yields true

- **WHEN** the derived category is empty but the tech-title detector flags the title (e.g. "Senior Software Engineer")
- **THEN** `is_tech` is `true`

#### Scenario: Blacklist non-tech category yields false

- **WHEN** the derived category is one of the non-technical categories (marketing, sales, support, management)
- **THEN** `is_tech` is `false`

#### Scenario: Detector-only non-tech yields false

- **WHEN** the derived category is empty, the tech detector is silent, but the non-tech detector flags the title (e.g. "Warehouse Cleaner")
- **THEN** `is_tech` is `false`

#### Scenario: Unresolved job stays unknown

- **WHEN** no category resolves and neither the tech nor the non-tech detector fires (e.g. "Senior Mechanical Engineer")
- **THEN** `is_tech` is unknown (absent), not `true` and not `false`
