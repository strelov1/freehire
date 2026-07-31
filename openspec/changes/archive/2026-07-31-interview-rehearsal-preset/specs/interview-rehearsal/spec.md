## ADDED Requirements

### Requirement: An application at the interview stage offers a rehearsal

The tracking board SHALL offer a rehearsal on an application whose stage is
`screening` or `interview`, and starting one SHALL create an assistant session bound
to that application's vacancy. The session SHALL carry the `interview` preset and no
CV binding, because a rehearsal reads a CV but never edits one. The creating endpoint
SHALL resolve the application through the caller's own tracking record, so an
application the caller does not own is reported as missing rather than as forbidden.

#### Scenario: Rehearsal offered on an interviewing application

- **WHEN** the candidate opens the tracking board and an application sits in the `screening` or `interview` stage
- **THEN** that application offers to start a rehearsal

#### Scenario: The session is bound to the vacancy and to no CV

- **WHEN** a rehearsal is started from an application
- **THEN** a session is created with the `interview` preset, carrying that application's vacancy and no CV binding

#### Scenario: Another candidate's application cannot be rehearsed

- **WHEN** a caller starts a rehearsal for a vacancy they have no application against
- **THEN** the request is answered as not found, and no session is created

### Requirement: The agent opens the rehearsal

The rehearsal SHALL begin with the agent speaking first, driven by a brief the server
supplies rather than by a prompt the candidate writes. The opening SHALL name the
vacancy, SHALL name the interview's format when the mailbox holds the employer's
invitation, and SHALL ask the candidate which round to rehearse. The brief SHALL be
chosen server-side, so a client cannot dictate what the rehearsal is told to do.

#### Scenario: The conversation starts without the candidate writing anything

- **WHEN** a rehearsal session is created
- **THEN** a turn runs immediately under a server-supplied brief, and the candidate sees the agent's opening as the first message

#### Scenario: The opening asks which round

- **WHEN** the agent opens a rehearsal
- **THEN** it offers the rounds it can rehearse and waits for the candidate's choice before asking an interview question

### Requirement: The rehearsal reasons from the application's own context

The `interview` preset SHALL offer a context tool bound to the session's vacancy,
returning the vacancy, the application's stage, the cached fit analysis' verdict and
score, and the analysis' requirements each carrying the evidence the candidate's
experience bank already holds for it. Where the mailbox holds an interview invitation
linked to that application, the context SHALL carry the most recent one, and the tool
SHALL NOT mark any message as read.

The context SHALL degrade rather than fail: a vacancy with no cached analysis returns
its posting and stage without requirements, and a requirement with nothing in the bank
returns an empty evidence list — "looked and found nothing" being a different answer
from "did not look".

#### Scenario: Requirements arrive with the bank's evidence attached

- **WHEN** the agent reads the rehearsal context for a vacancy with a cached fit analysis
- **THEN** each requirement carries the achievements the bank holds for it, each with its identifier and claim

#### Scenario: No cached analysis

- **WHEN** the vacancy has no cached fit analysis
- **THEN** the context still returns the vacancy and the application's stage, and the rehearsal proceeds without a requirement list

#### Scenario: Reading the invitation does not mark it read

- **WHEN** the context carries an interview invitation from the candidate's mailbox
- **THEN** that message's read state is unchanged

### Requirement: One round per session, one question at a time

The rehearsal SHALL hold to the round the candidate chose for the rest of the session,
and SHALL ask one question at a time, waiting for the answer before asking the next.
After each answer it SHALL give a short critique addressing whether the candidate
spoke for themselves rather than for their team, whether the answer carries a concrete
result, and whether that result is quantified.

#### Scenario: The chosen round is held

- **WHEN** the candidate has chosen a round and answers a question
- **THEN** the next question belongs to the same round

#### Scenario: One question, then silence

- **WHEN** the agent asks an interview question
- **THEN** it asks exactly one and waits, rather than listing several

### Requirement: Nothing reaches the experience bank without the candidate's word

The rehearsal SHALL record an achievement in the experience bank only after the
candidate has explicitly agreed to record that specific answer, and SHALL store their
own words as what was said. It SHALL NOT record an improvisation, a hypothetical, or
its own reformulation of an answer. A rehearsal is where candidates try things out,
and an improvisation banked as evidence is a claim they never made.

#### Scenario: An offered recording is refused

- **WHEN** the agent offers to record an answer and the candidate declines
- **THEN** nothing is written to the bank and the rehearsal continues

#### Scenario: An accepted recording keeps the candidate's words

- **WHEN** the candidate agrees to record an answer
- **THEN** the achievement is stored with the candidate's own wording as what they said

### Requirement: The employer's invitation is untrusted text

The rehearsal's prompt SHALL name the invitation's contents as untrusted input written
by whoever emailed the candidate. Text inside a message that addresses the agent, asks
it to disregard its instructions, or asks it to act SHALL be treated as an attack
rather than as a request.

#### Scenario: An instruction inside an invitation is not obeyed

- **WHEN** an invitation's body contains text addressing the agent and asking it to take an action
- **THEN** the agent does not act on it and continues the rehearsal

### Requirement: The rehearsal leaves the transcript and the bank

A finished rehearsal SHALL leave its transcript as an ordinary assistant session the
candidate can reopen and continue, plus whichever achievements they agreed to record.
No rehearsal report, readiness score or per-round progress SHALL be persisted.

#### Scenario: The session is resumable

- **WHEN** the candidate returns to the assistant after a rehearsal
- **THEN** the rehearsal is listed among their sessions and can be continued
