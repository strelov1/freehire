## ADDED Requirements

### Requirement: Tailoring starts a job-bound copy of the base CV

The system SHALL, on a tailoring bootstrap request for a vacancy, create a new CV row bound to
that vacancy (`cvs.job_id` set) whose document is copied from the user's base CV (`job_id = NULL`),
and SHALL return the tailored CV id, the base CV id, and the cached fit analysis. The base CV MUST
remain unchanged by the bootstrap, and the tailored CV MUST be owner-scoped to the requesting user.

#### Scenario: Bootstrap creates a tailored copy bound to the vacancy

- **WHEN** a signed-in beta user requests tailoring for a vacancy and already has a base CV
- **THEN** a new CV is created with `job_id` set to that vacancy, its document equals the base CV's document, and the response returns both ids plus the cached analysis

#### Scenario: The base CV is untouched by bootstrap

- **WHEN** the tailoring bootstrap creates a tailored copy
- **THEN** the base CV's document and `updated_at` are unchanged

### Requirement: The base CV is seeded from the structured résumé when absent

The system SHALL, when the user has no base CV at tailoring time, seed one from the stored
structured résumé using the existing deterministic seed mapping, persist it as the base CV
(`job_id = NULL`), and then create the tailored copy from it. When no structured résumé is
available, the bootstrap MUST fail with a client error that tells the user to add a résumé first,
and MUST NOT create any CV row.

#### Scenario: A first-time user gets a base CV seeded from their résumé

- **WHEN** a beta user with a stored structured résumé but no base CV requests tailoring
- **THEN** a base CV is seeded from the structured résumé and a tailored copy is created from it

#### Scenario: Tailoring without a résumé is refused

- **WHEN** a beta user with no stored résumé requests tailoring
- **THEN** the request fails with a 409 telling them to add a résumé, and no CV row is created

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

### Requirement: The tailoring agent acts as the user via a scoped, short-lived credential

The system SHALL, at tailoring bootstrap, mint a short-lived scoped API key for the requesting user
and return it so the agent's CLI can authenticate to the CV endpoints as that user. Patches and
reads made with that key MUST be owner-scoped to the same user, so the agent can never read or edit
another user's CV.

#### Scenario: The minted key edits only its owner's CVs

- **WHEN** the agent uses the minted key to patch a CV id that belongs to a different user
- **THEN** the request is rejected as not found / forbidden and no document is mutated

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
