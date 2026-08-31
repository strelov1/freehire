## ADDED Requirements

### Requirement: Starting a tailoring session draws on the daily session allowance

The system SHALL consume one tailoring-session allowance when a tailoring bootstrap
creates a NEW session for a vacancy, before any CV is copied or any model is called.
Returning to a session that already exists SHALL consume nothing, because the workspace
is addressed by vacancy and a reload re-runs the bootstrap — charging that would bill a
user for pressing refresh. A bootstrap refused for want of allowance SHALL create no CV,
no conversation and no model call.

#### Scenario: First tailoring of a vacancy within allowance

- **WHEN** a user with tailoring allowance remaining bootstraps a vacancy they have not
  tailored for
- **THEN** the session is created and one tailoring-session allowance is consumed

#### Scenario: Returning to an existing session

- **WHEN** a user reloads the workspace for a vacancy they have already bootstrapped
- **THEN** the existing CV and conversation are returned and no allowance is consumed,
  regardless of what remains

#### Scenario: Bootstrap refused for want of allowance

- **WHEN** a user who has spent today's tailoring allowance bootstraps a vacancy they
  have not tailored for
- **THEN** the system responds `402`, and no CV, conversation or model call is created

#### Scenario: Bootstrap that fails returns the allowance

- **WHEN** the bootstrap consumes allowance and then fails before a usable session exists
- **THEN** the allowance is returned and the user may try again

### Requirement: A tailoring session is bounded by a turn ceiling

The system SHALL bound how many assistant turns one tailoring session may run, separately
from how many sessions a day may be started. A turn beyond the ceiling SHALL be refused
with `402` naming the session; the session, its CV and its transcript SHALL be left
intact. The user SHALL be able to continue by consuming another tailoring-session
allowance, which extends that session by one further ceiling's worth of turns.

The two bounds exist because they stop different things. The daily session count bounds
how many vacancies a user works on; the turn ceiling bounds how deep one of them goes.
Measured on production, the median session ran 2.7 turns and one ran 54 — a session count
alone leaves that session unbounded, which is the shape of the hole this replaces.

#### Scenario: Turn beyond the ceiling

- **WHEN** a user sends a turn in a tailoring session that has reached its turn ceiling
- **THEN** the system responds `402`, no model call is made, and the session's CV and
  transcript are unchanged

#### Scenario: Continuing a session past its ceiling

- **WHEN** a user with tailoring allowance remaining elects to continue a session that
  reached its ceiling
- **THEN** one tailoring-session allowance is consumed and the session may run one
  further ceiling's worth of turns

#### Scenario: Continuing with no allowance left

- **WHEN** a user who has spent today's tailoring allowance elects to continue a session
  that reached its ceiling
- **THEN** the system responds `402` and the session remains readable and unchanged

#### Scenario: A ceiling belongs to one session

- **WHEN** a user runs a session to its ceiling and then bootstraps a different vacancy
  within their remaining allowance
- **THEN** the new session starts with a full turn ceiling of its own

## MODIFIED Requirements

### Requirement: Tailoring starts a job-bound copy of the base CV

The system SHALL, on a tailoring bootstrap request for a vacancy, reach exactly ONE tailored CV per
(user, vacancy): it returns the caller's existing copy for that vacancy when one exists, and
otherwise creates a new CV row bound to it (`cvs.job_id` set) whose document is copied from the
user's base CV (`job_id = NULL`). It SHALL return the tailored CV id, the base CV id, and the
cached fit analysis. Both ids SHALL be the CVs' unguessable ids. The base CV MUST remain unchanged
by the bootstrap, and the tailored CV MUST be owner-scoped to the requesting user.

Repeating the request MUST also reach the SAME conversation: the workspace is addressed by vacancy,
not by CV, so a reload re-runs this request, and minting a second conversation would rebind the CV
and orphan everything already said in the first. A bound session id that no longer resolves to a
conversation counts as none, and a fresh one is minted.

#### Scenario: Bootstrap creates a tailored copy bound to the vacancy

- **WHEN** a signed-in beta user requests tailoring for a vacancy and already has a base CV
- **THEN** a new CV is created with `job_id` set to that vacancy, its document equals the base CV's document, and the response returns both ids plus the cached analysis

#### Scenario: Repeating the bootstrap reaches the same CV and conversation

- **WHEN** the bootstrap is requested a second time for the same vacancy
- **THEN** it returns the CV and the conversation the first request produced, and no second CV, conversation or allowance consumption occurs

#### Scenario: Another vacancy gets its own copy

- **WHEN** the bootstrap is requested for a different vacancy
- **THEN** a separate tailored CV is created for it

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
