## MODIFIED Requirements

### Requirement: Owner-scoped session backend contract

The freehire API SHALL own assistant sessions and SHALL scope every session
operation to the authenticated caller. Creating a session SHALL persist it for
that caller. Listing SHALL return only sessions owned by the caller, newest
first. Reading a session's transcript SHALL return only that caller's session's
messages. Deleting SHALL remove an owned session and its transcript so it no
longer appears in the list, cannot be read, and cannot accept a further turn.
An operation naming a session the caller does not own or that does not exist
SHALL fail without side effects and MUST NOT reveal whether the session exists.

#### Scenario: List is filtered by owner

- **WHEN** the session list is requested with a valid session cookie
- **THEN** the response contains every chat owned by the caller and no session owned by anyone else

#### Scenario: A tailoring conversation is not a chat

- **WHEN** the caller has a CV-tailoring conversation and requests the session list
- **THEN** it is absent from the response, and the chat sidebar therefore never offers it

#### Scenario: Delete an owned session

- **WHEN** a delete is requested for a session the caller owns
- **THEN** the session and its stored messages are removed and success is returned; a subsequent list omits it and a turn sent to it is rejected

#### Scenario: Delete a non-owned or unknown session

- **WHEN** a delete names a session the caller does not own or that does not exist
- **THEN** the request fails, makes no change, and does not distinguish the two cases

#### Scenario: Reading another user's transcript is refused

- **WHEN** a transcript read names a session owned by a different user
- **THEN** the request fails and no message content is returned

### Requirement: Switching between sessions replays history

Selecting a session in the sidebar SHALL make it the active pane and repaint its
full prior transcript. The page SHALL load that session's stored transcript from
the freehire API and fold it through the existing chat reducer, so replayed
history and live turn events render identically.

#### Scenario: Selecting an older session shows its messages

- **WHEN** the user clicks a session that already contains a multi-message exchange
- **THEN** the chat pane clears and repaints that session's full user/assistant/tool history in order, and further messages continue that conversation

#### Scenario: Switching away and back preserves the conversation

- **WHEN** the user switches from session A to session B and later back to A
- **THEN** session A's transcript is shown again in full, reconstructed from the stored messages

#### Scenario: Switching mid-turn is safe

- **WHEN** the user switches sessions while a turn is streaming in the current one
- **THEN** the page ends the in-flight turn cleanly for the old session and does not interleave its events into the newly selected session

## ADDED Requirements

### Requirement: A session id is unguessable

A session's id SHALL be random, not sequential. Access is confined by ownership
on every operation, so the id is not a capability — but a countable id would
publish how many conversations the platform has created, and would turn a single
missing owner check on any future endpoint into bulk extraction rather than one
unusable request. An id that is not well-formed SHALL be reported as a missing
session, so "not a session" and "not yours" stay one answer.

#### Scenario: Two sessions get unrelated ids

- **WHEN** the same caller creates two conversations in a row
- **THEN** their ids are independently random, so neither reveals the other nor how many exist

#### Scenario: A malformed id is missing, not invalid

- **WHEN** a request names a session id that is not well-formed
- **THEN** it is refused as not found, indistinguishable from a session the caller does not own

### Requirement: A conversation has its own address

Each chat SHALL be addressable by a URL carrying its id, so it can be
bookmarked, reopened later, and reached with the browser's Back button.
Selecting a chat SHALL navigate to that chat's address, adding a history entry;
entering the assistant without an id SHALL open the newest chat (or start one)
and replace the address with that chat's own, so landing there is not itself a
step in the user's history.

#### Scenario: Switching chats changes the address

- **WHEN** the user selects a different chat in the sidebar
- **THEN** the address becomes that chat's URL, and pressing Back returns to the chat they came from

#### Scenario: A saved link reopens its chat

- **WHEN** the user opens a previously saved chat URL
- **THEN** that chat is opened and its transcript is repainted

#### Scenario: A dead link explains itself

- **WHEN** the URL names a conversation the caller cannot open — deleted, another user's, or a tailoring conversation that belongs to a CV
- **THEN** the page says the chat is unavailable and offers a way back to the chat list, rather than silently opening a different conversation
