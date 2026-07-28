# experience-bank Specification

## Purpose
The durable record of what a candidate has actually done — employments and the evidence
atoms attached to them — and the surfaces that let its owner see, correct and extend it.

## Requirements
### Requirement: The bank names a way into the interviewer in every state

The experience view SHALL offer a labelled action that opens the `profile` interviewer,
and SHALL offer it whether the bank holds entries or none — the empty bank is the one that
most needs filling. The action SHALL be named for what the candidate gets rather than for
the machine that produces it, and SHALL be accompanied by a concrete example of an answer,
so the expected grain of a reply (one result, ideally carrying a number) is visible before
the conversation starts.

#### Scenario: A populated bank offers the action

- **WHEN** a signed-in user opens the experience view with achievements on record
- **THEN** an action opening the interviewer is shown alongside the count, with an example
  of the kind of achievement to describe

#### Scenario: An empty bank offers the same action

- **WHEN** a signed-in user opens the experience view with nothing recorded
- **THEN** the explanation of what the bank is for is shown together with the same action
  and example, rather than text alone

### Requirement: Entering the interviewer starts the conversation

Opening the assistant under the `profile` preset SHALL begin the interview without
requiring the candidate to compose an opening message: the entry SHALL create a new
session and send a first message on the candidate's behalf, so the agent's first response
is a question about a thin spot in their bank. The message SHALL be sent only into a
session with no history, so reloading or reopening a conversation that has already started
never repeats it. Entering the assistant by any other route — the account nav, a saved
chat URL, "New chat" — SHALL send nothing and open silent.

#### Scenario: The interview opens on a question

- **WHEN** the candidate follows the experience view's action into the assistant
- **THEN** a new `profile` session is created, an opening message is recorded as theirs,
  and the agent's first reply asks about a specific gap in their bank

#### Scenario: The opening message survives the move to the session's address

- **WHEN** the entry rewrites the address to the newly created session's own URL while
  that first turn is streaming
- **THEN** the turn continues to completion and its answer is rendered, rather than being
  aborted and leaving the candidate's message unanswered

#### Scenario: A conversation with history is never re-opened for them

- **WHEN** the candidate reloads a `profile` session that already holds messages
- **THEN** the stored transcript is repainted and no further opening message is sent

#### Scenario: Other entries stay silent

- **WHEN** the candidate opens the assistant from the account navigation or starts a new
  chat from the session rail
- **THEN** no message is sent on their behalf and the composer waits for them
