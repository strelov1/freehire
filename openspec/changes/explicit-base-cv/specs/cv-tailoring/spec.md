## ADDED Requirements

### Requirement: A tailored copy whose vacancy was deleted stays a tailored copy

Whether a CV is a tailored copy SHALL be recorded when it is created, not inferred from whether it
still links to a vacancy. Deleting a vacancy removes the link but SHALL NOT change what the CV is:
an orphaned tailored copy MUST NOT become the seed for later tailoring, the baseline for a
comparison, or the CV a base lookup returns.

#### Scenario: A pruned vacancy does not promote its tailored copy

- **WHEN** a vacancy with a tailored copy is deleted and the user then requests tailoring for a
  different vacancy
- **THEN** the new copy is seeded from the user's own non-tailored CV, not from the orphan

#### Scenario: The base lookup ignores an orphan

- **WHEN** a user has a non-tailored CV and an orphaned tailored copy edited more recently
- **THEN** the base lookup returns the non-tailored CV

#### Scenario: An orphan does not count as a base

- **WHEN** a user's only vacancy-less CV is an orphaned tailored copy
- **THEN** the user is treated as having no base CV

## MODIFIED Requirements

### Requirement: Tailoring starts a job-bound copy of the base CV

The system SHALL, on a tailoring bootstrap request for a vacancy, create a new CV row bound to
that vacancy (`cvs.job_id` set) whose document is copied from the user's base CV, and SHALL return
the tailored CV id, the base CV id, and the cached fit analysis. Both ids SHALL be the CVs'
unguessable ids. The base CV MUST remain unchanged by the bootstrap, and the tailored CV MUST be
owner-scoped to the requesting user.

The base CV SHALL be the user's most recently edited **non-tailored** CV. A user may own several
non-tailored CVs, so "the base" is a derived choice among them rather than a unique row; what it
MUST NOT include is a tailored copy that merely lost its vacancy link.

#### Scenario: Bootstrap creates a tailored copy bound to the vacancy

- **WHEN** a signed-in beta user requests tailoring for a vacancy and already has a base CV
- **THEN** a new CV is created with `job_id` set to that vacancy, its document equals the base CV's document, and the response returns both ids plus the cached analysis

#### Scenario: The base CV is untouched by bootstrap

- **WHEN** the tailoring bootstrap creates a tailored copy
- **THEN** the base CV's document and `updated_at` are unchanged

#### Scenario: The returned ids are not guessable

- **WHEN** the bootstrap responds
- **THEN** `tailor_cv_id` and `base_cv_id` are random ids, and neither can be derived from the other or from any previously issued id

#### Scenario: The newest non-tailored CV wins

- **WHEN** a user owns several non-tailored CVs
- **THEN** the bootstrap copies the most recently edited one

#### Scenario: An orphaned tailored copy is not a candidate base

- **WHEN** the user's most recently edited vacancy-less CV is an orphaned tailored copy
- **THEN** the bootstrap copies a non-tailored CV instead

### Requirement: The base CV is seeded from the structured résumé when absent

The system SHALL, when the user has no base CV at tailoring time, seed one from the stored
structured résumé using the existing deterministic seed mapping, persist it as a non-tailored CV,
and then create the tailored copy from it. When no structured résumé is available, the bootstrap
MUST fail with a client error that tells the user to add a résumé first, and MUST NOT create any
CV row.

"No base CV" SHALL mean the user owns no non-tailored CV. A user whose only vacancy-less CV is an
orphaned tailored copy has no base CV and SHALL get one seeded.

#### Scenario: A first-time user gets a base CV seeded from their résumé

- **WHEN** a beta user with a stored structured résumé but no base CV requests tailoring
- **THEN** a base CV is seeded from the structured résumé and a tailored copy is created from it

#### Scenario: Tailoring without a résumé is refused

- **WHEN** a beta user with no stored résumé requests tailoring
- **THEN** the request fails with a 409 telling them to add a résumé, and no CV row is created

#### Scenario: An orphan does not stand in for a missing base

- **WHEN** a user whose only vacancy-less CV is an orphaned tailored copy requests tailoring
- **THEN** a base CV is seeded from their résumé, and the orphan is left untouched
