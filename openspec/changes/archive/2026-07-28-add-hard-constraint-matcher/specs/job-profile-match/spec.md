## ADDED Requirements

### Requirement: Hard-constraint blockers beside skill coverage

The profile-match result SHALL, when the caller's structured résumé and the job's structured requirements are available, include the deterministic hard-constraint blockers alongside the skill-coverage classification. The blockers MUST be advisory: they never hide, downrank, or filter the job. When the structured inputs are unavailable, the result MUST degrade to skill coverage only, with no blockers and no error.

#### Scenario: Blockers surface next to coverage

- **WHEN** an authenticated caller with a structured résumé views a job whose requirements they do not fully meet
- **THEN** the profile-match payload carries both the skill coverage and the unmet hard-constraint blockers, and the job remains fully visible and clickable

#### Scenario: No structured résumé degrades to coverage only

- **WHEN** the caller has no structured résumé
- **THEN** the profile-match payload carries skill coverage with no blockers and returns no error
