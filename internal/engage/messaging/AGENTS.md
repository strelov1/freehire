# Messaging core

## Scope
Conversations between accounts for every feature that lets people write to each other
(openspec/changes/messaging-core). A conversation carries a `kind`; today the only kind is
`buddy` ([internal/engage/buddy](../buddy/AGENTS.md)). Tables `conversations`,
`conversation_participants`, `messages`, `user_blocks`, `conversation_reports` (migration
0202), queries in `internal/platform/db/queries/messaging.sql`. HTTP is
`internal/api/handler/conversations.go`; the worker is `cmd/message-notify`.

## Always true
- **Every participant speaks as a persona** (`conversation_participants.persona`): one
  account has several — its real self, an anonymous buddy handle, later a mentor or an
  employer. The other side is drawn from the persona it speaks as, never from the
  conversation's kind and never from the account.
- **Personas are never joined.** Nothing a `Persona` returns, and nothing any response
  carries, may identify the same account under another persona: no account id, no email,
  no other handle. `TestBuddyPayloadsNeverNameTheAccount` (internal/api/handler) holds
  every buddy conversation payload to it.
- **The core never imports a feature.** A feature implements `Persona` (how an account
  speaking as it is drawn and named) and `Kind` (the wording and link of its notices);
  personas are registered when the `Service` is built, kinds when the `Notifier` is. Who
  may START a conversation, and its pair key, are the feature's rules: its own service
  checks them, then calls `Start`. The core enforces only what holds everywhere —
  participation, blocks, the pair key, message length.
- **A conversation the caller does not take part in is "not found"**, for every operation,
  exactly as one that does not exist.
- **An unreachable counterpart serves nothing** but its persona name. Whatever a persona
  returns, the core blanks the detail when `Reachable` is false.
- **Blocks are account-wide**, across personas: a block closes every conversation between
  the two accounts, of every kind, and is checked inside the start and answer writes, not
  by an earlier read. A refusal caused by a block never says so.
- **A pair key reopens, never duplicates.** A start on a taken key reopens a conversation
  closed without a block (old messages read for both, the new one unread) and is refused
  in every other status.
- **Two participants per conversation, today.** "The other participant" queries and the
  deletion trigger (deleting either account deletes the conversation, its messages and
  reports) assume it; a multi-party kind must revisit both.
- **Notices claim before they send** and carry no message text and no account detail.
