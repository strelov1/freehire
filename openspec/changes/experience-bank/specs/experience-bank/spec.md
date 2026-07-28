## ADDED Requirements

### Requirement: The experience bank is a durable, owner-scoped store of employments and evidence atoms

The system SHALL persist, per user, an **experience bank** of two entities: an **employment** (a job or a project — company, role, location, free-form start/end dates, a current flag, and a one-line context summary) and an **atom** (one piece of evidence — a claim, a context paragraph, metrics, canonical skill slugs, a provenance, and a source reference), where an atom MAY be attached to an employment or stand alone. Every read and write SHALL be scoped by `user_id`; a bank entry the caller does not own MUST be reported as missing, never as forbidden. The bank SHALL survive résumé re-upload, CV deletion, and session expiry — only an explicit user deletion removes an entry.

#### Scenario: The bank outlives the artifact it was seeded from

- **WHEN** a user whose bank was seeded from a CV deletes that CV
- **THEN** the bank retains its employments and atoms, and remains readable

#### Scenario: A bank entry is owner-scoped

- **WHEN** a signed-in user requests an atom belonging to another user
- **THEN** the response reports the atom as not found

### Requirement: Atom skills are canonicalized through the vacancy skill dictionary

The system SHALL canonicalize every skill recorded on an atom through the same `internal/skilltag` alias→canonical dictionary that produces the `jobs.skills` facet, and SHALL persist only canonical slugs. A skill token that the dictionary does not recognize SHALL be dropped rather than stored raw, matching the dict-only rule applied to every other production facet.

#### Scenario: An alias is stored in canonical form

- **WHEN** an atom is recorded with the skill "k8s"
- **THEN** the persisted atom carries the canonical slug `kubernetes`

#### Scenario: A requirement naming a skill matches evidence recorded under an alias

- **WHEN** the bank is searched for a vacancy requirement naming "Kubernetes" and the only matching atom was recorded as "k8s"
- **THEN** that atom is returned as evidence

#### Scenario: An unrecognized skill token is not persisted

- **WHEN** an atom is recorded with a skill token absent from the dictionary
- **THEN** that token is dropped and the atom is persisted with its remaining canonical skills

### Requirement: Every atom records where it came from, and provenance gates publication

