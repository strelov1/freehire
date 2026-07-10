## ADDED Requirements

### Requirement: Curated skill-bundle coverage

The system SHALL define a curated dictionary of market-recognised skill bundles
(each a bundle name → a set of member skill slugs) and compute a CV's coverage of
each bundle from the CV's parsed skill set: `covered` = the count of member skills
present, `total` = the bundle size. A bundle SHALL be reported as covered when
`covered / total` ≥ a configured threshold (`BundleCoveredPct`). The computation
SHALL be pure, deterministic, and require no LLM (mirroring the other dictionaries).

#### Scenario: Bundle fully covered
- **WHEN** the `cloud-ops` bundle is {docker, kubernetes, ci-cd, terraform} and the CV lists all four
- **THEN** its coverage is `covered` = 4, `total` = 4, and it is reported as covered

#### Scenario: Bundle partially covered below threshold
- **WHEN** the `web-stack` bundle has 5 members, the CV lists 1 of them, and the threshold is 50%
- **THEN** its coverage is `covered` = 1 and it is NOT reported as covered

#### Scenario: Deterministic
- **WHEN** the same CV skill set is scored against the bundles twice
- **THEN** the bundle coverage is identical
