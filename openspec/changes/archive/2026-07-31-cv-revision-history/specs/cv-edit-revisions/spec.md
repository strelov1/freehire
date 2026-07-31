## ADDED Requirements

### Requirement: Every change to a stored CV is a recorded revision

The system SHALL record every change to a stored CV as a revision carrying the operations that
made it, the inverse operations that would undo it, the actor who made it, and the entry point it
came through. There MUST be exactly one code path that writes a CV's stored state, and it MUST
write a revision in the same transaction as the state it changes — a change that leaves no
revision, or a revision without its change, would make the feed lie about the document.

The actor MUST be decided by the entry point and MUST NOT be read from the request body: a caller
naming itself is not evidence of who it is.

#### Scenario: A candidate's edit is recorded

- **WHEN** the candidate's editor saves a changed document
- **THEN** a revision is stored naming the candidate as actor, with the operations that changed the document and their inverses

#### Scenario: An agent's edit is recorded

- **WHEN** the tailoring agent commits an edit
- **THEN** a revision is stored naming the agent as actor, carrying the batch of the turn it belongs to

#### Scenario: A rejected change writes nothing

- **WHEN** a commit is refused (an unknown path, a policy denial, a missing evidence citation)
- **THEN** neither the document nor the revision feed changes

### Requirement: Edits are expressed as operations over typed paths

An edit SHALL be expressed as one or more operations of kind `set`, `insert`, `remove` or `move`,
each addressing a typed path into the CV's editable state — its title, its template, and the
fields of its document. A path MUST be validated against the state's own structure, so that no
path can exist in an operation that does not exist in the document.

Applying operations MUST be all-or-nothing: if any operation in a batch fails, the stored state
MUST be left exactly as it was, so that a rejected batch is never a partial edit.

#### Scenario: An operation addresses a nested field

- **WHEN** an operation sets `experience[3].bullets[1]`
- **THEN** that bullet changes and no other part of the document does

#### Scenario: An unknown path is refused

- **WHEN** an operation addresses a field the document does not define, or an index beyond the end of a list
- **THEN** the commit is refused as a client error and the document is unchanged

#### Scenario: One bad operation rejects the whole batch

- **WHEN** a batch of four operations has an invalid third operation
- **THEN** none of the four is applied and the document is unchanged

### Requirement: A revision can be undone on its own

The system SHALL offer, for each revision, an undo that applies that revision's inverse
operations to the CV's current state. Undoing MUST NOT discard changes made after the revision
being undone: only what that revision did is reversed.

An undo MUST itself be recorded as a revision that names the revision it reverses, and the
reversed revision MUST be marked as undone. The log is never rewritten — a feed that erases its
own entries cannot be trusted to describe the document.

When a revision's inverse can no longer be applied — the place it would restore is gone — the
request MUST be refused with that reason, and the document MUST be unchanged.

#### Scenario: Undoing an older edit keeps the newer ones

- **WHEN** three edits are made and the first is undone
- **THEN** the first edit's change is reversed and the second and third remain in the document

#### Scenario: An undo is itself a revision

- **WHEN** a revision is undone
- **THEN** a new revision is recorded naming the reversed one, and the reversed one is marked as undone

#### Scenario: An undo can be undone

- **WHEN** an undo revision is itself undone
- **THEN** the original edit is back in the document

#### Scenario: An inapplicable undo is refused with its reason

- **WHEN** undoing an edit whose target has since been deleted
- **THEN** the request is refused with a message naming that the place it changed no longer exists, and the document is unchanged

### Requirement: Consecutive edits to the same place are coalesced

The system SHALL amend the newest revision rather than record a new one when an incoming change
has the same actor and entry point, touches exactly the same paths, and arrives within a short
window of it. Amending MUST replace the stored operations with the new ones and MUST leave the
stored inverse operations untouched, so that undoing still returns to the state before the first
of the coalesced edits.

Without this, autosave would file one revision per keystroke burst and undo would step back one
debounce interval at a time.

#### Scenario: Typing a sentence files one revision

- **WHEN** the candidate types into one bullet and autosave fires several times in quick succession
- **THEN** the feed holds one revision for that bullet

#### Scenario: Undoing a coalesced revision returns to the start

- **WHEN** a coalesced revision is undone
- **THEN** the bullet holds the text it had before the first of those saves

#### Scenario: A different place starts a new revision

- **WHEN** the candidate edits one bullet and then a different one
- **THEN** the feed holds two revisions

### Requirement: The revision feed is bounded

The revision feed SHALL be capped at a fixed number of most-recent revisions per CV, trimmed in
the same statement that records a new one. A revision log is an aid to the candidate's current
work, not an archive, and each row carries two operation documents on the table read on every CV
page.

#### Scenario: The oldest revisions fall off

- **WHEN** a CV accumulates more revisions than the cap
- **THEN** the oldest are removed and the newest are retained
