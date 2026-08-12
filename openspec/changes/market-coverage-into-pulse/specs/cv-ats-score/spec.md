## MODIFIED Requirements

### Requirement: Role keyword-match distinct from market-coverage

The system SHALL report a keyword-match: of the selected role's top in-demand
skills (the role's `skills` facet), how many appear as literal skill-tag matches
in the CV text. This SHALL be computed from the CV TEXT (not the profile's stored
skill set) and SHALL name the top missing role skills in the check's fix. The role
SHALL come from the request's facet params (Profile's CV readiness tab filter).

#### Scenario: Keyword-match counts role skills present in the CV text
- **WHEN** a role's top skills are {go, kubernetes, kafka} and the CV text contains "go" and "kafka" but not "kubernetes"
- **THEN** keyword-match reflects 2 of 3 and the fix names "kubernetes" as missing

#### Scenario: Role comes from the request filter
- **WHEN** the caller requests the report with `?category=data`
- **THEN** keyword-match uses the data role's top skills, independent of the profile's stored specializations
