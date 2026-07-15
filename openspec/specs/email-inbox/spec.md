# email-inbox Specification

## Purpose
TBD - created by archiving change gmail-inbox. Update Purpose after archive.
## Requirements
### Requirement: Subject-grouped inbox listing

The system SHALL expose an inbox endpoint returning the caller's ATS mail grouped
by normalized subject (Re:/Fwd: prefixes stripped and trimmed), each group with a
message count, latest received time, and distinct senders, scoped to the caller,
and SHALL accept an optional search term that filters the groups.

#### Scenario: Mail grouped by normalized subject

- **WHEN** an authenticated user requests their inbox
- **THEN** messages sharing a normalized subject are returned as one group with its count and latest date

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
full body, both scoped to the caller.

#### Scenario: Read a group's messages

- **WHEN** an authenticated user opens a subject group
- **THEN** the response returns that group's messages with from, subject, and received time, newest first

#### Scenario: Read a message body

- **WHEN** the user opens a message
- **THEN** the response returns its full text and HTML bodies, and a message that is not theirs is a 404

### Requirement: Inbox SPA page

The web SPA SHALL present a `/my/inbox` page: a "Connect Gmail" button when the
caller has not connected, and once connected the subject-grouped list with an
expandable group and a sandboxed reading pane for a message body.

#### Scenario: Not connected

- **WHEN** a signed-in user without a Gmail connection opens `/my/inbox`
- **THEN** the page shows a Connect Gmail button

#### Scenario: Connected user reads mail

- **WHEN** a connected user opens `/my/inbox`
- **THEN** they see their ATS mail grouped by subject and can expand a group and read a message body in a sandboxed pane

### Requirement: Manual application linking in the inbox

The inbox SHALL let the caller manually link an email that has no application
link and no active suggestion to one of their tracked applications. Choosing an
application SHALL link the email to it and mark the link as manual.

The picker SHALL list the caller's tracked applications and SHALL let the caller
filter them by text. When the caller tracks no applications, the picker SHALL say
so rather than present an empty menu.

#### Scenario: Linking an unlinked email

- **WHEN** the caller opens the "Link to application" picker on an unlinked email
  and selects one of their tracked applications
- **THEN** the email becomes linked to that application
- **AND** the row shows "Linked to <company>" with an Unlink control

#### Scenario: No tracked applications

- **WHEN** the caller opens the picker but tracks no applications
- **THEN** the picker states that there is nothing to link to instead of showing
  an empty list

### Requirement: Undo an unlink

Immediately after the caller unlinks an email, the inbox SHALL offer an Undo that
re-links the email to the application it was just unlinked from. The Undo SHALL
apply only to the email that was just unlinked and SHALL be discarded when the
caller acts on a different email or establishes a new link.

#### Scenario: Undo restores the previous link

- **WHEN** the caller unlinks an email and then chooses Undo
- **THEN** the email is re-linked to the same application it was unlinked from

#### Scenario: Undo does not leak across emails

- **WHEN** the caller unlinks one email and then selects a different email
- **THEN** the different email does not offer to undo the first email's unlink

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

