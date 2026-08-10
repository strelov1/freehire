## ADDED Requirements

### Requirement: Voice mode is offered only when the credential is configured

The `interview` session view SHALL offer a "Voice mode" entry point only when the
backend's Realtime credential is configured. When it is not configured, the entry point
SHALL NOT render, following the same nil-client convention `internal/speech` uses for
dictation.

#### Scenario: Entry point hidden when unconfigured

- **WHEN** the deployment has no Realtime credential configured
- **THEN** the `interview` session view does not offer a voice mode entry point

#### Scenario: Entry point shown when configured

- **WHEN** the deployment has a Realtime credential configured
- **THEN** the `interview` session view offers a voice mode entry point

### Requirement: Starting voice mode mints a session-scoped credential

Starting voice mode SHALL request a short-lived Realtime API client secret scoped to
one existing `interview` session. The minting endpoint SHALL resolve the session
through the caller's own ownership, the same way other `interview` preset endpoints do,
so a session the caller does not own is reported as missing rather than as forbidden.
Only a session carrying the `interview` preset SHALL be eligible.

#### Scenario: Owner can start voice mode

- **WHEN** the caller starts voice mode on an `interview` session they own
- **THEN** the backend mints and returns a short-lived client secret scoped to that
  session

#### Scenario: Another candidate's session cannot be used

- **WHEN** a caller requests a voice-mode credential for a session they do not own
- **THEN** the request is answered as not found, and no credential is minted

#### Scenario: A non-interview session is not eligible

- **WHEN** a caller requests a voice-mode credential for a session carrying a preset
  other than `interview`
- **THEN** the request is refused and no credential is minted

### Requirement: The voice session carries the same context as the text mode

The minted Realtime session's instructions SHALL include the same vacancy, application
stage, fit-analysis requirements with evidence, and employer invitation (when present)
that the text mode's `interview_context` tool provides, fetched once at mint time.

#### Scenario: Instructions include the rehearsal context

- **WHEN** a voice-mode credential is minted for a session
- **THEN** the Realtime session's instructions include that session's vacancy, stage,
  requirements with evidence, and employer invitation if one exists

### Requirement: A voice turn cannot write to the candidate's profile

A voice-mode conversation SHALL NOT have access to any tool that mutates candidate
data. In particular, nothing said in a voice turn SHALL be written to the candidate's
experience bank, regardless of what the candidate confirms verbally.

#### Scenario: Experience bank is not writable from voice

- **WHEN** a candidate states a new fact about their experience during a voice-mode
  turn
- **THEN** no experience-bank atom is created from that turn

### Requirement: Completed voice turns persist to the session transcript

Each completed exchange in a voice-mode call (the candidate's transcribed line, the
agent's spoken line) SHALL be appended to the same session transcript the text mode
writes to, as soon as that turn completes. A session's transcript SHALL read the same
way regardless of which turns were typed and which were spoken, so the session's own
history view is complete and a candidate can continue a call by typing without losing
what was said out loud.

#### Scenario: A spoken turn appears in the transcript

- **WHEN** the candidate and the agent complete one spoken exchange in voice mode
- **THEN** both lines are appended to the session's transcript before the next turn
  begins

#### Scenario: A session can continue in text after voice turns

- **WHEN** a candidate sends a typed message to an `interview` session that already
  holds voice-mode turns
- **THEN** the model's context includes those prior voice turns alongside the typed
  ones, in the order they happened

### Requirement: A dropped call does not lose already-completed turns

If a voice-mode call ends before the conversation is finished (network loss, tab
close, or any other disconnect), every turn that had already completed SHALL remain in
the session transcript. A turn that was in progress at the moment of disconnect MAY be
lost.

#### Scenario: Prior turns survive a mid-call disconnect

- **WHEN** a voice-mode call disconnects after three completed exchanges but before a
  fourth finishes
- **THEN** the session transcript contains the three completed exchanges
