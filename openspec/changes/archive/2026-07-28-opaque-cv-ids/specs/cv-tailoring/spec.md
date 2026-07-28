## MODIFIED Requirements

### Requirement: Tailoring starts a job-bound copy of the base CV

The system SHALL, on a tailoring bootstrap request for a vacancy, create a new CV row bound to
that vacancy (`cvs.job_id` set) whose document is copied from the user's base CV (`job_id = NULL`),
and SHALL return the tailored CV id, the base CV id, and the cached fit analysis. Both ids SHALL be
the CVs' unguessable ids. The base CV MUST remain unchanged by the bootstrap, and the tailored CV
MUST be owner-scoped to the requesting user.

#### Scenario: Bootstrap creates a tailored copy bound to the vacancy

- **WHEN** a signed-in beta user requests tailoring for a vacancy and already has a base CV
- **THEN** a new CV is created with `job_id` set to that vacancy, its document equals the base CV's document, and the response returns both ids plus the cached analysis

#### Scenario: The base CV is untouched by bootstrap

- **WHEN** the tailoring bootstrap creates a tailored copy
- **THEN** the base CV's document and `updated_at` are unchanged

#### Scenario: The returned ids are not guessable

- **WHEN** the bootstrap responds
- **THEN** `tailor_cv_id` and `base_cv_id` are random ids, and neither can be derived from the other or from any previously issued id
