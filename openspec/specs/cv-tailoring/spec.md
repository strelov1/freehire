# cv-tailoring Specification

## Purpose
TBD - created by archiving change add-cv-tailoring. Update Purpose after archive.
## Requirements
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
- **THEN** it returns the CV and the conversation the first request produced, and no second CV, conversation or debit is created

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

### Requirement: The bootstrap response flags a first-time cold start

The system SHALL NOT require a cached fit analysis to exist before starting tailoring. The
bootstrap response SHALL carry a boolean indicating whether this call just created a new tailored
CV for a (user, vacancy) pair that had none yet — reusing the same "just created" signal the
bootstrap already computes today — so the workspace can tell a genuine cold start from re-opening
an existing CV. Nothing about analysis or autopilot is started by the bootstrap itself; the flag
only signals the workspace to trigger the autopilot run automatically (see `tailor-autopilot`'s
cold-start requirements), instead of gating or sequencing anything server-side at bootstrap time.

Repeating the bootstrap for the same (user, vacancy) pair MUST NOT report a cold start a second
time — the existing idempotency rule (same CV, same conversation, no second debit) extends to this
flag: it is true on at most one bootstrap call per (user, vacancy).

#### Scenario: A first-time bootstrap flags a cold start

- **WHEN** a beta user with a base CV requests tailoring for a vacancy they have never tailored
  before
- **THEN** the bootstrap response returns immediately with the new tailored CV, the session id, and
  the cold-start flag set to true, without waiting on any analysis or autopilot run

#### Scenario: Repeating the bootstrap does not flag a cold start again

- **WHEN** the bootstrap is requested again for a (user, vacancy) pair that already has a tailored CV
- **THEN** the existing CV and conversation are returned and the cold-start flag is false

### Requirement: CV edits are applied as sanitized field-level patches

The system SHALL apply CV edits as batches of typed path operations rather than as named
field-level patches. An operation is one of `set`, `insert`, `remove` or `move` against a path
into the CV's editable state, and the four of them together MUST reach every field of the
document — not only the summary, bullets, skill groups and stack line that the earlier named
vocabulary happened to cover. Every batch MUST be applied through a pure transform and then
passed through the document sanitizer (length and cardinality bounds, prompt-injection guard)
before persistence. A batch containing an operation that addresses a field or index that does not
exist MUST be rejected as a client error, and MUST NOT mutate the document at all.

The set of addressable paths MUST be derived from the document's own structure rather than
maintained as a list beside it, so a field added to the document is addressable without a second
edit somewhere else. A vocabulary maintained by hand drifts: an op once went missing from the
schema a model reads, and a model that cannot see an operation cannot use it.

#### Scenario: A bullet is added to one experience entry, leaving others intact

- **WHEN** an operation inserts a bullet into experience entry 0
- **THEN** entry 0 gains the bullet, every other section of the document is byte-for-byte unchanged, and the result is sanitized before saving

#### Scenario: Out-of-range addressing is rejected

- **WHEN** an operation targets an experience index that does not exist
- **THEN** the batch fails with a 422 and the stored document is unchanged

#### Scenario: Bullets can be reordered by relevance

- **WHEN** operations move an entry's bullets into a given order
- **THEN** that entry's bullets appear in the requested order and no bullet is added or dropped

#### Scenario: A field the old vocabulary could not reach is editable

- **WHEN** an operation sets a certification's issuer, a language's level, or a page margin
- **THEN** the change applies, without any operation having been added for that field

### Requirement: Tailoring context is served from the cached analysis

The system SHALL expose a read that returns the tailoring context for a tailored CV — the verdict,
the recommendation, the per-dimension comments, and the requirement coverage split into
`missing-have` (evidence exists but the CV omits it) and `missing-gap` (a genuine gap) — sourced
from the cached fit analysis with no LLM recompute. The read MUST be owner-scoped and MUST require
an authenticated caller (session cookie or API key).

#### Scenario: The consumer can distinguish reframe-able requirements from genuine gaps

- **WHEN** an authenticated owner reads the tailoring context for their tailored CV
- **THEN** the response lists the verdict, recommendation, dimension comments, and requirements labelled `missing-have` versus `missing-gap`

### Requirement: Tailoring is beta-gated

The system SHALL gate every tailoring endpoint behind beta access — the union of the CV builder
gate and the agent's beta-tester flag.

#### Scenario: A non-beta user cannot reach tailoring

- **WHEN** a signed-in non-beta user calls the tailoring bootstrap
- **THEN** the request is refused by the beta gate

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

### Requirement: The tailoring agent is told wording is already aligned

The tailoring system prompt SHALL tell the model that skill surface forms already
match the vacancy and that it MUST NOT rename skills for wording.

#### Scenario: Prompt forbids synonym hunting

- **WHEN** a tailoring session starts
- **THEN** the system prompt includes an instruction that wording alignment is
  done and must not be repeated

### Requirement: Tailoring places the vacancy on the Tracking Kanban

The system SHALL, on every successful tailor bootstrap for a vacancy
(`POST /api/v1/me/cvs/tailor`), ensure that vacancy appears on the caller's
Tracking Kanban board. Concretely it SHALL bookmark the vacancy (`saved_at`
set) and, when the vacancy has no application stage yet, set stage to
`applied`. It MUST NOT set `applied_at`: preparing a CV is not submitting an
application, and silence clocks MUST NOT start from the bootstrap alone.

The placement SHALL run for both a newly created tailored CV and an idempotent
resume of an existing one. An existing non-empty stage MUST be left unchanged.
Clearing board progress later MUST NOT delete or invalidate the tailored CV.

A failure to write tracking MUST NOT fail the bootstrap response once the
tailored CV and its session already exist.

#### Scenario: First tailor puts the vacancy in Applied

- **WHEN** a signed-in user successfully bootstraps tailoring for a vacancy they
  have never tailored
- **THEN** a tailored CV is created AND the vacancy is saved with stage
  `applied` and no `applied_at`

#### Scenario: Resume heals a missing board placement

- **WHEN** a signed-in user bootstraps tailoring for a vacancy they already have
  a tailored CV for, and that vacancy has no stage
- **THEN** the existing CV and conversation are returned AND the vacancy is
  staged as `applied` with `saved_at` set

#### Scenario: An advanced stage is preserved

- **WHEN** the vacancy already has stage `interview` and the user re-bootstraps
  tailoring
- **THEN** the stage remains `interview`

#### Scenario: Tailoring does not claim a submitted application

- **WHEN** a signed-in user successfully bootstraps tailoring for a vacancy
- **THEN** the vacancy's tracking interaction does not gain `applied_at` from
  the bootstrap alone

#### Scenario: A tracking write failure does not undo the tailored CV

- **WHEN** the tailored CV and session have been created and placing the vacancy
  on the board fails
- **THEN** the bootstrap still returns success with the tailored CV and session
  ids

