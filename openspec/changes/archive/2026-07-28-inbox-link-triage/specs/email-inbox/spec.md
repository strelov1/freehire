## MODIFIED Requirements

### Requirement: Mark all read respecting active filters

The system SHALL expose an action that marks every unread message matching the
caller's currently active filters (account source, unread, label, link state, and
search) as read, scoped to the caller, and SHALL leave soft-deleted messages
untouched. The action SHALL report how many messages it marked.

A filter that narrows what the caller sees but not what the action touches is
worse than no filter at all: a caller working through the confirmation queue
would mark their whole mailbox read while believing they marked a handful.

#### Scenario: Mark all read under a filter

- **WHEN** the caller invokes mark-all-read while a label filter is active
- **THEN** only unread messages matching that filter become read
- **AND** messages outside the filter remain unchanged

#### Scenario: Scoped to caller

- **WHEN** the caller invokes mark-all-read
- **THEN** only their own messages are affected, never another user's

#### Scenario: Mark all read under a link-state filter

- **WHEN** the caller invokes mark-all-read while the link state is filtered to
  `suggested`
- **THEN** only unread messages carrying a pending suggestion become read
- **AND** the reported count covers only those
- **AND** messages outside that link state remain unread
