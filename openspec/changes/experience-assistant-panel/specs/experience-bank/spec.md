## MODIFIED Requirements

### Requirement: The bank names a way into the interviewer in every state

The experience view SHALL offer a labelled action that opens the `profile` interviewer,
and SHALL offer it whether the bank holds entries or none — the empty bank is the one that
most needs filling. The action SHALL be named for what the candidate gets rather than for
the machine that produces it, and SHALL be accompanied by a concrete example of an answer,
so the expected grain of a reply (one result, ideally carrying a number) is visible before
the conversation starts. The action SHALL open the interviewer **in place**, beside the
bank, rather than navigating away from the experience view: the bank the candidate was
reading is what the conversation is about, and losing it to reach the agent is the defect
this replaces. The addressable interviewer SHALL remain reachable by URL for a bookmark or
a shared link, and the opening message SHALL be identical whichever way it is entered.

#### Scenario: A populated bank offers the action

- **WHEN** a signed-in user opens the experience view with achievements on record
- **THEN** an action opening the interviewer is shown alongside the count, with an example
  of the kind of achievement to describe

#### Scenario: An empty bank offers the same action

- **WHEN** a signed-in user opens the experience view with nothing recorded
- **THEN** the explanation of what the bank is for is shown together with the same action
  and example, rather than text alone

#### Scenario: The action opens the conversation without leaving the bank

- **WHEN** the candidate follows the experience view's action into the interviewer
- **THEN** the conversation opens on the same page with the bank still shown, and no
  navigation away from the experience view occurs

#### Scenario: The addressable interviewer still works

- **WHEN** the candidate opens the interviewer's own URL directly, with or without atom
  ids in the address
- **THEN** the conversation opens on its own page as before, with the same opening message
  the in-place panel would have sent

## ADDED Requirements

### Requirement: The in-place interviewer leaves the bank live and in sync

While the interviewer is open in place, the bank SHALL remain visible and interactive on a
wide viewport: the candidate SHALL be able to scroll it, change the selection, and edit,
merge or remove an achievement without closing the conversation. The panel SHALL NOT trap
focus or make the bank inert on a wide viewport, because the conversation exists to be had
*about* the rows behind it. On a viewport too narrow to show both, the panel MAY cover the
bank, and SHALL then offer an explicit way back to it.

When the conversation writes to the bank — recording, correcting or merging an achievement
— the bank behind it SHALL reflect that write without the candidate reloading the page. A
list that disagrees with the transcript beside it is worse than no list, since the
candidate cannot tell which one is true.

Closing the panel SHALL leave the bank exactly as it stands, including the current
selection, and SHALL NOT undo anything the conversation did.

#### Scenario: The bank stays usable beside the conversation

- **WHEN** the interviewer is open in place on a wide viewport
- **THEN** the candidate can scroll the bank, select a different achievement, and edit or
  remove one, without the panel closing or blocking the interaction

#### Scenario: A merge made in conversation shows up in the list

- **WHEN** the agent merges two achievements during a turn in the in-place panel
- **THEN** the bank behind the panel refreshes to show the single kept achievement, with
  the other gone, without a page reload

#### Scenario: A narrow viewport covers the bank but offers the way back

- **WHEN** the interviewer is opened in place on a viewport too narrow for two columns
- **THEN** the conversation covers the bank and a visible control returns to it

#### Scenario: Closing keeps what the conversation did

- **WHEN** the candidate closes the panel after the agent has changed the bank
- **THEN** the bank remains on screen carrying those changes, and the selection the
  candidate had made is still in place

### Requirement: A selected set opens the interviewer on those achievements in place

When the candidate has selected achievements and follows the tailor action, the interviewer
SHALL open in place and its opening message SHALL name exactly those achievements, so the
agent's first question is about reconciling or enriching that selection. The message SHALL
be built from the same rule as the addressable entry, so the two entries cannot drift.
Identifiers that are not the candidate's own achievements SHALL be ignored rather than
carried into the conversation.

#### Scenario: The selection becomes the conversation's subject

- **WHEN** the candidate selects two near-duplicate achievements and follows the tailor
  action
- **THEN** the panel opens a `profile` conversation whose opening message names those two
  achievements, and the agent's first reply is about them rather than an unrelated gap

#### Scenario: Both entries ask the same thing

- **WHEN** the same set of achievements is opened once through the in-place action and once
  through the interviewer's own address
- **THEN** the opening message is the same in both conversations

### Requirement: The selection actions stay reachable while the bank scrolls

The experience view's selection actions SHALL remain visible while the bank is scrolled, so
a pair of near-duplicates found far down a long list can be merged where it was found. The
actions SHALL be positioned clear of any surrounding fixed page furniture rather than
underneath it — an action pinned behind the site header is not reachable, and is
indistinguishable to the candidate from one that was never offered.

#### Scenario: The actions survive a long scroll

- **WHEN** the candidate selects two achievements and scrolls far down a bank long enough
  to leave the top of the list
- **THEN** the merge and tailor actions remain fully visible and clickable, not hidden
  behind the page header

#### Scenario: No selection shows no bar

- **WHEN** nothing is selected
- **THEN** no selection action bar is shown and the bank occupies the full column
