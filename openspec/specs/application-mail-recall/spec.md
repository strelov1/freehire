# application-mail-recall Specification

## Purpose

Mail-to-application linking runs one way: a message arrives and the system works out which
application it belongs to. This capability is the other direction — from an application, find
the caller's mail that belongs to it.

It exists because the inbound path has no way to be re-asked. When its deterministic signals
miss and its suggestion goes unconfirmed, the message sits unattached and the application
looks untouched, with nothing a person can press. The calendar inherits the same gap and pays
more for it: a meeting attaches to an application only through the identifier of an invitation
already linked, so an invitation that never linked means an interview nobody can see.

The capability PROPOSES and never links. That is not caution but arithmetic: a model reads
attacker-controlled text, a wrong link rewrites a company's public reply history and is
noticed months later, and a wrong proposal costs one press to dismiss. Everything it produces
is resolved through the confirmation surfaces the inbox already has.

## Requirements

### Requirement: Recalling an application's mail

The system SHALL let an authenticated caller ask, from one of their own recorded
applications, for the mail in their mailbox that belongs to it. Authentication MAY be
by session cookie or by full-scope API key.

The action SHALL gather candidates, adjudicate them in a single model call, and report the
confident ones as proposals. It SHALL report how many messages it examined, which ones it
proposes, and how many of those carry a calendar invitation identifier.

Whether a proposal is also RECORDED depends on where the candidate came from — see the two
requirements below. Nothing is linked either way.

An application the caller does not own, one that does not exist, and a tracking row
that was never applied to SHALL all be answered as not found — a row with no
recorded application date is not an application and has no mail to find.

#### Scenario: Mail is found and proposed

- **WHEN** an authenticated caller invokes the action on their own recorded application
- **THEN** the response carries the number of messages examined and the messages
  proposed for that application

#### Scenario: The application is not the caller's

- **WHEN** a caller invokes the action on an application recorded by someone else
- **THEN** the response is not found

#### Scenario: The row was never applied to

- **WHEN** a caller invokes the action on a job they track but never applied to
- **THEN** the response is not found

### Requirement: The action proposes and never links

The action SHALL NOT attach a message to an application, SHALL NOT advance an
application's stage, and SHALL NOT write to the application event ledger. Its most it may
persist is a pending suggestion — and on the search path not even that — which the caller
resolves through the existing confirm and reject actions.

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

### Requirement: A run is bounded and its output is verified against its input

The amount of each body handed to the model SHALL be capped, and any message the model
names that was not in the candidate set SHALL be discarded. A run whose candidate set is
empty SHALL NOT call the model at all.

The candidate count needs no cap of its own on the search path: the search returns what
names the employer, which is a small set, rather than everything that arrived in a window.

#### Scenario: An answer outside the candidate set is discarded

- **WHEN** the model names a message that was not among the candidates
- **THEN** that message is not proposed and nothing about it is written

#### Scenario: An empty candidate set costs nothing

- **WHEN** the candidate set is empty
- **THEN** no model call is made
- **AND** the response reports nothing examined and nothing proposed

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

### Requirement: The candidate set comes from a gated mailbox search

The candidate set SHALL be gathered by searching the caller's connected mailbox for the
employer, inside a time window around the application's recorded date, rather than by
reading mail already stored. A caller with no connected mailbox that can be searched SHALL
fall back to the stored-mail path.

The search SHALL be gated so that scoping by an employer's name cannot reach the caller's
personal correspondence: a message qualifies only if it names the employer AND is
job-shaped. Job-shaped SHALL mean any of hiring vocabulary, a calendar-invitation
attachment, or the application's role title. The invitation attachment and the role title
are both required members of that set: a gate of hiring vocabulary alone was measured to
drop both calendar invitations for an interview and a live recruiter thread whose only
subject was the role.

The mailbox search reads message bodies, which is why it succeeds where a query over stored
metadata could not: matched against sender name and subject alone the employer's name is
absent from the median application's mail entirely.

#### Scenario: Mail is found by searching the mailbox

- **WHEN** an authenticated caller invokes the action on their own recorded application
- **THEN** the candidates are the messages the mailbox search returned for that employer
- **AND** no candidate is taken from the stored mail table

#### Scenario: A calendar invitation is not gated out

- **WHEN** the mailbox holds an invitation for an interview with that employer whose
  subject uses no hiring vocabulary
- **THEN** it is still a candidate, because it carries a calendar attachment

#### Scenario: Mail naming only the role is not gated out

- **WHEN** the mailbox holds a message from that employer whose subject is the role title
  and which never says "application" or "interview"
- **THEN** it is still a candidate

#### Scenario: Personal correspondence is not reached

- **WHEN** the mailbox holds a personal message that merely mentions the employer's name
  and is not job-shaped
- **THEN** it is not a candidate

#### Scenario: A caller with no searchable mailbox falls back

- **WHEN** the caller has no connected mailbox the action can search
- **THEN** the candidates come from the stored-mail path instead
- **AND** the response is otherwise the same shape

### Requirement: A proposal is an unstored message until the caller links it

A proposed message SHALL NOT be written to the caller's stored mail as a side effect of the
sweep. The action SHALL report proposals from the search result itself, and a message SHALL
be stored only when the caller links it, at which point it is imported and then linked.

This is what keeps the sweep from planting state nobody asked for: what a person has not
confirmed is not kept.

#### Scenario: A sweep stores nothing

- **WHEN** the action proposes messages
- **THEN** no new stored mail and no pending suggestion is created

#### Scenario: Linking imports first

- **WHEN** the caller links a proposed message that is not yet stored
- **THEN** the message is imported into their stored mail
- **AND** it is then linked to the application exactly as a stored message would be

#### Scenario: Linking a message already stored does not duplicate it

- **WHEN** the caller links a proposed message that the mail sync had already stored
- **THEN** the existing message is linked rather than a second copy created

### Requirement: A failed search is reported, not disguised

When the mailbox cannot be searched, or the model cannot be reached, or its answer cannot
be read, the action SHALL fail with an error. It SHALL NOT answer as though it examined the
mailbox and found nothing.

The caller pressed a button and is waiting for it; an empty success is indistinguishable
from a mailbox with nothing in it.

#### Scenario: The mailbox search fails

- **WHEN** the mailbox search returns an error
- **THEN** the action responds with an error rather than an empty result

#### Scenario: The model is unreachable

- **WHEN** the model call fails
- **THEN** the action responds with an error
- **AND** nothing is imported and nothing is linked

