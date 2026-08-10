## ADDED Requirements

### Requirement: The panel talks to freehire's own assistant

The panel's chat SHALL run against freehire's in-process assistant over its HTTP
surface, and SHALL NOT depend on any separate agent service. A turn SHALL be one
request whose response body streams the turn's events, so nothing is held open
between turns.

The panel SHALL authenticate with the session JWT the connect flow stored,
presented as `Authorization: Bearer`. hire's session cookie is not available to
extension code across origins, so it is never relied on.

#### Scenario: A message runs a turn

- **WHEN** the user sends a message
- **THEN** the panel posts it to the caller's conversation and renders the streamed events in order, ending with the turn's single terminal event

#### Scenario: Signing out drops the conversation

- **WHEN** the user signs out
- **THEN** the panel forgets the conversation and its transcript, so a later sign-in never resumes the previous user's chat

### Requirement: A turn can be cancelled and its failure is legible

The panel SHALL let the user stop a turn in flight, and stopping SHALL be reported
as a normal outcome rather than an error. A turn that fails for any other reason
SHALL leave the panel usable and say what happened, rather than appearing to still
be working.

#### Scenario: The user stops a running turn

- **WHEN** the user cancels a turn that is streaming
- **THEN** the stream is abandoned, the composer becomes available again, and no error is presented

#### Scenario: The assistant is unreachable

- **WHEN** a turn cannot be started
- **THEN** the panel says so and the composer becomes available again

### Requirement: A conversation survives the panel closing

The panel SHALL remember which conversation it is holding and SHALL repaint that
conversation's transcript when it is reopened. A conversation the server no longer
has SHALL start a fresh one rather than presenting an error, since a deleted
conversation is not something the user can act on from here.

The panel SHALL create its conversation under the browsing preset, which is what
gives its agent the page tool.

#### Scenario: Reopening resumes

- **WHEN** the panel is closed mid-conversation and opened again
- **THEN** the prior exchange is on screen and the next message continues the same conversation

#### Scenario: The remembered conversation is gone

- **WHEN** the panel reopens and the conversation it remembered no longer exists
- **THEN** it starts a new one silently

#### Scenario: Starting over

- **WHEN** the user asks for a new chat
- **THEN** a fresh conversation replaces the current one and the transcript is cleared

### Requirement: The panel shows what the agent is doing

While a turn runs, the panel SHALL show the tools the agent calls, so a pause is
legible as work rather than as a hang. Vacancies SHALL be rendered as cards from
the agent's own presentation tool; the panel SHALL NOT render a vacancy from prose,
because the agent is instructed never to write one there.

#### Scenario: A tool call is visible

- **WHEN** the agent calls a tool during a turn
- **THEN** the panel shows that it did, and shows when it finished

#### Scenario: Vacancies arrive as cards

- **WHEN** the agent presents vacancies
- **THEN** each is drawn as a card carrying its title, company and the agent's reason for it

### Requirement: The page reaches the agent through the agent's own tool

The panel SHALL NOT offer a manual "read this page" action. The agent decides when
to look, as it decides when to search, and asking the user to hand it the page
duplicates that decision badly — the page it was handed goes stale the moment the
user navigates.

#### Scenario: The user refers to the open page

- **WHEN** the user asks about the page they are on without attaching it
- **THEN** the agent obtains it through its own tool, and the panel needs no affordance for it
