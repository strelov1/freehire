## ADDED Requirements

### Requirement: The tailoring agent consults the bank before asking the candidate

The tailoring agent SHALL, for each requirement it intends to address, search the experience bank before putting a question to the candidate. When the search returns publishable evidence, the agent SHALL reframe that evidence into a bullet in the vacancy's language and MUST stay inside what the atom states. Only when the bank holds no evidence for that requirement SHALL the agent ask. This ordering SHALL apply to `missing-have` and `missing-gap` requirements alike, because the split reflects what the CV surfaces, not what the candidate has.

#### Scenario: A banked requirement is answered without a question

- **WHEN** the agent addresses a requirement for which the bank holds a matching atom
- **THEN** it surfaces that evidence as a bullet and does not ask the candidate about it

#### Scenario: An unbanked requirement is asked about

- **WHEN** the agent addresses a requirement for which the bank holds no evidence
- **THEN** it asks the candidate before writing anything

#### Scenario: A second tailoring session reuses the first one's answers

- **WHEN** a candidate tailors for a second vacancy sharing a requirement they answered during the first
- **THEN** the requirement is answered from the bank and the same question is not asked again

### Requirement: What the candidate confirms while tailoring is persisted before it is written

The system SHALL, when the candidate confirms real experience in response to a tailoring question, persist it to the experience bank as an atom with provenance `stated_in_chat` before that experience is written into the tailored CV. A tailored CV is a derived, disposable artifact; the knowledge produced while making it MUST outlive it.

#### Scenario: A confirmed answer outlives the tailored CV

- **WHEN** a candidate confirms experience during tailoring and the tailored CV is later deleted
- **THEN** the atom recorded from that answer remains in the bank

#### Scenario: A declined question writes nothing

- **WHEN** a candidate answers that they do not have the experience asked about
- **THEN** no atom is recorded and nothing is written into the CV

## MODIFIED Requirements

### Requirement: CV edits are applied as sanitized field-level patches

The system SHALL expose an operation that applies a single field-level patch to a CV document —
addressing the summary, a specific experience entry's bullets (add, replace, remove, reorder), a
skill group, or a header field — without re-emitting the rest of the document. Every patch MUST be
applied through a pure document transform and then passed through the document sanitizer (length
and cardinality bounds, prompt-injection guard) before persistence. A patch that addresses a field
or index that does not exist MUST be rejected as a client error and MUST NOT mutate the document.
A patch whose content is backed only by a non-publishable (`agent_inferred`) experience atom MUST
be rejected with a reason the model can act on, and MUST NOT mutate the document.

#### Scenario: A bullet is added to one experience entry, leaving others intact

- **WHEN** a patch adds a bullet to experience entry 0
- **THEN** entry 0 gains the bullet, every other section of the document is byte-for-byte unchanged, and the result is sanitized before saving

#### Scenario: Out-of-range addressing is rejected

- **WHEN** a patch targets an experience index that does not exist
- **THEN** the operation fails with a 422 and the stored document is unchanged

#### Scenario: Bullets can be reordered by relevance

- **WHEN** a patch reorders the bullets of an experience entry to a given permutation
- **THEN** that entry's bullets appear in the requested order and no bullet is added or dropped

#### Scenario: Content backed only by an agent's inference is refused

- **WHEN** a patch writes a bullet whose only backing evidence is an `agent_inferred` atom
- **THEN** the operation is rejected with a message naming the unconfirmed evidence, and the stored document is unchanged

### Requirement: The base CV is seeded from the structured résumé when absent

The system SHALL, when the user has no base CV at tailoring time, seed one from the user's
**experience bank** — using the banked employments and their publishable atoms for the work
history, and the stored structured résumé for the sections the bank does not own (contacts,
summary, education, languages) — persist it as the base CV (`job_id = NULL`), and then create the
tailored copy from it. When the bank is empty and no structured résumé is available, the bootstrap
MUST fail with a client error that tells the user to add a résumé first, and MUST NOT create any
CV row.

#### Scenario: A first-time user gets a base CV seeded from their bank

- **WHEN** a beta user with a populated experience bank but no base CV requests tailoring
- **THEN** a base CV is seeded from the bank and a tailored copy is created from it

#### Scenario: Seeding carries experience the uploaded CV never contained

- **WHEN** a beta user whose bank holds atoms confirmed in chat requests a base CV
- **THEN** those atoms appear in the seeded CV alongside the ones imported from their uploaded résumé

#### Scenario: Tailoring without any experience is refused

- **WHEN** a beta user with an empty bank and no stored résumé requests tailoring
- **THEN** the request fails with a 409 telling them to add a résumé, and no CV row is created
