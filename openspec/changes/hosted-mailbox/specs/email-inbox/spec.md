## MODIFIED Requirements

### Requirement: Subject-grouped inbox listing

The system SHALL expose an inbox endpoint returning the caller's mail from **all
sources** (Gmail-synced and hosted-mailbox) grouped by normalized subject (Re:/Fwd:
prefixes stripped and trimmed), each group with a message count, latest received time,
and distinct senders, scoped to the caller, and SHALL accept an optional search term
that filters the groups and an optional **source filter** (a single account) that narrows
the listing to that source. The grouping and search are source-agnostic — a group MAY span
messages from different sources when no source filter is applied.

#### Scenario: Mail grouped by normalized subject

- **WHEN** an authenticated user requests their inbox
- **THEN** messages sharing a normalized subject are returned as one group with its count and latest date, regardless of source

#### Scenario: Both sources appear in one inbox

- **WHEN** a user has both Gmail-synced mail and hosted-mailbox mail and no source filter is applied
- **THEN** both are listed together in the same inbox, not in separate lists

#### Scenario: Filter to one account

- **WHEN** a user requests the inbox with a source filter for one account (Gmail or hosted)
- **THEN** only that account's mail is returned

#### Scenario: Re/Fwd folded into the base group

- **WHEN** the user has "Subject X" and "Re: Subject X"
- **THEN** both fall in the same group

#### Scenario: Search filters the groups

- **WHEN** an authenticated user requests the inbox with a search term
- **THEN** only groups with a message whose subject, sender, or body matches the term are returned

#### Scenario: Scoped to caller

- **WHEN** a user requests the inbox
- **THEN** only their own mail is returned, never another user's

### Requirement: Group thread and message body

The system SHALL expose a group's messages (newest first) and a single message's
full body, both scoped to the caller, and MUST mark a message read when its body is
opened.

#### Scenario: Read a group's messages

- **WHEN** an authenticated user opens a subject group
- **THEN** the response returns that group's messages with from, subject, source, and received time, newest first

#### Scenario: Read a message body

- **WHEN** the user opens a message
- **THEN** the response returns its full text and HTML bodies, and a message that is not theirs is a 404

#### Scenario: Opening a message marks it read

- **WHEN** the user opens an unread message's body
- **THEN** the message is marked read and subsequent listings report it read

### Requirement: Read and unread state

The system SHALL track per-message read/unread state and expose it in inbox
listings so unread mail is distinguishable.

#### Scenario: New mail is unread

- **WHEN** a message is stored by any source and never opened
- **THEN** the inbox reports it unread

#### Scenario: Opened mail is read

- **WHEN** the user has opened a message
- **THEN** the inbox reports it read

### Requirement: Inbox SPA page

The web SPA SHALL present a `/my/inbox` page that offers **both** mail options — a
"Connect Gmail" action and a "Get a freehire mailbox" action — shows the claimed
mailbox address when present, and lists the caller's mail grouped by subject with an
expandable group and a sandboxed reading pane for a message body, distinguishing unread
messages. When both sources are connected the page SHALL present an **account switcher**
(`All` · `Gmail` · `freehire mailbox`) that filters the list to the chosen account.

#### Scenario: Neither source configured

- **WHEN** a signed-in user with no Gmail connection and no mailbox opens `/my/inbox`
- **THEN** the page offers both a Connect Gmail action and a Get-a-mailbox action

#### Scenario: Hosted mailbox shown

- **WHEN** a user has claimed a mailbox
- **THEN** the page shows their `<handle>@<MAIL_DOMAIN>` address

#### Scenario: User reads mail from either source

- **WHEN** a user with mail (from Gmail, the hosted mailbox, or both) opens `/my/inbox`
- **THEN** they see it grouped by subject, can expand a group and read a message body in a sandboxed pane, and unread messages are visually distinct

#### Scenario: Switch accounts

- **WHEN** a user with both sources connected picks an account in the switcher
- **THEN** the list narrows to that account's mail, and picking "All" shows both
