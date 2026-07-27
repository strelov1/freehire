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
- **THEN** the response contains every session owned by the caller and no session owned by anyone else

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
