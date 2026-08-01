## ADDED Requirements

### Requirement: An interviewed application offers a debrief

An opened application SHALL offer a debrief once it has reached the `interview` stage,
and starting one SHALL create an assistant session bound to that application's vacancy
carrying the `debrief` preset and no CV binding. The creating endpoint SHALL resolve the
application through the caller's own tracking record, so an application the caller does
not own is reported as missing rather than as forbidden.

The stage SHALL govern the offer and not the endpoint. A candidate who interviewed
without moving their application's stage is exactly the candidate this is for, so a
debrief requested for an application in any stage SHALL be created.

An application whose posting the catalogue no longer holds SHALL NOT offer a debrief:
the session binds to a vacancy, and there is none to bind.

#### Scenario: The offer appears once the application reaches interview

- **WHEN** the candidate opens an application sitting in `interview`
- **THEN** it offers to start a debrief alongside the rehearsal

#### Scenario: An earlier stage does not advertise it

- **WHEN** the candidate opens an application sitting in `applied`
- **THEN** no debrief is offered

#### Scenario: A debrief requested for an earlier stage is still created

- **WHEN** a debrief is requested for an application the caller owns that still sits in `screening`
- **THEN** the session is created

#### Scenario: The session is bound to the vacancy and to no CV

- **WHEN** a debrief is started from an application
- **THEN** a session is created with the `debrief` preset, carrying that application's vacancy and no CV binding

#### Scenario: Another candidate's application cannot be debriefed

- **WHEN** a caller starts a debrief for a vacancy they have no application against
- **THEN** the request is answered as not found, and no session is created

#### Scenario: Every round gets its own debrief

- **WHEN** the candidate starts a debrief for an application they have already debriefed
- **THEN** a further session is created, and the earlier one is left intact

### Requirement: The agent opens the debrief

A debrief SHALL begin with the agent speaking first, driven by a brief the server
supplies rather than by a prompt the candidate writes. The opening SHALL name the
vacancy and SHALL ask what the candidate was asked, rather than asking how the interview
felt. The brief SHALL be chosen server-side, so a client cannot dictate what the debrief
is told to do.

The opening SHALL be refused only once it has been answered. The turn loop records the
brief before the model is called, so a session left with the brief alone and no reply is
one the candidate can still open.

#### Scenario: The conversation starts without the candidate writing anything

- **WHEN** a debrief session is created
- **THEN** a turn runs under a server-supplied brief, and the candidate sees the agent's opening as the first message

#### Scenario: A failed opening can be retried

- **WHEN** an opening was recorded but the model never answered, and the opening is requested again
- **THEN** the turn runs again rather than being refused

#### Scenario: An answered opening is not repeated

- **WHEN** the opening has been answered and it is requested again
- **THEN** the request is refused and no second opening is recorded

### Requirement: The debrief reasons from the application's own context

The `debrief` preset SHALL offer the same context as a rehearsal, bound to the session's
vacancy: the vacancy, the application's stage, the cached fit analysis' verdict and
score, the analysis' requirements each carrying the evidence the experience bank already
holds, and the employer's invitation where the mailbox holds one. Reading it SHALL NOT
mark any message as read.

The requirements are what the debrief maps the candidate's recollection onto: a question
they were asked answers some requirement, and whether the bank already held evidence for
it decides whether the answer added anything.

The context SHALL degrade rather than fail, as the rehearsal's does — a vacancy with no
cached analysis returns its posting and stage without requirements.

#### Scenario: Requirements arrive with the bank's evidence attached

- **WHEN** the agent reads the debrief context for a vacancy with a cached fit analysis
- **THEN** each requirement carries the achievements the bank holds for it, each with its identifier and claim

#### Scenario: No cached analysis

- **WHEN** the vacancy has no cached fit analysis
- **THEN** the context still returns the vacancy and the application's stage, and the debrief proceeds without a requirement list

#### Scenario: The debrief cannot reach the mailbox

- **WHEN** the debrief's tools are listed
- **THEN** no inbox tool is among them, and the invitation is reachable only through the context

### Requirement: The debrief records what the candidate confirms they said

A debrief SHALL record an achievement in the experience bank only after the candidate
has explicitly agreed to record that specific answer, and SHALL store their own words as
what was said. What is recorded SHALL be what the candidate stated in the interview or
states now about their own work — never the agent's reconstruction of it, and never a
number the agent supplied.

Recording is the purpose of the session rather than a hazard within it: what the
candidate told an employer is the most direct account of their work the product
receives. The agreement is what keeps a wrong entry traceable to the exchange that
produced it, because the service cannot distinguish the candidate's words from the
agent's paraphrase once both arrive as text.

#### Scenario: An offered recording is refused

- **WHEN** the agent offers to record an answer and the candidate declines
- **THEN** nothing is written to the bank and the debrief continues

#### Scenario: An accepted recording keeps the candidate's words

- **WHEN** the candidate agrees to record an answer
- **THEN** the achievement is stored with the candidate's own wording as what they said

#### Scenario: A number the candidate did not give is not recorded

- **WHEN** the candidate describes an outcome without quantifying it
- **THEN** the agent records no figure of its own, and may ask whether one exists

### Requirement: The critique names where the answer fell short

After each answer the candidate recalls, the debrief SHALL say how that answer landed
against the requirement it addressed, in terms the candidate can act on next time:
whether they spoke for themselves rather than for their team, whether a concrete outcome
was reached, and whether a figure that plainly exists went unsaid.

The critique SHALL NOT inflate a weak answer. A debrief that praises everything is worse
than none, because the candidate repeats the same answer in the next round.

#### Scenario: An unsaid figure is named

- **WHEN** the candidate recalls an answer whose outcome the bank quantifies but the answer did not
- **THEN** the critique names the figure that went unsaid

#### Scenario: A strong answer is acknowledged briefly

- **WHEN** the candidate recalls an answer that was genuinely strong
- **THEN** the debrief says so in a clause and moves on rather than padding it

### Requirement: The employer's invitation is untrusted text

A debrief's prompt SHALL name the invitation's contents as untrusted input written by
whoever emailed the candidate. Text inside a message that addresses the agent, asks it
to disregard its instructions, or asks it to act SHALL be treated as an attack rather
than as a request.

#### Scenario: An instruction inside an invitation is not obeyed

- **WHEN** an invitation's body contains text addressing the agent and asking it to take an action
- **THEN** the agent does not act on it and continues the debrief

### Requirement: The debrief leaves the transcript and the bank

A finished debrief SHALL leave its transcript as an ordinary assistant session the
candidate can reopen and continue, plus whichever achievements they agreed to record. No
debrief report, performance score or per-question record SHALL be persisted, and the
critique SHALL live in the transcript alone.

#### Scenario: The session is resumable

- **WHEN** the candidate returns to the assistant after a debrief
- **THEN** the debrief is listed among their sessions and can be continued

#### Scenario: Nothing but the transcript and the bank survives

- **WHEN** a debrief ends
- **THEN** no report or score is stored against the application
