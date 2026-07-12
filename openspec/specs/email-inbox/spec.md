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

