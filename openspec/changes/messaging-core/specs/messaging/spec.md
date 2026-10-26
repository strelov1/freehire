## ADDED Requirements

### Requirement: Conversations have a kind and participants
Every conversation SHALL carry a kind that names the feature it belongs to (today only `buddy`) and a set of participant accounts. Only participants SHALL be able to read, send into, mark read, block from, or report a conversation. A kind SHALL be able to declare a pair key so that at most one conversation of that kind exists per pair of accounts.

#### Scenario: Outsider cannot reach a conversation
- **WHEN** an account that is not a participant reads, sends into, marks read, accepts, declines, blocks from or reports a conversation
- **THEN** the system responds 404, exactly as for a conversation that does not exist

#### Scenario: Pair key prevents a duplicate
- **WHEN** a conversation of a kind already exists for a pair key and a second one of the same kind is started for the same pair
- **THEN** no second conversation is created

#### Scenario: Conversation names its kind and counterpart
- **WHEN** a participant lists their conversations
- **THEN** each entry carries its kind and a counterpart description, together with status, last message time and unread count

### Requirement: Participants speak as a persona
Every participant SHALL speak in a conversation as one persona (today only `buddy`). What a viewer sees of the other participant SHALL be decided by that participant's persona. No response about a conversation SHALL carry anything that identifies the account behind a persona: no account id, no email, and none of the account's other personas.

#### Scenario: Counterpart follows the other participant's persona
- **WHEN** a participant lists their conversations
- **THEN** each entry's counterpart names the other participant's persona and carries only what that persona shows

#### Scenario: Personas are not joined
- **WHEN** a participant reads the list, the messages or a report of a buddy conversation
- **THEN** the response contains no account id or email of the other participant

### Requirement: Request and accept
A conversation SHALL start as pending, holding the starter's first message. The other participant SHALL be able to accept (the conversation becomes open) or decline (it becomes declined). A declined request SHALL NOT notify the starter; the starter SHALL see it as unanswered. Whether an account may start a conversation with another SHALL be decided by the conversation's kind.

#### Scenario: Accepting opens the conversation
- **WHEN** the recipient accepts a pending conversation
- **THEN** it becomes open and both participants can send messages

#### Scenario: Declining is silent
- **WHEN** the recipient declines a pending conversation
- **THEN** it becomes declined, the starter receives no notification, and the starter sees it as unanswered

#### Scenario: Starter cannot accept their own request
- **WHEN** the starter accepts or declines their own pending conversation
- **THEN** the system responds 409

### Requirement: Messages and read state
Participants of an open conversation SHALL be able to send text messages of 1 to 4000 characters and read the conversation's messages oldest first. Sending into a pending, declined or closed conversation SHALL be refused, whichever participant sends. The system SHALL track per participant which messages they have read and report an unread count per conversation.

#### Scenario: Message is delivered
- **WHEN** A sends a message in an open conversation with B
- **THEN** B's next read includes it and B's unread count for the conversation increases by one

#### Scenario: Starter cannot send before acceptance
- **WHEN** the starter sends a second message while the conversation is pending
- **THEN** the system responds 409

#### Scenario: Empty or oversized message is refused
- **WHEN** a message is empty or longer than 4000 characters
- **THEN** the system responds 400

#### Scenario: Reading marks messages read
- **WHEN** B marks the conversation read
- **THEN** B's unread count for it becomes zero

#### Scenario: New messages since a cursor
- **WHEN** the client asks for messages after a given message id
- **THEN** only newer messages are returned, oldest first

### Requirement: Account-wide blocks
A participant SHALL be able to block the other participant of a conversation. A block SHALL be between two accounts, not tied to a kind: it SHALL close every conversation between them permanently and stop either from starting a conversation of any kind with the other. A refusal caused by a block SHALL NOT reveal the block.

#### Scenario: Block closes every conversation
- **WHEN** A blocks B from one conversation
- **THEN** every conversation between A and B is closed and neither can send into any of them

#### Scenario: Block stops new conversations of any kind
- **WHEN** B tries to start a conversation of any kind with A after either has blocked the other
- **THEN** the system responds 403 without saying that a block exists

### Requirement: Reports to moderators
A participant SHALL be able to report a conversation once, with a reason from `spam`, `harassment`, `scam`, `other` and an optional note of at most 1000 characters. Moderators SHALL list open reports with the conversation's kind and messages, and resolve them. Accounts without the moderator role SHALL be refused.

#### Scenario: Report reaches moderation
- **WHEN** A reports a conversation
- **THEN** a moderator listing conversation reports sees the report, its kind, its reason and the conversation's messages

#### Scenario: Non-moderator cannot list reports
- **WHEN** an account without the moderator role lists or resolves conversation reports
- **THEN** the system responds 403

### Requirement: Unread notices
A worker SHALL notify a participant about a conversation that holds a message from another participant that has stayed unread for at least 5 minutes, at most once until the participant reads it, over the participant's enabled channels. A pending conversation SHALL be notified the same way. A declined or closed conversation SHALL NOT be notified. The notice SHALL carry neither the message body nor the sender's account details; its wording and link SHALL come from the conversation's kind. A run SHALL claim a notice before sending it and release the claim when every channel fails.

#### Scenario: One notice for many messages
- **WHEN** B has three unread messages in one conversation, the oldest older than 5 minutes, and the worker runs twice
- **THEN** B receives exactly one notice for that conversation

#### Scenario: Recent messages wait
- **WHEN** B's only unread message is 2 minutes old
- **THEN** the worker sends nothing for it on this run

#### Scenario: Notified again after reading and a new message
- **WHEN** B reads the conversation and later a new message stays unread for 5 minutes
- **THEN** the worker notifies B again

#### Scenario: Failed delivery is retried
- **WHEN** every channel fails for a claimed notice
- **THEN** the claim is released and the next run tries again

### Requirement: Account deletion removes messaging data
Deleting an account SHALL delete its participations, every conversation it took part in with all their messages and reports, and every block it is a side of.

#### Scenario: Deleted account leaves nothing behind
- **WHEN** account A is deleted
- **THEN** no conversation, message, report or block involving A remains
