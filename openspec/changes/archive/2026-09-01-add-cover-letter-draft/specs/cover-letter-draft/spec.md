## ADDED Requirements

### Requirement: A cover letter is drafted by a fixed three-stage chain

The system SHALL draft a cover letter for a (candidate, vacancy) pair through a fixed, typed
prompt-chain of three stages, and SHALL NOT draft one through an autonomous tool-calling agent.
The stages SHALL be:

1. **Select** — choose the achievement atoms that answer this vacancy's requirements;
2. **Draft** — write the letter from the selected atoms and the vacancy;
3. **Audit** — a skeptic pass over the draft.

Stage 3 SHALL merge onto the Stage 2 draft. When Stage 3's output cannot be parsed, the system
SHALL serve the un-audited Stage 2 draft rather than failing the request, the same degradation
`ai-fit-analysis` applies to its own audit stage.

The server SHALL own every bound on the result — the length ceiling, the count of cited atoms, and
the vocabulary of any enumerated field. The model SHALL NOT be trusted to observe them.

#### Scenario: The three stages produce a letter

- **WHEN** a candidate with banked experience requests a draft for a vacancy that has a cached fit analysis
- **THEN** the three stages run in order and the response carries the audited letter body and the identifiers of the atoms it cites

#### Scenario: The audit stage fails to parse

- **WHEN** Stage 3 returns output that cannot be decoded
- **THEN** the Stage 2 draft is sanitized and served, and the request does not fail

#### Scenario: A model over-long body is bounded by the server

- **WHEN** the chain returns a body longer than the configured ceiling
- **THEN** the stored and served body is clipped to the ceiling

### Requirement: Only evidence the candidate asserted may be cited

Stage 1 SHALL select only atoms whose provenance is `manual`, `cv_import`, or `stated_in_chat`.
An atom whose provenance is `agent_inferred`, or whose provenance is absent or unrecognised, SHALL
NOT be offered to the chain and SHALL NOT appear in a letter's cited evidence.

The filter SHALL be applied in the service, before any model call, and SHALL NOT be expressed only
as an instruction inside a prompt.

#### Scenario: An inferred atom is withheld

- **WHEN** a candidate's bank holds an `agent_inferred` atom that scores highly against the vacancy
- **THEN** the atom is not sent to the model and does not appear among the letter's cited atoms

#### Scenario: An unrecognised provenance is withheld

- **WHEN** an atom carries a provenance value outside the known set
- **THEN** it is treated as not publishable and withheld

#### Scenario: A candidate with no publishable evidence gets no letter

- **WHEN** every atom in the candidate's bank is `agent_inferred`
- **THEN** no chain runs, no model is called, and the response reports that there is no publishable evidence to draft from

### Requirement: The letter reframes what the CV leaves implicit

Stage 1 SHALL receive the fit analysis's requirement split and SHALL prioritise atoms answering
requirements classified `missing-have` — requirements the candidate meets but whose evidence the CV
does not surface. Requirements classified `missing-gap` SHALL NOT be answered with invented
evidence; the letter SHALL either omit them or address them with adjacent publishable evidence,
named as adjacent.

#### Scenario: A missing-have requirement is answered

- **WHEN** the fit analysis marks a requirement `missing-have` and the bank holds a publishable atom matching it
- **THEN** the letter cites that atom against that requirement

#### Scenario: A genuine gap is not invented over

- **WHEN** the fit analysis marks a requirement `missing-gap` and no publishable atom answers it
- **THEN** the letter makes no claim of experience with it

### Requirement: The skeptic stage enforces support and length

Stage 3 SHALL remove from the draft any sentence asserting the candidate's experience that is not
supported by one of the atoms Stage 1 selected, and SHALL reduce the draft to the requested length
band. Brevity SHALL be produced by this stage, not solely by an instruction in the drafting prompt.

Sentences that carry no claim about the candidate's experience — the address, the statement of
interest in the role or the employer, the closing — are not subject to the support rule.

#### Scenario: An unsupported claim is cut

- **WHEN** the Stage 2 draft asserts an achievement that matches no selected atom
- **THEN** that assertion is absent from the audited letter

#### Scenario: Motivation survives the audit

- **WHEN** the Stage 2 draft states why the candidate is interested in the employer
- **THEN** that statement is retained, because it asserts no experience

#### Scenario: An over-long draft is shortened

- **WHEN** the Stage 2 draft exceeds the requested length band
- **THEN** the audited letter falls within the band

### Requirement: The letter is written in the language of the vacancy

The letter SHALL be written in the language of the vacancy, determined from the posting, and SHALL
NOT be written in the candidate's profile language when the two differ. A stored draft SHALL record
the language it was written in.

#### Scenario: Profile language and vacancy language differ

- **WHEN** a candidate whose profile language is Russian drafts a letter for a vacancy written in German
- **THEN** the letter is written in German

#### Scenario: The language is recorded

- **WHEN** a draft is stored
- **THEN** the row records the language the letter was written in

### Requirement: Raw CV text never reaches the model

The candidate context sent to the chain SHALL be the de-identified structured projection of the
stored CV and nothing else. The raw CV text SHALL NOT be sent. A field absent from that projection
SHALL NOT reach the model by any other route.

#### Scenario: Only the projection is sent

- **WHEN** a draft is requested for a candidate whose stored CV carries a phone number and a home address
- **THEN** neither reaches the model, because the projection does not carry them

### Requirement: One current draft is stored per candidate and vacancy

The system SHALL store at most one draft per (candidate, vacancy). A new draft for the same pair
SHALL replace the stored one. The system SHALL NOT keep a revision history and SHALL NOT offer undo.

A stored draft SHALL be stamped with the model that produced it and the language it was written in,
and SHALL be reported stale when either differs from the live value.

#### Scenario: A second draft replaces the first

- **WHEN** a candidate drafts a letter twice for the same vacancy
- **THEN** the second body is stored and the first is not retained

#### Scenario: A model upgrade marks a draft stale

- **WHEN** the configured model changes after a draft was stored
- **THEN** a read of that draft reports it stale

### Requirement: Reading a draft never calls a model

A read of a stored draft SHALL serve what is stored, or report that none exists, and SHALL NOT call
a language model. Only an explicit request to draft SHALL run the chain.

#### Scenario: A read with no stored draft

- **WHEN** a candidate reads a draft for a vacancy they have never drafted for
- **THEN** the response reports no draft and no model is called

#### Scenario: A read with a stale stored draft

- **WHEN** a candidate reads a draft whose model stamp is stale
- **THEN** the stored body is served with its staleness reported, and no model is called

### Requirement: The assistant tool and the endpoint share one chain

The assistant SHALL offer a tool that drafts a cover letter, and that tool SHALL execute the same
chain, the same provenance gate, and the same bounds as the endpoint. The tool SHALL act as the
authenticated owner of the session.

#### Scenario: The chat path and the button path agree

- **WHEN** a candidate asks the assistant to draft a letter for a vacancy
- **THEN** the letter is produced by the same chain the endpoint runs, and is stored as that pair's current draft

#### Scenario: The tool observes the provenance gate

- **WHEN** the assistant drafts a letter for a candidate whose bank holds `agent_inferred` atoms
- **THEN** those atoms are withheld from the chain exactly as they are on the endpoint path

### Requirement: A drafting failure degrades and does not corrupt

An unconfigured or failing language model SHALL leave any previously stored draft untouched and
SHALL return no letter, rather than storing a partial or empty body.

#### Scenario: The gateway is unreachable

- **WHEN** the model gateway fails mid-chain and a draft already exists for the pair
- **THEN** the stored draft is unchanged and the request reports failure
