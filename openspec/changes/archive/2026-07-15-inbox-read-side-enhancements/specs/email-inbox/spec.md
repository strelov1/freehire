## ADDED Requirements

### Requirement: Unread-only inbox filter

The inbox listing SHALL accept an optional unread-only filter that restricts the
returned messages to those the caller has not yet read. The filter SHALL compose
with the existing account-source and search filters and SHALL be reflected in the
message count used for pagination.

#### Scenario: Unread filter narrows the listing

- **WHEN** an authenticated user requests the inbox with the unread-only filter on
- **THEN** only messages with no read timestamp are returned, and the total count reflects only those

#### Scenario: Unread filter composes with source and search

- **WHEN** the unread-only filter is combined with an account source and a search term
- **THEN** the listing returns only unread messages of that source matching the search

### Requirement: Label filter by classified status

The inbox listing SHALL accept an optional label filter that restricts the
returned messages to a single classified status signal. The filter value MUST be
one of the known classification labels; an unknown value SHALL be rejected with a
client error rather than silently returning nothing.

#### Scenario: Label filter narrows to one status

- **WHEN** an authenticated user requests the inbox filtered to a valid status label
- **THEN** only messages classified with that status are returned

#### Scenario: Unknown label is rejected

- **WHEN** the inbox is requested with a label value that is not a known classification
- **THEN** the request is rejected with a 400 error

### Requirement: Mark all read respecting active filters

The system SHALL expose an action that marks every unread message matching the
caller's currently active filters (account source, unread, label, and search) as
read, scoped to the caller, and SHALL leave soft-deleted messages untouched. The
action SHALL report how many messages it marked.

#### Scenario: Mark all read under a filter

- **WHEN** the caller invokes mark-all-read while a label filter is active
- **THEN** only unread messages matching that filter become read
- **AND** messages outside the filter remain unchanged

#### Scenario: Scoped to caller

- **WHEN** the caller invokes mark-all-read
- **THEN** only their own messages are affected, never another user's

### Requirement: Soft-delete a message with Undo

The inbox SHALL let the caller delete a message, which soft-deletes it: the
message is hidden from the listing and its count but is retained and can be
restored. Immediately after a delete the inbox SHALL offer an Undo that restores
the message. Delete and restore SHALL be scoped to the caller, and a message that
is not the caller's SHALL be a 404. A soft-deleted message SHALL remain deleted
across a re-sync of its source.

#### Scenario: Deleting hides the message

- **WHEN** the caller deletes a message
- **THEN** the message no longer appears in the listing and is excluded from the count

#### Scenario: Undo restores the message

- **WHEN** the caller deletes a message and then chooses Undo
- **THEN** the message is restored and appears in the listing again

#### Scenario: Delete is scoped to the caller

- **WHEN** a user attempts to delete or restore a message that is not theirs
- **THEN** the request returns a 404 and no message is changed

#### Scenario: Soft-delete survives re-sync

- **WHEN** a soft-deleted Gmail message is re-encountered during a later sync
- **THEN** it stays deleted rather than reappearing in the listing
