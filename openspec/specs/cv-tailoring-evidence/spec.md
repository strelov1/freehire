# cv-tailoring-evidence Specification

## Purpose
The evidence gate: citing evidence for claims about the candidate, refusing job-shaped project entries, and the editor's path policy. Split out of cv-tailoring.

## Requirements

### Requirement: Tailoring respects hard-constraint guardrails

The tailoring flow SHALL consume the deterministic hard-constraint blockers' anti-hallucination action strings as guardrails, so the tailored output never fabricates a credential, degree, year count, or authorization the caller has not evidenced. When a blocker is unmet, its action string MUST be surfaced to the tailoring step as an explicit "do not claim unless true" instruction.

#### Scenario: Missing certification is not fabricated

- **WHEN** a job requires a certification the caller does not hold and the caller tailors their CV for it
- **THEN** the tailoring step receives the blocker's action string and does not invent the certification in the tailored output

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

### Requirement: A job-shaped project entry is refused for the agent

The system SHALL refuse an agent-authored operation that writes a `projects[i].name` carrying a
`<start year> - <end year or present>` date span, since `cv.Project` has no `company` or date
fields at all and a job filed there loses its tenure outright rather than merely sitting under the
wrong heading. The message MUST name `experience[]` as the place for a role with a start and end
date. This check MUST NOT run for a candidate's own edit — only `ActorAgent` batches are checked,
the same actor scope `requireEvidence` already uses.

The check MUST look only at a project's `name`, never its `bullets`: a bullet legitimately cites a
year as part of an achievement ("cut P95 latency in 2021") without the entry being misfiled.

Where a batch carries several operations, each MUST answer for itself, and one operation tripping
this refusal MUST reject the whole batch — the same all-or-nothing contract the evidence gate and
the bullet-cap guard already keep.

#### Scenario: A job-dated project name is refused

- **WHEN** the agent commits an operation setting `projects[0].name` to "Senior Engineer, Acme Corp (2020 - 2023)"
- **THEN** the commit is refused and the message names `experience[]` as the place for a role with a start and end date

#### Scenario: A project name with no date span is allowed

- **WHEN** the agent commits an operation setting a project's `name` to "Freelance Consulting, Acme Corp"
- **THEN** the commit proceeds — there is no date-range signal to refuse on

#### Scenario: A date mentioned in a bullet is allowed

- **WHEN** the agent commits an operation adding a bullet that reads "Cut P95 latency in half in 2021" to a project
- **THEN** the commit proceeds — only a project's `name` is checked, never its bullets

#### Scenario: The candidate's own edit is never refused by this check

- **WHEN** the candidate (not the agent) sets a project's `name` to "Senior Engineer, Acme Corp (2020 - 2023)" through their own CLI edit
- **THEN** the commit proceeds — this check only applies to `ActorAgent` batches

#### Scenario: One job-shaped project rejects the whole batch

- **WHEN** a batch writes two project names and one of them carries a job's date span
- **THEN** neither is applied

#### Scenario: The refusal holds even when a later operation shifts the entry's index

- **WHEN** a batch sets a job-dated name on `projects[0]` and then inserts a new entry ahead of it, shifting the rewritten entry to `projects[1]` in the final document
- **THEN** the commit is still refused — the check reads what each operation writes, not where it ends up after the rest of the batch runs

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

### Requirement: A cited claim can be checked for fidelity to its own evidence

The tailoring agent SHALL have access to a tool that, given the `evidence_id` an edit cited,
returns that atom's own `claim`, `context`, and `metrics` exactly as banked. The tool MUST call no
model and MUST NOT alter the CV, the CV's report, or the experience bank — it is a read, not a
gate. `tailorPrompt` SHALL instruct the agent to call this tool after a batch of edits that cited
evidence, compare the wording it just wrote against what the tool returns, and revise via
`cv_edit` when the written claim states more scope, seniority, or a bigger metric than the atom
supports.

This is independent of the citation requirement: an edit citing a real, publishable atom is
already accepted by the evidence gate regardless of whether this check ever runs. The check is
advisory and agent-directed — bounded the same way the existing `job_match` self-check is bounded
(a soft cap of 2-3 rounds, the agent's own judgment to stop), not a second server-side refusal.
Fidelity is a semantic judgment a deterministic check cannot safely arbitrate; a hard block here
would risk refusing an honest, well-worded bullet.

#### Scenario: The tool returns the cited atom's own text

- **WHEN** the agent calls the fidelity-check tool with an `evidence_id` that names a banked atom
- **THEN** the result carries that atom's `claim`, `context`, and `metrics`, with no model call
  made and no CV, report, or bank state changed

#### Scenario: An overstated bullet is revised after the check

- **WHEN** the agent has written a bullet that claims more scope or seniority than the atom it
  cited supports, and then calls the fidelity-check tool for that citation
- **THEN** the agent's next `cv_edit` call may revise that bullet to stay inside what the atom
  says, and the revision is applied under the same evidence-citation rule as any other edit

#### Scenario: A faithful bullet needs no revision

- **WHEN** the agent calls the fidelity-check tool for a bullet whose wording already stays
  inside what the cited atom says
- **THEN** the agent may proceed without any further `cv_edit` call for that citation, and nothing
  about the CV is changed by the check itself

#### Scenario: The check never blocks a citation the evidence gate already accepted

- **WHEN** an edit cites a real, publishable atom and the fidelity-check tool is never called for it
- **THEN** the edit is still applied exactly as the evidence-citation requirement already governs,
  unaffected by this requirement

#### Scenario: Both tailoring rhythms carry the same rule

- **WHEN** a claim-shaped edit is made during an ordinary conversational tailoring turn or during
  an autopilot run
- **THEN** the same fidelity-check tool and `tailorPrompt` instruction apply in both, since both
  run the same tailoring prompt

