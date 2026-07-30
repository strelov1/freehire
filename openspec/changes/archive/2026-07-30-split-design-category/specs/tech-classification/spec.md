## MODIFIED Requirements

### Requirement: Tri-state is_tech derivation

The system SHALL derive a tri-state `is_tech` signal for every job during facet derivation, from the title and the derived category, with technical evidence taking precedence over non-technical evidence. The value MUST be `true` when the derived category is a recognized technical category OR the tech-title detector flags the title, `false` when the derived category is a known non-technical category OR the non-tech detector flags the title, and otherwise unknown (absent). Technical evidence is evaluated first, so a title carrying both signals resolves to `true`. An unknown value MUST NOT be coerced to `true` or `false` — the absence is the honest state used to measure remaining coverage. The known non-technical categories include `engineering_design`: engineering draughting and design work is engineering, but it is not the IT work this catalogue serves, so it derives `false` rather than the `true` it inherited while it shared the `design` category.

#### Scenario: Recognized tech category yields true
- **WHEN** the title resolves to a technical category (e.g. `backend`)
- **THEN** `is_tech` is `true`

#### Scenario: Detector-only tech title yields true
- **WHEN** the derived category is empty but the tech-title detector flags the title (e.g. "Senior Software Engineer")
- **THEN** `is_tech` is `true`

#### Scenario: Blacklist non-tech category yields false
- **WHEN** the derived category is one of the non-technical categories (marketing, sales, support, management, engineering_design)
- **THEN** `is_tech` is `false`

#### Scenario: Engineering design yields false
- **WHEN** a job titled "Mechanical Design Engineer" resolves to `engineering_design`
- **THEN** `is_tech` is `false`, where the same title previously derived `true` through the `design` category

#### Scenario: Detector-only non-tech yields false
- **WHEN** the derived category is empty, the tech detector is silent, but the non-tech detector flags the title (e.g. "Warehouse Cleaner")
- **THEN** `is_tech` is `false`

#### Scenario: Unresolved job stays unknown
- **WHEN** no category resolves and neither the tech nor the non-tech detector fires (e.g. "Drainage Engineer" — a discipline neither dictionary names)
- **THEN** `is_tech` is unknown (absent), not `true` and not `false`
