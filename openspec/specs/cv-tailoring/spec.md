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

### Requirement: What an actor may edit is an explicit path policy

The system SHALL decide what an actor may change from an explicit policy over paths, evaluated on
every commit. The agent MUST be denied the candidate's identifying header fields, the CV's title
and its template; the candidate MUST be denied nothing. A denial MUST name what the actor may
change instead, because for a model the error message is its only route to correcting itself
inside the turn.

This replaces access control by omission. Previously the agent could not write a contact field
because the edit vocabulary named no operation for it — a real defence, but an accidental one
that widens silently whenever the vocabulary grows.

#### Scenario: The agent is refused a contact field

- **WHEN** the tailoring agent commits an operation addressing the candidate's email
- **THEN** the commit is refused, the stored value is unchanged, and the message names what the agent may edit

#### Scenario: The candidate edits the same field freely

- **WHEN** the candidate changes their email in the editor
- **THEN** the change is committed as a revision

#### Scenario: A newly addressable field is closed to the agent by policy, not by omission

- **WHEN** the document gains a field and the agent must not write it
- **THEN** it is denied by naming it in the policy, and the denial is covered by a test

### Requirement: Any operation that asserts something about the candidate must cite evidence

The system SHALL require a citation of banked evidence for every operation that writes a claim
about the candidate, identified by the path it writes rather than by the name of the operation.
The gated paths MUST include the summary, an experience entry's summary, its bullets, a project's
bullets, an experience entry's technology line, and a skill group's items. The cited evidence MUST
carry publishable provenance — something the candidate asserted, never something the model
inferred.

Operations that remove or move MUST require no citation: they rearrange or delete what was
already said, and assert nothing new.

Where a batch carries several writing operations, each MUST answer for itself, and one uncited
operation MUST reject the whole batch. Otherwise an unevidenced claim could ride in among valid
ones.

The technology line and skill groups are newly gated. Under the earlier vocabulary only two
operations required evidence, so an agent could put a technology on the CV as a stack entry or a
skill unevidenced while the same claim written as a bullet was refused — the same assertion in
different syntax.

#### Scenario: An unevidenced bullet is refused

- **WHEN** the agent commits an operation writing a bullet without citing evidence
- **THEN** the commit is refused and the message says how to obtain a citation

#### Scenario: An unevidenced technology is refused

- **WHEN** the agent commits an operation adding a technology to an experience entry's stack without citing evidence
- **THEN** the commit is refused, exactly as it would be for a bullet making the same claim

#### Scenario: One uncited operation rejects a batch

- **WHEN** a batch writes three bullets and cites evidence for two of them
- **THEN** none of the three is applied

#### Scenario: Removing a bullet needs no citation

- **WHEN** the agent removes a bullet
- **THEN** the commit proceeds without a citation

#### Scenario: Model-inferred evidence cannot be cited

- **WHEN** an operation cites evidence recorded as the model's own inference
- **THEN** the commit is refused and the message says to have the candidate confirm it first

### Requirement: The agent's tailoring context carries the bank's answer per requirement

The tailoring agent's context tool SHALL attach, to every requirement it reports, the evidence the
candidate's experience bank already holds for it — each piece named by the id a CV edit must cite,
so the agent knows before it acts which requirements it can evidence and which it must ask about.
Retrieval MUST be the same scoring the search tool uses, and MUST NOT call a model: it is a scan
over the caller's own atoms, so attaching it costs a round nobody has to spend.

A requirement the bank has nothing for MUST say so explicitly rather than omit the field, because
"no evidence" is the answer that decides whether to ask the candidate, and an absent field reads
as "not looked at".

#### Scenario: A requirement the bank can evidence arrives with its evidence

- **WHEN** the agent reads the tailoring context for a vacancy whose requirement the bank holds evidence for
- **THEN** that requirement carries the evidence's id and claim, ready to be cited in an edit

#### Scenario: A requirement the bank cannot evidence says so

- **WHEN** a reported requirement has nothing scoring against it in the bank
- **THEN** it is reported with an empty evidence list rather than with the field left out

#### Scenario: Reading the context calls no model

- **WHEN** the agent reads the tailoring context
- **THEN** the evidence attached to each requirement comes from local scoring, with no LLM call

### Requirement: The agent's context carries what it can act on, not the narrative

The tailoring context served to the AGENT SHALL carry the vacancy, the verdict and score, and the
requirements with their evidence — and SHALL NOT carry the per-dimension comments, strengths, gaps
or recommendation the endpoint serves the page. None of them is something a CV edit can be made
from, all of them are on the candidate's screen already, and on a measured session they were 3 KB
of an 11.4 KB result the agent had to carry for the rest of the turn.

#### Scenario: The narrative sections stay with the page

- **WHEN** the agent reads the tailoring context
- **THEN** the result carries the vacancy, verdict, score and the evidenced requirements, and none of the dimension comments, strengths, gaps or recommendation

#### Scenario: The endpoint is unchanged

- **WHEN** a client reads the tailoring context over HTTP
- **THEN** it receives the full projection it received before, narrative sections included

### Requirement: The agent reads the posting as text, not as markup

The tailoring context served to the agent SHALL render the vacancy description from stored HTML to
plain text, and SHALL bound its length, exactly as the vacancy-reading tool already does. The
posting is the least trusted text in the conversation and the largest; sending it as markup spends
the turn's context on tags and widens what the model is asked to interpret.

#### Scenario: The description reaches the model without markup

- **WHEN** the agent reads the tailoring context for a vacancy whose stored description is HTML
- **THEN** the description it receives carries the posting's words and none of its tags

#### Scenario: A very long posting is bounded

- **WHEN** the stored description is longer than the context allows
- **THEN** it is truncated to the bound rather than sent whole

### Requirement: The evidence gate is a construction-time dependency of the editor

The component that answers "may this claim be published?" SHALL be supplied when the CV
editor is created, and SHALL NOT be attachable afterwards. No assembly order, and no
combination of features built or not built, may produce an editor that admits an agent's
uncited claim.

The system SHALL NOT carry more than one place that answers the evidence question by
returning "permitted" for a missing dependency. Where the gate's own collaborator is
absent, that is a construction error, not a silent permission.

An editor built with no gate remains legitimate for candidate-authored editing only —
the candidate writing about their own career is the source the bank exists to record, and
the rule already exempts them — but it SHALL NOT be reachable from any agent write path.

#### Scenario: The agent write path cannot be built without a gate

- **WHEN** the HTTP surface is assembled
- **THEN** the editor serving `PATCH /me/cvs/:id` carries the evidence gate as a value it
  was constructed with, so an API-key caller editing as the agent is checked regardless of
  which other features the assembly built

#### Scenario: The gate does not depend on another feature's constructor

- **WHEN** the CV handlers are built in an assembly that does not build the assistant
- **THEN** the editor still refuses an agent's uncited claim, because the gate was never
  the assistant's to attach

#### Scenario: A missing bank is not a permission

- **WHEN** the gate is asked whether a claim is publishable and its backing bank is absent
- **THEN** the answer is not "permitted" — the absence is a wiring error the assembly
  cannot express, rather than a silent pass

