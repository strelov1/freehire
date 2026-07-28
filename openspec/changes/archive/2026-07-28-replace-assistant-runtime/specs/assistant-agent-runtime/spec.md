## ADDED Requirements

### Requirement: The assistant runs inside the freehire backend

The assistant SHALL execute in the freehire API process. A conversation turn
SHALL require no software installed by the user, no process on the user's
machine, and no separate agent service. The user SHALL be able to send a message
immediately after opening the assistant, with no setup step and no connected-device
precondition.

#### Scenario: A first-time user sends a message with no setup

- **WHEN** a signed-in beta tester opens the assistant for the first time and sends a message
- **THEN** the turn runs and streams a reply, with no installation prompt and no "connect a machine" state anywhere in the flow

#### Scenario: No external agent service is contacted

- **WHEN** a turn runs
- **THEN** the model is called through the backend's configured LLM gateway and every tool executes in-process; no request is made to a separate agent backend

### Requirement: A turn is a bounded tool-calling loop

A turn SHALL be a loop in which the model is called with the conversation
history and the session's tool schemas; when the model returns tool calls the
system SHALL execute them, append their results to the history, and call the
model again. The loop SHALL stop when the model answers without tool calls, or
when a configured maximum number of tool-call rounds is reached. On reaching the
maximum the system SHALL make one final model call with no tools offered, so the
turn always ends in a text answer rather than an unbounded chain.

#### Scenario: A turn that needs tools ends in an answer

- **WHEN** the model answers a question by calling `facets` and then `search_jobs`
- **THEN** both tools run, their results are fed back to the model, and the turn ends with the model's text answer

#### Scenario: The step cap forces a final answer

- **WHEN** the model keeps requesting tool calls past the configured maximum rounds
- **THEN** the system stops offering tools, requests one final answer, and ends the turn with that text — the loop does not continue

#### Scenario: A turn that needs no tools

- **WHEN** the user sends a message the model answers directly
- **THEN** no tool is executed and the turn ends with the model's text answer

### Requirement: Tools act as the authenticated caller

Every tool SHALL execute as the authenticated owner of the session, resolved
server-side from the request's session. The system MUST NOT mint, store, or pass
any API credential to run a tool. A tool that reads or writes user-owned data —
CVs, saved and applied jobs, tracking notes — SHALL be confined to the caller's
own rows by the same ownership checks the equivalent HTTP endpoints apply.

#### Scenario: A tool cannot reach another user's data

- **WHEN** the model calls a CV tool with a CV id owned by a different user
- **THEN** the tool fails as not found, no document is read or mutated, and the failure is reported back to the model as a tool error

#### Scenario: No credential is issued for a session

- **WHEN** a chat or tailoring session is created
- **THEN** no API key is minted and the session response contains no credential

### Requirement: The tool surface mirrors the CLI's job-seeker commands

The agent SHALL be given typed tools covering the same operations the `freehire`
CLI exposes to a job seeker: reading the filter vocabulary (`facets`), searching
vacancies with keyword and facet filters (`search_jobs`), reading one vacancy
(`get_job`) or company (`get_company`), scoring a skill set against the market
(`market_fit`), saving, unsaving and applying to a vacancy, setting an
application stage or note (`track_job`), listing the caller's tracked jobs
(`my_jobs`), and — in a tailoring session — reading the tailoring context and CV
document and applying a CV patch. Rendering the CV SHALL NOT be a tool: the
workspace already previews the document beside the chat, so a render would return
bytes the model cannot read and the user a copy of what is on screen. Each tool
SHALL declare a
JSON schema for its arguments and return structured data, not human-formatted
text. Moderator-only operations (job authoring, submission review) SHALL NOT be
exposed.

#### Scenario: Search results carry the data needed to recommend

- **WHEN** the model calls `search_jobs`
- **THEN** the result contains each hit's structured fields including its `public_slug` and its full description, so the model can screen the set without a follow-up call per hit

#### Scenario: An action tool changes the caller's state

- **WHEN** the model calls the apply tool for a vacancy on the user's behalf
- **THEN** the vacancy is recorded as applied for that user, exactly as the equivalent HTTP endpoint would record it, and the result reports the new state

