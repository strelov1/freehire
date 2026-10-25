## ADDED Requirements

### Requirement: Per-application apply session
The system SHALL let a signed-in candidate create an assistant session with preset `apply`
bound to one application they hold, and SHALL resume the existing live session for that
(candidate, job) instead of creating a second one.

#### Scenario: Candidate opens the chat for their application
- **WHEN** a candidate creates a session with `preset=apply` and the slug of a job they track
- **THEN** a session bound to that job is returned, labelled with the vacancy

#### Scenario: Reopening resumes the same conversation
- **WHEN** the candidate creates an `apply` session for a job that already has one
- **THEN** the existing session is returned and no new session is created

#### Scenario: A job the candidate does not hold is refused
- **WHEN** a candidate creates an `apply` session for a job with no application of theirs
- **THEN** the request is refused and no session is created

### Requirement: Tools scoped to the session's application
An `apply` session SHALL offer the tools to read the attempt, answer its questions (answer
bank and screening answers), retry it, review it and cancel it, each acting only on the
session's own job and taking no job identifier from the model. These tools SHALL NOT require
the `agents_may_apply` switch. They SHALL keep every existing auto-apply rule: answers are
the candidate's, one review decision per attempt, and an attempt that may already have
reached the employer is never retried or cancelled.

#### Scenario: The model acts on the session's attempt
- **WHEN** the model calls the retry tool in an `apply` session
- **THEN** the session's job's attempt is retried, with no job argument accepted

#### Scenario: The switch for connected agents does not apply
- **WHEN** a candidate whose `agents_may_apply` is off answers and approves in the chat
- **THEN** the review is recorded

#### Scenario: Other presets do not gain the tools
- **WHEN** a `chat` or `profile` session lists its tools
- **THEN** none of the auto-apply tools are offered

### Requirement: Catch-up narration on sync
The system SHALL provide a sync call for an `apply` session that, when the attempt has
events the session has not yet narrated, runs one model turn over those events and the
attempt's current state, stores the turn's input as a hidden system note, and advances the
session's narrated-event cursor only after the turn is stored. With no new events it SHALL
run no turn. A catch-up turn SHALL NOT draw on the daily assistant-message allowance.

#### Scenario: Nothing new
- **WHEN** sync is called and no event is newer than the cursor
- **THEN** no turn runs and the cursor is unchanged

#### Scenario: The attempt stopped on questions
- **WHEN** sync is called after the attempt became `blocked`
- **THEN** one assistant message is written listing the open questions as one numbered list,
  and the cursor advances past the narrated events

#### Scenario: A failed catch-up is retried
- **WHEN** the catch-up turn fails
- **THEN** the cursor is unchanged and the next sync narrates the same events

#### Scenario: Narration is not metered
- **WHEN** a catch-up turn runs
- **THEN** the candidate's assistant-message allowance is unchanged

### Requirement: Language
In an `apply` session the model SHALL write in the candidate's site locale, and SHALL save
answers for the employer in the language of the application form, quoting them as they will
be sent before asking for approval.

#### Scenario: Russian-speaking candidate, English form
- **WHEN** a candidate whose site locale is Russian answers a question in Russian
- **THEN** the chat replies in Russian, and the saved answer is in English

### Requirement: Opening the chat from the application
The job drawer SHALL offer to open the attempt's `apply` chat in the docked assistant panel
whenever the attempt is `blocked` or `pending_review`, while keeping the existing
per-question form as an alternative. The chat SHALL call sync when it opens and periodically
while it stays open.

#### Scenario: Blocked attempt
- **WHEN** a candidate opens the drawer of a `blocked` application
- **THEN** an action to answer in chat is shown alongside the form

### Requirement: MCP asks for one numbered list
The signed-in MCP server's guidance and `auto_apply_get`'s `next_step` SHALL ask a connected
agent to put every open question to the person as one numbered list and to translate answers
into the form's language.

#### Scenario: Agent reads a blocked attempt
- **WHEN** an agent calls `auto_apply_get` on a `blocked` attempt
- **THEN** `next_step` tells it to ask all open questions as one numbered list
