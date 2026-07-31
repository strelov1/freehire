## ADDED Requirements

### Requirement: A settled turn offers up to three follow-up questions

When a turn ends normally with an answer, the chat SHALL request up to three short follow-up
questions and render them beneath that answer. Each is a plain question in the caller's own
voice — the next thing they would reasonably ask — not a description of what the agent could
do.

Follow-ups SHALL be requested only for a turn that ended with `end_turn` and produced answer
text. A turn that errored, was cancelled, or hit the step ceiling MUST NOT produce them: the
conversation is in a state the caller has to resolve, and suggesting what to ask next reads as
if nothing went wrong.

#### Scenario: An answered turn gets suggestions

- **WHEN** a turn ends with `end_turn` and non-empty answer text
- **THEN** up to three follow-up questions render beneath that answer

#### Scenario: A failed turn gets none

- **WHEN** a turn ends as errored, cancelled, or at the step ceiling
- **THEN** no follow-up questions are requested and none render

#### Scenario: A silent turn gets none

- **WHEN** a turn ends with `end_turn` but produced no answer text
- **THEN** no follow-up questions are requested

### Requirement: Only the newest answer carries follow-ups

Follow-ups SHALL render beneath the last assistant message only, and SHALL be cleared the
moment the next turn starts. Older answers in the same conversation MUST NOT carry them, and
opening a stored conversation MUST NOT request them — a suggestion about what to ask next is
about the present moment, and paying a model call to reconstruct it for history is spend with
no reader.

#### Scenario: Starting the next turn clears them

- **WHEN** follow-ups are shown and the caller sends any message
- **THEN** they disappear before the answer begins streaming

#### Scenario: Replaying history shows none

- **WHEN** a stored conversation is opened
- **THEN** no follow-ups render and no request for them is made

### Requirement: Clicking a follow-up sends it as an ordinary message

Activating a follow-up SHALL send its text as a message from the caller, taking the same path
a typed message takes — including the queue when a turn is already in flight.

#### Scenario: A click starts a turn

- **WHEN** the caller activates a follow-up
- **THEN** its text is sent as their message and the strip clears

### Requirement: Follow-ups render as inert text

A follow-up SHALL be rendered as plain text, never as markup, and SHALL be truncated for
display past a reasonable length. The model that writes them has read job descriptions and
other attacker-controlled text, so a follow-up may be an attacker's words; because activating
one speaks in the caller's voice, what they are agreeing to must be legible and must not be
able to style, link, or hide itself.

The server SHALL cap both the number returned and the length of each, discarding rather than
truncating a suggestion that arrives over the limit.

#### Scenario: Markup in a suggestion is not rendered as markup

- **WHEN** a returned follow-up contains markdown or raw HTML
- **THEN** it is displayed as literal characters and no element from it reaches the DOM

#### Scenario: An overlong suggestion is dropped

- **WHEN** the model returns a follow-up longer than the server's per-item cap
- **THEN** that item is not included in the response

### Requirement: Failing to produce follow-ups is invisible

Follow-up generation SHALL NOT affect the turn it follows. A failure — an unconfigured
gateway, a model error, a timeout, a malformed response — MUST leave the answer intact and
surface no error to the caller; the strip simply does not appear.

#### Scenario: An unconfigured gateway shows no strip

- **WHEN** no model is configured for follow-up generation
- **THEN** the endpoint answers with an empty list and no strip renders

#### Scenario: A model failure shows no strip and no error

- **WHEN** follow-up generation fails
- **THEN** no error is shown to the caller and the answer remains as it was

### Requirement: The follow-up endpoint is owner-scoped

`POST /api/v1/assistant/sessions/:id/followups` SHALL answer
`{"data": {"followups": ["...", "..."]}}` for a session the caller owns, and SHALL report a
session belonging to anyone else as missing rather than as forbidden, matching the rest of the
session API.

It SHALL run on the cheap general-purpose model rather than the assistant's tool-calling
model, and SHALL be given only the most recent exchange rather than the whole transcript.

#### Scenario: Another caller's session is missing

- **WHEN** the caller requests follow-ups for a session they do not own
- **THEN** the response is `404`

#### Scenario: An unauthenticated caller is refused

- **WHEN** the request carries no accepted credential
- **THEN** the response is `401` and no model call is made
