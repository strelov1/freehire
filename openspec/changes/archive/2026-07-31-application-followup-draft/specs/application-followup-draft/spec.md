## ADDED Requirements

### Requirement: A follow-up draft is offered for a silent application

The system SHALL assemble a follow-up message for an application the candidate owns, containing at
minimum the role, the company, and how long it has been since the application last moved. The draft
SHALL be assembled deterministically — the same application and the same underlying data produce the
same text — and MUST NOT call a language model or consume AI credits.

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

### Requirement: The draft states one true strength, taken from the fit analysis

The draft SHALL include one concrete reason the candidate fits the role, taken from the cached fit
analysis for that (candidate, vacancy) pair — which derives it from the candidate's own CV. The
system MUST NOT compose a new claim about the candidate: if no cached analysis exists, or it names
no strength, the draft SHALL omit the line rather than invent one.

#### Scenario: A cached analysis supplies the line
- **WHEN** a fit analysis is cached for the application's vacancy and names at least one strength
- **THEN** the draft states that strength

#### Scenario: No analysis means no claim
- **WHEN** no fit analysis is cached, or the cached one names no strengths
- **THEN** the draft is still assembled and simply carries no strength line

### Requirement: The recipient is prefilled only when it is known

When mail linked to the application carries a sender address, the draft SHALL prefill that address
and display name as the recipient. When no linked mail exists — the commonest silent case, an
application nobody ever answered — the draft SHALL be issued with no recipient rather than withheld
or guessed.

#### Scenario: A replied-then-quiet application prefills its recipient
- **WHEN** the application has linked mail carrying a sender address
- **THEN** the draft's recipient is that address

#### Scenario: An unanswered application drafts without a recipient
- **WHEN** the application has no linked mail
- **THEN** the draft is still returned, with no recipient set

### Requirement: The system does not send the follow-up

The system SHALL NOT transmit the follow-up. The draft is handed to the candidate to send from their
own mail client, and no endpoint in this capability delivers mail to a third party.

#### Scenario: Requesting a draft sends nothing
- **WHEN** a draft is requested
- **THEN** no message is transmitted to any address

### Requirement: A recorded follow-up does not stop the silence clock

The candidate SHALL be able to record that they followed up, and the system SHALL store when. That
record MUST NOT feed the last-activity derivation: silence measures how long the *other side* has
been quiet, and the candidate chasing is not a reply. An application that was silent for 24 days and
has since been chased SHALL still report its silence, alongside the fact that it was chased.

#### Scenario: Chasing does not reset the silence
- **WHEN** the candidate records a follow-up on an application silent for 24 days
- **THEN** the application still reports silence, and its days-silent keeps counting from the last
  inbound activity

#### Scenario: A reply does reset it
- **WHEN** mail linked to the application arrives after the recorded follow-up
- **THEN** the last activity moves to that mail, as it would have without any follow-up

#### Scenario: The record is owner-scoped
- **WHEN** a caller records a follow-up on an application belonging to a different account
- **THEN** the request is refused as not found, and nothing is written
