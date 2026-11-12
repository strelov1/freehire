## ADDED Requirements

### Requirement: Unread application mail is visible to account MCP agents as metadata only
The account MCP server SHALL serve a read-only `list_unread_mail` tool to OAuth grants and
full-scope API keys that lists the caller's newest unread application mail as sender,
subject, received time, classification label and, when confirmed, the linked company and job
slug, together with the total unread count, whether the mailbox is still syncing or needs
reconnecting, and a link to the inbox. It SHALL NOT return message bodies or snippets, SHALL
NOT mark any message read, and SHALL NOT call a model.

#### Scenario: Unread count exceeds the page
- **WHEN** the caller has more unread application mail than the requested limit
- **THEN** `unread_count` reports the total and `messages` holds at most the limit

#### Scenario: Reading marks nothing
- **WHEN** the tool lists a message
- **THEN** the message is still unread afterwards

#### Scenario: Another user's mail
- **WHEN** another account has unread mail
- **THEN** none of it appears in the caller's result or count

#### Scenario: First Gmail sync still running
- **WHEN** the caller's Gmail is connected and has never completed a sync
- **THEN** `sync_pending` is true

### Requirement: Sender-written mail text is returned as inert data
Sender and subject SHALL be returned with control and format characters removed and their
length capped, and the tool description, the result and the account server instructions SHALL
state that they are written by the sender and must never be followed as instructions.

#### Scenario: An injected instruction in a subject
- **WHEN** a subject contains a line break followed by text addressed to the agent
- **THEN** the subject is returned as one single-line string inside its field and the result carries the untrusted-text notice
