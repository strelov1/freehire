## ADDED Requirements

### Requirement: Deterministic keyword status fast-path

The classifier SHALL, for an email needing no application disambiguation (no
candidate applications offered), first attempt a deterministic keyword
classification of the status and use it WITHOUT calling the LLM when a confident
status is found. When no confident keyword status is found, or when candidate
disambiguation is required, classification SHALL proceed via the LLM as before.

The keyword classification SHALL be precision-first: it SHALL return a status
only for strong, unambiguous signals and SHALL defer otherwise. A rejection
signalled by an explicit negative phrase SHALL take precedence over an
acknowledgement opener in the same email.

#### Scenario: Confident keyword status skips the LLM

- **WHEN** an email with no candidate applications contains an explicit rejection
  phrase ("we regret to inform you… not to proceed")
- **THEN** it is classified as a rejection without an LLM call

#### Scenario: Ambiguous email defers to the LLM

- **WHEN** an email's status is not clear from strong keywords
- **THEN** the classifier falls back to the LLM

#### Scenario: Candidates present always use the LLM

- **WHEN** candidate applications are offered for disambiguation
- **THEN** the classifier uses the LLM regardless of keyword matches