#### Scenario: Moderator operations are unavailable

- **WHEN** a session is created for a user who is also a moderator
- **THEN** no job-authoring or submission-review tool is offered to the model

### Requirement: Malformed tool calls are reported to the model, not crashed on

Tool arguments SHALL be decoded strictly: unknown fields, missing required
fields, and type mismatches SHALL be rejected. A rejected call and a failing
tool SHALL both be returned to the model as a tool result describing the error,
so it can correct itself within the same turn. Such a failure SHALL count
against the turn's step cap and MUST NOT abort the turn or surface as a request
error.

#### Scenario: A mis-shaped argument object is corrected in-turn

- **WHEN** the model calls a tool with an argument object that fails strict decoding
- **THEN** the turn continues with a tool result naming the decoding problem, and the model may retry the call

#### Scenario: A failing dependency does not kill the turn

- **WHEN** a tool fails because a backing service is unavailable
- **THEN** the error is returned to the model as that tool's result and the turn continues within its step cap

### Requirement: Tool results are bounded before entering the conversation

The system SHALL cap the size of a tool result appended to the model's history,
truncating oversized payloads with an explicit marker. The conversation history
sent to the model SHALL likewise be bounded, keeping the most recent messages
when a session grows long.

#### Scenario: A large search result is truncated, not dropped

- **WHEN** a tool returns a payload larger than the configured cap
- **THEN** the model receives a truncated result that states it was truncated, and the turn proceeds

### Requirement: A turn is streamed as named events

The system SHALL stream a turn to the client as a sequence of named events over
a single HTTP response, covering at least: the user's prompt as recorded, the
assistant's text as it is produced, each tool invocation with its name and
arguments, each tool's result, and a terminal event carrying the stop reason and
whether the turn errored. The stream SHALL survive an idle model by emitting a
periodic keep-alive.

#### Scenario: The client renders a turn as it happens

- **WHEN** a turn calls a tool and then answers
- **THEN** the client receives the tool invocation, then the tool result, then the assistant's text incrementally, then a terminal event with the stop reason

#### Scenario: A failed turn ends with an explicit terminal event

- **WHEN** the model call fails irrecoverably mid-turn
- **THEN** the stream emits a terminal event marking the turn as errored, and the client is not left waiting

### Requirement: A turn stops when the caller goes away

The system SHALL cancel an in-flight turn when the client disconnects or
explicitly cancels, stopping before the next model call and not starting new
tool work. Work already committed by a tool (an applied job, a CV patch) SHALL
remain committed; the partial transcript up to the cancellation SHALL be
persisted so the session is resumable.

#### Scenario: Closing the tab stops the turn

- **WHEN** the user closes the assistant while a turn is streaming
- **THEN** the turn stops at the next boundary, no further model call is made, and the messages produced so far are stored

### Requirement: A session's preset selects its prompt and its tools

A session SHALL record a preset. The general-chat preset SHALL offer the
discovery and tracking tools; the CV-tailoring preset SHALL additionally offer
the CV tools and SHALL be bound to the tailored CV and its vacancy. The preset
SHALL select the system prompt. No other behaviour SHALL differ between presets,
so the same chat surface serves both.

#### Scenario: A chat session has no CV tools

- **WHEN** a general chat session runs a turn
- **THEN** no CV-editing tool is offered to the model

#### Scenario: A tailoring session is bound to its CV

- **WHEN** a tailoring session runs a turn
- **THEN** the CV tools are offered and operate on the CV the session is bound to, and the tailoring context for its vacancy is available to the model

### Requirement: A resumed session continues the model's history

When a turn is run in an existing session, the system SHALL reconstruct the
model's conversation history from the session's stored messages — including
prior assistant tool calls and their results — so the model continues the
conversation rather than restarting it.

#### Scenario: A follow-up references earlier work

- **WHEN** the user sends "save the second one" after an earlier turn listed vacancies
- **THEN** the model's history contains that earlier turn's tool calls and results, and it can act on the referenced vacancy without searching again
