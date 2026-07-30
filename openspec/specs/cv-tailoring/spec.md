# cv-tailoring Specification

## Purpose
TBD - created by archiving change add-cv-tailoring. Update Purpose after archive.
## Requirements
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

### Requirement: Tailoring requires an existing fit analysis

The system SHALL require a cached fit analysis for the (user, vacancy) pair before tailoring, and
MUST NOT recompute it. When no cached analysis exists, the bootstrap MUST fail with a 409 telling
the user to run the fit analysis first.

#### Scenario: Tailoring is refused when no analysis is cached

- **WHEN** a beta user requests tailoring for a vacancy they have never analyzed
- **THEN** the request fails with a 409 telling them to run the fit analysis first

#### Scenario: Bootstrap returns the cached analysis without recomputing

- **WHEN** a beta user with a cached analysis requests tailoring
- **THEN** the response carries that cached analysis and no LLM call is made

### Requirement: CV edits are applied as sanitized field-level patches

The system SHALL expose an operation that applies a single field-level patch to a CV document —
addressing the summary, a specific experience entry's bullets (add, replace, remove, reorder), a
skill group, or a header field — without re-emitting the rest of the document. Every patch MUST be
applied through a pure document transform and then passed through the document sanitizer (length
and cardinality bounds, prompt-injection guard) before persistence. A patch that addresses a field
or index that does not exist MUST be rejected as a client error and MUST NOT mutate the document.

#### Scenario: A bullet is added to one experience entry, leaving others intact

- **WHEN** a patch adds a bullet to experience entry 0
- **THEN** entry 0 gains the bullet, every other section of the document is byte-for-byte unchanged, and the result is sanitized before saving

#### Scenario: Out-of-range addressing is rejected

- **WHEN** a patch targets an experience index that does not exist
- **THEN** the operation fails with a 422 and the stored document is unchanged

#### Scenario: Bullets can be reordered by relevance

- **WHEN** a patch reorders the bullets of an experience entry to a given permutation
- **THEN** that entry's bullets appear in the requested order and no bullet is added or dropped

### Requirement: Tailoring context is served from the cached analysis

The system SHALL expose a read that returns the tailoring context for a tailored CV — the verdict,
the recommendation, the per-dimension comments, and the requirement coverage split into
`missing-have` (evidence exists but the CV omits it) and `missing-gap` (a genuine gap) — sourced
from the cached fit analysis with no LLM recompute. The read MUST be owner-scoped and MUST require
an authenticated caller (session cookie or API key).

#### Scenario: The consumer can distinguish reframe-able requirements from genuine gaps

- **WHEN** an authenticated owner reads the tailoring context for their tailored CV
- **THEN** the response lists the verdict, recommendation, dimension comments, and requirements labelled `missing-have` versus `missing-gap`

### Requirement: Tailoring is beta-gated and surfaced only after analysis

The system SHALL gate every tailoring endpoint behind beta access (the union of the CV builder gate
and the agent's beta-tester flag), and the fit page SHALL surface the "tailor my CV" entry point
only when a cached, non-stale analysis exists for that user and vacancy.

#### Scenario: A non-beta user cannot reach tailoring

- **WHEN** a signed-in non-beta user calls the tailoring bootstrap
- **THEN** the request is refused by the beta gate

#### Scenario: The CTA is hidden without an analysis

- **WHEN** a beta user opens a fit page for a vacancy they have not analyzed
- **THEN** the "tailor my CV" entry point is not shown

### Requirement: Tailoring respects hard-constraint guardrails

The tailoring flow SHALL consume the deterministic hard-constraint blockers' anti-hallucination action strings as guardrails, so the tailored output never fabricates a credential, degree, year count, or authorization the caller has not evidenced. When a blocker is unmet, its action string MUST be surfaced to the tailoring step as an explicit "do not claim unless true" instruction.

#### Scenario: Missing certification is not fabricated

- **WHEN** a job requires a certification the caller does not hold and the caller tailors their CV for it
- **THEN** the tailoring step receives the blocker's action string and does not invent the certification in the tailored output

### Requirement: The tailoring agent never receives the CV contact block

The CV contact block MUST NOT reach any reader that puts the document in front of a model. That
covers both agent-facing read paths — the HTTP read authenticated with the short-lived tailoring
key, and the in-process assistant's `cv_get` tool — each of which SHALL omit the `Header` contact
fields (`full_name`, `email`, `phone`, and the personal links list) from the document it returns,
and SHALL reject any patch that targets those fields.

The omission SHALL be performed by one shared redaction in the service path, applied by every
agent-facing reader, rather than by a per-transport check in a handler. A guard keyed on how the
request arrived (holding an API key, say) does not survive a new agent surface that arrives over a
different transport or none at all, and the requirement is about who is reading, not how they got
there. A new surface must not be able to inherit the CV-reading capability without inheriting the
redaction.

The stored contact values are unchanged and appear in the rendered output (served on the owner's
own cookie-authenticated read and the PDF), so the finished CV is complete while the agent's model
never sees the identifiers. The candidate's own cookie-authenticated reads are unaffected.

#### Scenario: Agent read omits the contact block

- **WHEN** the tailoring key is used to read the CV document
- **THEN** the response document carries the body (experience, summary, skills, …) but no `full_name`, `email`, or `phone`

#### Scenario: The in-process tool read omits the contact block

- **WHEN** the assistant's `cv_get` tool reads the CV document during a tailoring session, holding no API key
- **THEN** the tool result carries the body but no `full_name`, `email`, `phone`, or personal links

#### Scenario: Agent cannot patch a contact field

- **WHEN** the tailoring key is used to patch `full_name`, `email`, or `phone`
- **THEN** the patch is rejected and the stored contact value is unchanged

#### Scenario: The owner still sees and renders full contacts

- **WHEN** the owner reads the CV over their cookie session, or the CV is rendered to PDF
- **THEN** the real contact block is present

#### Scenario: The tailored body carries no contact identifier back

- **WHEN** the agent patches the CV body during tailoring
- **THEN** no contact identifier is introduced into a body field (the agent never held one)

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
