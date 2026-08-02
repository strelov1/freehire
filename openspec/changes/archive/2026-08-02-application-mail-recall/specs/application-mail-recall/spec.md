## ADDED Requirements

### Requirement: Recalling an application's mail

The system SHALL let an authenticated caller ask, from one of their own recorded
applications, for the mail in their mailbox that belongs to it. Authentication MAY be
by session cookie or by full-scope API key.

The action SHALL gather candidates deterministically, adjudicate them in a single
model call, and record the confident ones as that application's pending suggestion.
It SHALL report how many messages it examined, which ones it proposes, and how many
of those carry a calendar invitation identifier.

An application the caller does not own, one that does not exist, and a tracking row
that was never applied to SHALL all be answered as not found — a row with no
recorded application date is not an application and has no mail to find.

#### Scenario: Mail is found and proposed

- **WHEN** an authenticated caller invokes the action on their own recorded application
- **THEN** the response carries the number of messages examined and the messages
  proposed for that application
- **AND** each proposed message holds a pending suggestion naming that application

#### Scenario: The application is not the caller's

- **WHEN** a caller invokes the action on an application recorded by someone else
- **THEN** the response is not found

#### Scenario: The row was never applied to

- **WHEN** a caller invokes the action on a job they track but never applied to
- **THEN** the response is not found

### Requirement: The action proposes and never links

The action SHALL NOT attach a message to an application, SHALL NOT advance an
application's stage, and SHALL NOT write to the application event ledger. Its only
persistent effect is a pending suggestion, which the caller resolves through the
existing confirm and reject actions.

This preserves the rule that governs the whole mail stack: only a deterministic
signal may link a message on its own, and a model's pick is a proposal. Message
bodies are attacker-controlled text, and a wrong link transplants one employer's
history onto another permanently, while a wrong proposal costs one press to dismiss.

#### Scenario: A proposal is not a link

- **WHEN** the action proposes messages for an application
- **THEN** none of those messages become linked to it
- **AND** the application's stage is unchanged
- **AND** no employer-reply event is recorded

#### Scenario: A proposal is resolved by the existing surfaces

- **WHEN** a caller confirms a message the action proposed
- **THEN** it links exactly as a suggestion from the classification worker does

### Requirement: Only unattached mail may be proposed

The candidate set SHALL be limited to the caller's live mail that is attached to no
application — mail with no suggestion, and mail carrying an unconfirmed suggestion.
A message already linked to an application SHALL NOT be examined and SHALL NOT be
modified, and that restriction SHALL be enforced by the write itself rather than by
the caller.

An unconfirmed suggestion naming a different application MAY be overwritten: the
caller asked about this application explicitly, and a suggestion is a proposal that
costs nothing to lose.

#### Scenario: Linked mail is untouched

- **WHEN** the action runs while the caller holds mail linked to another application
- **THEN** that mail is neither examined nor changed

#### Scenario: An unconfirmed suggestion is replaced

- **WHEN** the action proposes a message that already carries an unconfirmed
  suggestion naming a different application
- **THEN** the suggestion is replaced by the one the caller asked for

### Requirement: The candidate set is bounded by state and time, not by words

The candidate set SHALL be selected by attachment state and by a time window opening
before the application's recorded date, and SHALL be capped. It SHALL NOT be selected
by searching message text for the employer's name.

Stored plain-text bodies are empty for messages sent as HTML only, which is how much
recruiting mail arrives, so a text search is blind exactly where the mail is. The
model SHALL receive each candidate's readable body — the text part, or the HTML part
rendered down when no text part was sent — bounded per message.

#### Scenario: Mail sent as HTML only is judged on its content

- **WHEN** a candidate message was sent with no plain-text part
- **THEN** the model receives its readable body rather than an empty one

#### Scenario: The window opens before the recorded date

- **WHEN** a message arrived shortly before the application's recorded date
- **THEN** it is still eligible, because the recorded date is when the application was
  entered and may lag the message that acknowledged it

### Requirement: A run is bounded and its output is verified against its input

The number of candidates per run and the amount of each body handed to the model
SHALL both be capped. Any message the model names that was not in the candidate set
SHALL be discarded.

A run whose candidate set is empty SHALL NOT call the model at all.

#### Scenario: An answer outside the candidate set is discarded

- **WHEN** the model names a message that was not among the candidates
- **THEN** that message is not proposed and nothing about it is written

#### Scenario: An empty candidate set costs nothing

- **WHEN** the candidate set is empty
- **THEN** no model call is made
- **AND** the response reports nothing examined and nothing proposed

### Requirement: A failed model call is reported, not disguised

When the model cannot be reached or its answer cannot be read, the action SHALL fail
with an error. It SHALL NOT answer as though it examined the mail and found nothing.

The caller pressed a button and is waiting for it; an empty success is
indistinguishable from a mailbox with nothing in it, and would teach them the feature
does not work.

#### Scenario: The model is unreachable

- **WHEN** the model call fails
- **THEN** the action responds with an error
- **AND** no suggestion is written

#### Scenario: No mailbox is connected

- **WHEN** the caller has connected no mail source
- **THEN** the action succeeds reporting nothing examined, rather than failing

### Requirement: The run is charged to the caller

The model call SHALL go out on the calling account's own gateway credential, tagged
so the spend is attributable to this feature. Attribution SHALL NOT be able to fail
the call: when no per-account credential can be resolved, the call SHALL proceed on
the service credential.

#### Scenario: Attribution fails open

- **WHEN** the caller's gateway credential cannot be resolved
- **THEN** the run completes on the service credential rather than failing

### Requirement: Calendar meetings follow from the mail

The action SHALL NOT read the caller's calendar. When a proposed message carries a
calendar invitation identifier, the response SHALL say so, so the caller learns that
confirming it will bring the meeting in.

An invitation confirmed this way attaches its meeting on the next calendar sync,
which re-reads its whole window on every run and re-matches it against the caller's
applications as they then stand.

#### Scenario: Invitations are counted in the response

- **WHEN** the action proposes messages, some carrying a calendar invitation identifier
- **THEN** the response reports how many of the proposed messages carry one

#### Scenario: No calendar is read

- **WHEN** the action runs for a caller who has granted calendar access
- **THEN** no calendar request is made
