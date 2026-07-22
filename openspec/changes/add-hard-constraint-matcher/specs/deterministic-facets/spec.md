## ADDED Requirements

### Requirement: Required certifications are derived deterministically from the description

`internal/jobfacts` SHALL expose `RequiredCertifications(description)` returning the canonical credential slugs the posting requires, found by scanning the description with the shared credential vocabulary (whole-word alias matching, deduped). It is computed at read where the hard-constraint evaluator runs — not stored, not an enrichment field, not a Meilisearch facet — so it is correct for the whole catalogue the moment the code ships, with no re-enrich and no backfill.

#### Scenario: A recognized credential in the description is surfaced as a slug

- **WHEN** a description says "AWS Certified Solutions Architect required"
- **THEN** `RequiredCertifications` includes the canonical slug for that credential

#### Scenario: An unrecognized credential contributes nothing

- **WHEN** a description mentions a credential outside the curated vocabulary
- **THEN** `RequiredCertifications` does not include it (dict-only, never guessed)

### Requirement: Degree-optional postings are detected deterministically

`internal/jobfacts` SHALL expose `DegreeOptional(description)` returning true when the posting offers a degree with an equivalent-experience alternative ("or equivalent experience", "degree or equivalent", and like phrasings). The hard-constraint evaluator uses this flag to skip the education blocker, so a posting that explicitly accepts equivalent experience never raises a false education blocker. The `education_level` facet itself is unchanged.

#### Scenario: "Degree or equivalent experience" is degree-optional

- **WHEN** a description says "Bachelor's degree or equivalent experience"
- **THEN** `DegreeOptional` returns true and the education blocker is skipped for that job

#### Scenario: A hard degree requirement is not degree-optional

- **WHEN** a description says "Bachelor's degree required" with no equivalent-experience alternative
- **THEN** `DegreeOptional` returns false and the education requirement is evaluated normally
