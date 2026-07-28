## MODIFIED Requirements

### Requirement: Agent-shaped inbox listing

The inbox listing SHALL offer an agent mode that returns message bodies inline
and filters for mail awaiting triage, so an agent reads a page of work in one
request without marking anything read.

- The listing SHALL accept a body option that includes each message's readable
  body in the listing response.
- Requesting bodies SHALL NOT mark any message read, unlike opening a single
  message.
- The listing SHALL accept an unclassified filter returning only messages carrying
  no classification stamp — the agent's work queue.
- The listing SHALL accept a link-state filter with the values `linked`,
  `suggested`, and `unlinked`, returning respectively the messages attached to an
  application, those carrying a pending suggestion the caller has neither
  confirmed nor rejected, and those with neither. The three values partition the
  caller's mail: every message matches exactly one.
- An unknown link-state value SHALL be rejected with a client error rather than
  silently returning nothing, matching the label filter.
- All options SHALL compose with the existing source, unread, label, and search
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

#### Scenario: The suggested filter is the confirmation queue

- **WHEN** an agent lists the inbox filtered to link state `suggested`
- **THEN** only messages carrying a pending suggestion are returned, and the
  total counts only those

#### Scenario: Confirming a suggestion empties it from the queue

- **WHEN** the caller confirms a suggested link and re-requests the `suggested`
  listing
- **THEN** that message is no longer returned, and it is returned by the `linked`
  listing instead

#### Scenario: Rejecting a suggestion moves it to unlinked

- **WHEN** the caller rejects a suggested link and re-requests the listing
- **THEN** that message is returned by the `unlinked` listing and by neither the
  `suggested` nor the `linked` one

#### Scenario: The three link states partition the mailbox

- **WHEN** an agent lists each of `linked`, `suggested`, and `unlinked` for the
  same caller with no other filter
- **THEN** the three totals sum to the caller's unfiltered total, and no message
  appears in two of the listings

#### Scenario: Link state composes with the other filters

- **WHEN** an agent lists the inbox filtered to link state `unlinked` together
  with a classification label
- **THEN** only unlinked messages carrying that label are returned, and the total
  counts only those

#### Scenario: Unknown link state is rejected

- **WHEN** the inbox is requested with a link-state value outside the three
  known ones
- **THEN** the request is rejected with a 400 error
