## ADDED Requirements

### Requirement: Ingesting externally-fetched mail

The system SHALL accept a batch of messages an authenticated caller's own mail
client fetched, storing them in the caller's inbox under the source `external`.
Ingestion is the caller's responsibility: the system provides no transport and
never connects to the caller's mail server.

- The endpoint SHALL be `POST /api/v1/me/emails` and SHALL accept a list of
  messages, each carrying an external identifier, sender address, sender name,
  subject, received timestamp, and optionally a thread identifier and a text
  and/or HTML body.
- A message SHALL be stored idempotently on `(caller, external, external id)`, so
  re-sending a message the caller already pushed updates it rather than
  duplicating it.
- The external identifier SHALL be required and non-empty, because it is the
  deduplication key; a message lacking one SHALL be rejected.
- The endpoint SHALL report how many messages it inserted and how many it updated,
  so a syncing agent can tell new mail from a re-run.
- A batch SHALL be bounded in size; a batch above the limit SHALL be rejected with
  a client error naming the limit rather than silently truncated.
- Ingested mail SHALL be indistinguishable from other mail to every existing
  reader: the listing, the message body endpoint, linking, and the tracker's
  application detail all treat it the same.

#### Scenario: A batch is ingested

- **WHEN** an authenticated caller posts a batch of messages
- **THEN** each message is stored in their inbox with source `external` and the response reports the inserted count

#### Scenario: Re-pushing the same message does not duplicate it

- **WHEN** the caller posts a message whose external identifier they already pushed
- **THEN** the stored message is updated in place and the response counts it as updated, not inserted

#### Scenario: A message without an external identifier is rejected

- **WHEN** a batch contains a message with an empty external identifier
- **THEN** the request is rejected with a 400 error and no message in the batch is stored

#### Scenario: An oversized batch is rejected

- **WHEN** a caller posts more messages than the batch limit allows
- **THEN** the request is rejected with a 400 error stating the limit

#### Scenario: Ingested mail appears in the ordinary inbox

- **WHEN** the caller lists their inbox after ingesting
- **THEN** the pushed messages appear alongside their Gmail and hosted mail, newest first

### Requirement: External mail is never classified server-side

The system SHALL exclude mail with source `external` from server-side
classification, so a caller who brings their own mail also brings their own
classifier and costs no LLM tokens.

- The sweep that enqueues unclassified mail for the classification worker SHALL
  skip `external` mail, however long it stays unclassified.
- Gmail and hosted mail SHALL continue to be enqueued and classified exactly as
  before.

#### Scenario: Pushed mail is not enqueued

- **WHEN** the pending-classification sweep runs after a caller ingested mail
- **THEN** no `external` message is enqueued for the classification worker

#### Scenario: Hosted and Gmail mail still classify

- **WHEN** the sweep runs with unclassified Gmail or hosted mail present
- **THEN** that mail is enqueued as before

### Requirement: Agent triage of one message

The system SHALL accept an agent-produced verdict for one message in a single
call, recording the classified status, the application link, and the provenance
together, and SHALL advance the linked application's stage by the same rules the
classification worker uses.

- The endpoint SHALL be `POST /api/v1/me/emails/:id/triage` and SHALL accept a
  status signal, an optional application slug, and an optional confidence.
- The status signal MUST be one of the known classification labels; an unknown
  value SHALL be rejected with a client error.
- The write SHALL be one atomic update setting the status signal, the link when a
  slug was given, the link provenance `agent`, the confidence, and the
  classification timestamp — never leaving a message classified but unstamped or
  linked but unclassified.
- When a slug is given, it SHALL resolve to a job the caller can link; an unknown
  slug SHALL be a 404 and SHALL change nothing.
- Triage SHALL clear any pending suggestion on the message, since the agent's
  verdict supersedes it.
- When the message is linked and the signal implies forward progress, the
  application's stage SHALL advance by the existing monotonic-forward rules,
  which never move a settled application backwards into the active pipeline.
- Triage SHALL be scoped to the caller; a message that is not theirs SHALL be a
  404.
- Triage SHALL be repeatable: re-triaging a message overwrites the previous
  verdict rather than failing.

#### Scenario: Triage classifies and links in one call

- **WHEN** an agent triages a message with a status signal and an application slug
- **THEN** the message is stored with that status, linked to that application, marked as agent-linked, and stamped as classified

#### Scenario: Triage without a slug classifies only

- **WHEN** an agent triages a message with a status signal and no slug
- **THEN** the message's status is recorded and its link is left as it was

#### Scenario: An unknown status signal is rejected

- **WHEN** a triage request carries a status signal outside the known vocabulary
- **THEN** the request is rejected with a 400 error and the message is unchanged

#### Scenario: An unknown slug changes nothing

- **WHEN** a triage request names a slug that resolves to no job
- **THEN** the request returns 404 and the message keeps its previous status and link

#### Scenario: A forward signal advances the stage

- **WHEN** an agent triages a message as an interview invitation and links it to an application at an earlier stage
- **THEN** that application advances to the interview stage

#### Scenario: A settled application is not resurrected

- **WHEN** an agent triages a forward signal onto an application already rejected
- **THEN** the application's stage is left unchanged

#### Scenario: Triage is scoped to the caller

- **WHEN** a caller triages a message that belongs to another user
- **THEN** the request returns 404 and nothing is changed

### Requirement: Agent-shaped inbox listing

The inbox listing SHALL offer an agent mode that returns message bodies inline
and a filter for mail awaiting triage, so an agent reads a page of work in one
request without marking anything read.

- The listing SHALL accept a body option that includes each message's readable
  body in the listing response.
- Requesting bodies SHALL NOT mark any message read, unlike opening a single
  message.
- The listing SHALL accept an unclassified filter returning only messages carrying
  no classification stamp — the agent's work queue.
- Both options SHALL compose with the existing source, unread, label, and search
  filters and SHALL be reflected in the total used for pagination.

#### Scenario: Bodies are returned inline

- **WHEN** an agent lists the inbox with the body option
- **THEN** each returned message includes its readable body

#### Scenario: Listing with bodies does not mark mail read

- **WHEN** an agent lists unread mail with the body option and lists it again
- **THEN** the same messages are still unread

#### Scenario: The unclassified filter is the work queue

- **WHEN** an agent lists the inbox filtered to unclassified mail
- **THEN** only messages with no classification stamp are returned, and the total counts only those

#### Scenario: A triaged message leaves the queue

- **WHEN** an agent triages a message and re-requests the unclassified listing
- **THEN** that message is no longer returned

### Requirement: The mail surface accepts an API key

The mail endpoints SHALL authenticate a full-scope API key as well as a session
cookie, so an agent harness can read and triage its owner's mail without a
browser session.

- The inbox listing, the single-message read, mark-all-read, delete and restore,
  the linking actions, ingestion, and triage SHALL all accept a full-scope key.
- A request carrying no credential SHALL remain a 401.
- Key-authenticated access SHALL be scoped to the key's owner exactly as a session
  is; a key SHALL never reach another user's mail.

#### Scenario: An agent lists mail with a key

- **WHEN** a request carrying a full-scope API key lists the inbox
- **THEN** the key owner's mail is returned

#### Scenario: An unauthenticated request is refused

- **WHEN** a request carries neither a session cookie nor an API key
- **THEN** the request is refused with 401

#### Scenario: A key reaches only its owner's mail

- **WHEN** a request carrying a full-scope API key reads a message belonging to another user
- **THEN** the request returns 404