The system SHALL stamp every atom with one provenance value: `cv_import` (parsed from the user's uploaded CV), `stated_in_chat` (the user asserted it in an assistant session), `manual` (the user entered it directly), or `agent_inferred` (the model originated it without user assertion). An atom whose provenance is `cv_import`, `stated_in_chat` or `manual` SHALL be publishable — eligible to be rendered into a CV bullet. An atom whose provenance is `agent_inferred` MUST NOT be publishable until the user confirms it, at which point it is re-stamped. This gate SHALL be enforced in the service layer, not by system-prompt instruction.

#### Scenario: An agent's own inference cannot reach the CV

- **WHEN** a CV edit is applied whose only backing evidence is an `agent_inferred` atom
- **THEN** the edit is rejected with a reason the model can act on, and the CV document is unchanged

#### Scenario: Confirming an inference makes it publishable

- **WHEN** the user confirms an `agent_inferred` atom
- **THEN** the atom is re-stamped as user-asserted and becomes eligible for CV content

#### Scenario: Provenance is derived, not chosen by the model

- **WHEN** the model records an atom it originated rather than one the user stated
- **THEN** the persisted provenance is `agent_inferred` regardless of the value the model supplied

### Requirement: Bank content is sanitized on persist

The system SHALL bound every atom and employment string, cap every array's cardinality, and drop entries carrying no content, before persistence and before serving. Bank content is user- and model-authored and is fed back into LLM prompts, so the sanitizer SHALL be both the persistence guard and the prompt-injection guard, matching the invariant already applied in `internal/cv` and `internal/resumeextract`.

#### Scenario: Over-long model output is coerced

- **WHEN** an atom is recorded with an over-long claim, an oversized metrics list, or an oversized skill list
- **THEN** the persisted atom has a bounded claim and capped arrays, and only the sanitized value is served

#### Scenario: An empty atom is not persisted

- **WHEN** an atom is recorded with no claim and no context
- **THEN** nothing is persisted and the caller is told what was missing

### Requirement: Importing a CV into the bank is additive and never destructive

The system SHALL, on every résumé upload, reconcile the extracted structure into the bank rather than replacing it: an employment SHALL be matched case-insensitively on (company, role) — filling only fields that are empty in the bank and never overwriting a value the user has edited — and created when no match exists; an achievement bullet SHALL be matched against existing atoms on a normalized claim and created with provenance `cv_import` when no match exists. Import MUST NOT delete an employment or an atom under any circumstances. The upload time SHALL be recorded as the atom's source reference.

#### Scenario: Uploading a shorter CV does not shrink the bank

- **WHEN** a user whose bank holds twelve atoms uploads a trimmed one-page CV listing four
- **THEN** the bank still holds at least twelve atoms and no entry is removed

#### Scenario: Re-uploading the same CV does not duplicate atoms

- **WHEN** a user uploads a CV whose bullets are already present in the bank
- **THEN** no duplicate atoms are created

#### Scenario: A user's edit survives re-import

- **WHEN** a user has corrected an employment's role and then uploads a CV naming the old role
- **THEN** the corrected role is preserved and is not overwritten by the import

#### Scenario: A late extraction still lands

- **WHEN** an extraction completes after the user has already uploaded a newer CV
- **THEN** its atoms are still imported, because additive import cannot produce a lost update

### Requirement: Evidence is retrieved by deterministic relevance scoring

The system SHALL rank a user's atoms against a query — a free-text requirement and/or a set of canonical skills — without an LLM call, scoring on canonical skill-set intersection as the dominant term, token overlap on the claim and context, the recency of the atom's employment, and a penalty for `agent_inferred`. The search SHALL return a bounded top-N so a large bank cannot flood a tool result.

#### Scenario: Skill-matching evidence outranks incidental text overlap

- **WHEN** the bank is searched for a requirement naming a skill, and one atom carries that canonical skill while another merely repeats words from the requirement text
- **THEN** the skill-carrying atom is ranked first

#### Scenario: A large bank returns a bounded result

- **WHEN** a user with several hundred atoms runs a search that matches most of them
- **THEN** the result is capped at the top-N most relevant atoms

#### Scenario: Retrieval performs no model call

- **WHEN** a search runs against the bank
- **THEN** no LLM request is made

### Requirement: The assistant can read and extend the bank from any preset

The system SHALL register the experience tools — search the bank, add an atom, update an atom, and list employments — for every assistant preset, because the moment a candidate articulates their experience is not confined to one surface. Each tool SHALL act as the session's owner and SHALL return structured data the model can act on. A malformed call or a rejected write SHALL be reported back to the model as a tool result naming what was invalid, never as a turn failure.

#### Scenario: Evidence stated in a general chat is captured

- **WHEN** a user mentions a concrete achievement during a general job-search chat
- **THEN** the agent can persist it as an atom with provenance `stated_in_chat`, and it is available to a later tailoring session

#### Scenario: An atom is attached to an existing employment rather than duplicating it

- **WHEN** the agent records an atom for a company already present in the bank
- **THEN** it attaches to the existing employment instead of creating a second one for the same company and role

#### Scenario: A rejected write is reported to the model

- **WHEN** the agent submits an atom with no claim
- **THEN** the tool returns an error message naming the missing field and the turn continues

### Requirement: The profile tool reports the bank's shape, not its contents

The system SHALL have `get_profile` report the bank as a summary — the employments, the atom count per employment, and which of the user's profile skills have supporting evidence — and SHALL NOT return the atoms themselves. Atom content SHALL be reachable only through a search, because a tool result is persisted in the transcript and replayed into the model's context on every subsequent turn.

#### Scenario: A large bank does not flood the transcript

- **WHEN** a user with several hundred atoms starts a session and the agent reads their profile
- **THEN** the result carries employments and counts, and no atom bodies

#### Scenario: Uncovered profile skills are visible to the agent

- **WHEN** the user's saved profile lists a skill for which the bank holds no atom
- **THEN** the profile summary marks that skill as lacking evidence

### Requirement: A dedicated preset interviews the user to fill the bank's gaps

The system SHALL offer a `profile` assistant preset whose system prompt directs the agent to find thin spots in the bank — an employment with no atoms, a saved profile skill with no supporting evidence, an achievement with no metric — and to fill them by asking the candidate, recording each confirmed answer as an atom. The preset SHALL differ from the others only in its prompt and the intent it carries; the experience tools are available in every preset regardless.

#### Scenario: The interviewer opens on a real gap

- **WHEN** a user starts a `profile` session and their bank holds an employment with no atoms
- **THEN** the agent asks about that employment rather than opening with a generic questionnaire

#### Scenario: A confirmed answer becomes evidence

- **WHEN** the user answers the interviewer with a concrete achievement
- **THEN** an atom is persisted with provenance `stated_in_chat`, attached to the employment under discussion

### Requirement: The bank is reviewable and deletable by its owner

The system SHALL expose the bank on the profile page: the employments and their atoms, with the provenance of each atom visible, and SHALL let the owner edit or delete any entry. Deletion SHALL be the only path that removes bank content.

#### Scenario: The user sees what the agent recorded about them

- **WHEN** a signed-in user opens the experience section of their profile
- **THEN** every atom is listed under its employment with its provenance shown

#### Scenario: The user removes an entry

- **WHEN** the owner deletes an atom
- **THEN** it is removed from the bank and no longer appears in searches or in CV seeding

### Requirement: Existing users' banks are seeded by a one-off backfill

The system SHALL provide a run-once worker that seeds the bank for users who already have a stored CV, reusing an existing structured extraction where one is present and invoking the extractor only where none is. The worker SHALL be idempotent — a repeat run MUST NOT duplicate employments or atoms — and SHALL exit non-zero on failure, matching the other `cmd/` workers.

#### Scenario: An already-extracted user costs no model call

- **WHEN** the backfill processes a user with a stored structured résumé
- **THEN** the bank is seeded from that structure and no LLM request is made for them

#### Scenario: Re-running the backfill is safe

- **WHEN** the backfill is run twice over the same users
- **THEN** the second run creates no duplicate employments or atoms
