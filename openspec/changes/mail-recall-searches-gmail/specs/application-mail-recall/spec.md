## MODIFIED Requirements

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

## REMOVED Requirements

### Requirement: Only unattached mail may be proposed

**Reason**: Its subject no longer exists. That requirement governed which STORED rows the
net was allowed to read and write, and on the search path there are no stored rows to
select from and no suggestion written. What it protected — that a message already linked to
an application cannot be modified — is now structural rather than enforced: the sweep
writes nothing at all, and the import-then-link path attaches a message the caller
explicitly chose.

**Migration**: The stored-mail fallback keeps the predicate in its query, so the guarantee
survives for the path that still reads rows. No data changes.

### Requirement: The candidate set is bounded by state and time, not by words

**Reason**: Replaced by "The candidate set comes from a gated mailbox search". The
prohibition on searching for the employer's name was correct about STORED metadata — the
name is absent from the median application's sender and subject — and wrong about the
mailbox, where a body search finds mail for 14 applications in 15. The reasoning it carried
about HTML-only bodies moves with it: the search reads bodies by nature.

**Migration**: None. The stored-mail fallback retains its window and its readable-body
handling.
