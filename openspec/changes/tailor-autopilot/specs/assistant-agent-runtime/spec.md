## MODIFIED Requirements

### Requirement: A turn is a bounded tool-calling loop

A turn SHALL be a loop in which the model is called with the conversation
history and the session's tool schemas; when the model returns tool calls the
system SHALL execute them, append their results to the history, and call the
model again. The loop SHALL stop when the model answers without tool calls, or
when the maximum number of tool-call rounds for that turn is reached. On reaching the
maximum the system SHALL make one final model call with no tools offered, so the
turn always ends in a text answer rather than an unbounded chain. The maximum MAY be
chosen per turn — a turn that runs a whole unattended workflow needs more rounds than a
question does — but it MUST be chosen server-side and MUST fall back to the configured
default when a turn names none. A client MUST NOT be able to raise it.

#### Scenario: A turn that needs tools ends in an answer

- **WHEN** the model answers a question by calling `facets` and then `search_jobs`
- **THEN** both tools run, their results are fed back to the model, and the turn ends with the model's text answer

#### Scenario: The step cap forces a final answer

- **WHEN** the model keeps requesting tool calls past the maximum rounds for that turn
- **THEN** the system stops offering tools, requests one final answer, and ends the turn with that text — the loop does not continue

#### Scenario: A turn that needs no tools

- **WHEN** the user sends a message the model answers directly
- **THEN** no tool is executed and the turn ends with the model's text answer

#### Scenario: A turn naming no maximum uses the configured default

- **WHEN** a turn is run without a per-turn maximum
- **THEN** the loop is bounded by the configured default, unchanged from before
