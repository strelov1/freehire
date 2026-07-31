## MODIFIED Requirements

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

## ADDED Requirements

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
