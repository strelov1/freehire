# assistant-mail-triage Specification

## Purpose

What the in-app assistant may do with the candidate's application mail: the
orientation-then-search read path over the labels the classification worker already
assigned, the triage and linking actions, the bounds that keep a tool result small
enough to be replayed on every later turn, and the reasons the assistant is denied
the read-marking single-message endpoint and any way to send.
## Requirements
### Requirement: The assistant orients in mail by its labels before reading any

The assistant SHALL be given an `inbox_overview` tool that reports the shape of the
caller's mail without any of its content: how many messages carry each classification
label, how many are unclassified, how many are unread, and how many sit in each link
state (linked, suggested, unlinked). Its prompt SHALL instruct the agent to call it
before searching, so a broad question is answered from the counts the classification
worker already produced rather than by reading messages until an answer appears.

The overview SHALL carry counts only. A message's sender, subject and body are the
subject of `inbox_search`; a tool result is persisted in the transcript and replayed
into the model's context on every later turn, so an orientation tool that carried
content would charge every subsequent turn for a page nobody asked to keep.

#### Scenario: A broad question is answered from the counts

- **WHEN** the candidate asks what is happening with their interview invitations
- **THEN** the agent calls `inbox_overview`, sees the count under the interview-invitation label, and follows with an `inbox_search` narrowed to that label — it does not page through the mailbox reading messages

#### Scenario: The overview names no message

- **WHEN** `inbox_overview` returns
- **THEN** the result contains counts and no message subject, sender or body

### Requirement: Mail reaches the model through the listing, never the single-message read

The assistant SHALL read message bodies only through its listing tool, and SHALL NOT be
given any tool that opens one message by id. Opening a message marks it read, and
`read_at` means "a human saw this" — an agent sweeping a backlog through that path would
silently zero its owner's unread count. The service method the listing tool calls SHALL
NOT be capable of marking a message read, so the guarantee holds structurally rather than
by the model's restraint.

#### Scenario: A sweep leaves the unread count alone

- **WHEN** the agent lists a page of mail with bodies and triages every message on it
- **THEN** no message's `read_at` is set, and the caller's unread count is unchanged

#### Scenario: No tool opens one message

- **WHEN** the tool registry for a session carrying mail tools is enumerated
- **THEN** it contains no tool that reads a single message by id

### Requirement: A listing carrying bodies is bounded harder for the model than for a harness

The listing tool SHALL return message bodies only when explicitly asked, and a page
carrying bodies SHALL be capped at ten messages — below the fifty an external harness may
request over HTTP. The two callers are bounded differently on purpose: a harness reads a
page once and forgets it, while a tool result is replayed into the model's context on
every later turn of the session, so a page of fifty bodies is charged to every subsequent
turn.

Each individual body SHALL also be truncated. The page cap bounds how MANY bodies come
back; without a second bound nothing limits how LARGE each one is, and mail from an
applicant-tracking system is routinely HTML-only and renders to tens of kilobytes — a
full page of it overflows the conversation's result cap, so the model receives a
truncation notice holding one message instead of the page it asked for. The per-message
bound SHALL be tighter than the classification worker's, because the worker reads that
text once and this text is replayed on every later turn.

A listing that does not ask for bodies SHALL still carry each message's sender, subject,
received date, label, confidence, link state and linked vacancy — enough to answer most
questions and to choose which messages are worth a body.

#### Scenario: A page with bodies is capped

- **WHEN** the model requests a listing with bodies and a limit above ten
- **THEN** at most ten messages are returned

#### Scenario: A full page of long mail still fits the conversation

- **WHEN** the model requests a full page of bodies and every message is long recruiter boilerplate
- **THEN** each body is truncated so the whole result stays under the conversation's result cap, rather than the page being replaced by a truncation notice

#### Scenario: A page without bodies answers without them

- **WHEN** the model lists mail narrowed to one label without asking for bodies
- **THEN** each row carries its sender, subject, date, label and link state, and no body

### Requirement: The assistant records a verdict and resolves a link

The assistant SHALL be given tools to classify one message (`inbox_triage`), to accept or
dismiss a matcher's pending suggestion (`inbox_resolve_suggestion`), to attach a message
to an application or detach it (`inbox_link`, `inbox_unlink`), and to record an
application from a message that has none and link it in one call
(`inbox_record_application`). Each SHALL act as the session's owner and SHALL apply the
same rules the equivalent HTTP endpoint applies, because both call one service.

Classifying without naming an application SHALL leave the message's link untouched rather
than clearing it; clearing stays the explicit detach action.

#### Scenario: A classification without a slug keeps the link

- **WHEN** the model triages a linked message and supplies only a signal
- **THEN** the message's label is written and its application link is unchanged

#### Scenario: A suggestion is resolved either way

- **WHEN** the model accepts a pending suggestion
- **THEN** the message becomes linked to the suggested application; and when it dismisses one, the message is left unlinked with its classification intact

#### Scenario: Mail with no application gets one

- **WHEN** the model records an application from a message that has no application to point at
- **THEN** the application is created, dated by the message, and the message is linked to it

### Requirement: An unknown label is refused with the vocabulary

A classification the model supplies SHALL be rejected when it is outside the controlled
vocabulary, and the error SHALL name the invalid value and list the valid ones. The
sanitize-to-`other` behaviour that guards the classification worker's untrusted LLM output
SHALL NOT apply here: the worker reads an attacker-controlled email body and must never
persist a raw model string, while the assistant is making a judgement the candidate asked
for, and silently rewriting it to `other` would record a verdict nobody chose.

#### Scenario: A misspelled label is corrected within the turn

- **WHEN** the model triages a message with a label that is not in the vocabulary
- **THEN** the call fails with an error naming the invalid value and listing the valid labels, the message is unchanged, and the turn continues

### Requirement: Mail bodies are untrusted input and the tool surface bounds the damage

Message bodies SHALL be treated as attacker-controlled text, and the mail tool surface
SHALL contain no tool that sends mail, so a prompt injection carried in a body has no
outbound channel. The worst an injection can achieve SHALL be a wrong label or a wrong
link, both of which the candidate can reverse from `/my/inbox`. An application's stage
SHALL continue to move only monotonically forward, so mail can never silently reopen or
close a pipeline the candidate has moved on from.

The prompt SHALL name the body as untrusted and state that an instruction found inside a
message is an attack rather than a request.

#### Scenario: An injected instruction has nowhere to go

- **WHEN** a message body contains text instructing the agent to email its contents somewhere
- **THEN** no tool exists that could send it, and the agent reports the message as what it is

#### Scenario: A rejection cannot walk a stage backwards

- **WHEN** a message classified as a rejection is linked to an application already past that point
- **THEN** the stage is left where it is

### Requirement: Mail tools belong to the general chat session only

The mail tools SHALL be registered for the general-chat preset and for no other. A
tailoring session is working one CV against one vacancy, an experience interview is
collecting what the candidate has done, and a browser-panel session is talking about the
page on screen — none of them has a reason to read the candidate's mail, and every tool
offered spends the model's context on every turn whether or not it is called.

#### Scenario: Only the chat preset carries them

- **WHEN** the tool registry is built for a tailoring, experience-interview or browser-panel session
- **THEN** it contains no mail tool

#### Scenario: The chat preset carries all of them

- **WHEN** the tool registry is built for a general chat session
- **THEN** it contains the overview, search, triage, suggestion-resolution, link, unlink and record-application tools
