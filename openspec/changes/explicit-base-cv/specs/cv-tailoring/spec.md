## ADDED Requirements

### Requirement: Exactly one base CV per user, enforced by the database

A user SHALL have at most one base CV, and that limit SHALL be enforced by the schema rather than
by the ordering of a lookup query. A second base CV for the same user MUST be rejected by the
database, not silently resolved by picking the most recently edited one.

#### Scenario: A second base CV is refused

- **WHEN** a second base CV is written for a user who already has one
- **THEN** the write fails on a uniqueness violation rather than creating it

#### Scenario: Tailored copies are unlimited

- **WHEN** a user has many tailored CVs
- **THEN** the constraint does not restrict them, however many vacancies they have tailored for

### Requirement: A tailored copy whose vacancy was deleted stays a tailored copy

Deleting a vacancy SHALL NOT convert its tailored copies into base CVs. A tailored CV whose
vacancy row is removed loses its vacancy link, and the system SHALL continue to treat it as a
tailored copy — it MUST NOT become the seed for later tailoring, the baseline for a comparison, or
the CV a base lookup returns.

#### Scenario: A pruned vacancy does not promote its tailored copy

- **WHEN** a vacancy with a tailored copy is deleted and the user then requests tailoring for a
  different vacancy
- **THEN** the new copy is seeded from the user's real base CV, not from the orphaned tailored copy

#### Scenario: The base lookup ignores an orphan

- **WHEN** a user has a base CV and an orphaned tailored copy edited more recently
- **THEN** the base lookup returns the base CV

## MODIFIED Requirements

### Requirement: Tailoring starts a job-bound copy of the base CV

The system SHALL, on a tailoring bootstrap request for a vacancy, create a new CV row bound to
that vacancy (`cvs.job_id` set) whose document is copied from the user's base CV — the CV **marked
as the base**, not merely one whose vacancy link is empty — and SHALL return the tailored CV id,
the base CV id, and the cached fit analysis. Both ids SHALL be the CVs' unguessable ids. The base
CV MUST remain unchanged by the bootstrap, and the tailored CV MUST be owner-scoped to the
requesting user.

#### Scenario: Bootstrap creates a tailored copy bound to the vacancy

- **WHEN** a signed-in beta user requests tailoring for a vacancy and already has a base CV
- **THEN** a new CV is created with `job_id` set to that vacancy, its document equals the base CV's document, and the response returns both ids plus the cached analysis

#### Scenario: The base CV is untouched by bootstrap

- **WHEN** the tailoring bootstrap creates a tailored copy
- **THEN** the base CV's document and `updated_at` are unchanged

#### Scenario: The returned ids are not guessable

- **WHEN** the bootstrap responds
- **THEN** `tailor_cv_id` and `base_cv_id` are random ids, and neither can be derived from the other or from any previously issued id

#### Scenario: An orphaned tailored copy is not a candidate base

- **WHEN** the user's most recently edited vacancy-less CV is an orphaned tailored copy
- **THEN** the bootstrap copies the base CV's document, not the orphan's

### Requirement: The base CV is seeded from the structured résumé when absent

The system SHALL, when the user has no base CV at tailoring time, seed one from the stored
structured résumé using the existing deterministic seed mapping, persist it **marked as the base
CV**, and then create the tailored copy from it. When no structured résumé is available, the
bootstrap MUST fail with a client error that tells the user to add a résumé first, and MUST NOT
create any CV row.

"No base CV" SHALL mean no CV marked as the base. A user whose only vacancy-less CV is an orphaned
tailored copy has no base CV and SHALL get one seeded.

#### Scenario: A first-time user gets a base CV seeded from their résumé

- **WHEN** a beta user with a stored structured résumé but no base CV requests tailoring
- **THEN** a base CV is seeded from the structured résumé and a tailored copy is created from it

#### Scenario: Tailoring without a résumé is refused

- **WHEN** a beta user with no stored résumé requests tailoring
- **THEN** the request fails with a 409 telling them to add a résumé, and no CV row is created

#### Scenario: An orphan does not stand in for a missing base

- **WHEN** a user whose only vacancy-less CV is an orphaned tailored copy requests tailoring
- **THEN** a base CV is seeded from their résumé, and the orphan is left untouched
