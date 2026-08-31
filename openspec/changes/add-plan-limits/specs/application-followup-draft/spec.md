## MODIFIED Requirements

### Requirement: A follow-up draft is offered for a silent application

The system SHALL assemble a follow-up message for an application the candidate owns, containing at
minimum the role, the company, and how long it has been since the application last moved. The draft
SHALL be assembled deterministically — the same application and the same underlying data produce the
same text — and MUST NOT call a language model or consume a plan allowance.

The draft SHALL be offered only for an application that is actually waiting on somebody: one whose
silence state is `silent`. An application in a terminal stage, one still inside its stage's tolerated
silence, or one with mail awaiting confirmation has nothing to chase and SHALL NOT be offered a
draft.

#### Scenario: A silent application yields a draft
- **WHEN** the candidate requests a follow-up draft for an application whose silence state is `silent`
- **THEN** the response carries a subject and a body naming the role, the company, and the elapsed time

#### Scenario: The same application drafts identically twice
- **WHEN** a draft is requested twice with no change to the application
- **THEN** both responses are identical

#### Scenario: An application that is not silent is refused
- **WHEN** the candidate requests a draft for an application whose silence state is `active`,
  `unconfirmed`, or absent (a terminal stage, or a job merely viewed or saved)
- **THEN** the request is refused and no draft is assembled

#### Scenario: An exhausted allowance does not withhold the draft
- **WHEN** a candidate who has spent every daily allowance requests a follow-up draft for a silent
  application
- **THEN** the draft is assembled and returned, because it calls no model and consumes nothing
