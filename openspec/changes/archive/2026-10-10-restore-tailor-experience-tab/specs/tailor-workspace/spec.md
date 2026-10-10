## ADDED Requirements

### Requirement: The left panel offers an Experience tab

The workspace's left panel SHALL offer an `Experience` tab, positioned between `Editor` and
`Templates`, that mounts the candidate's experience bank (`ExperienceBankView`) — the same
component and data `/my/profile` shows, so checking, confirming, or editing what the
assistant knows about the candidate never requires leaving the Tailor workspace.

#### Scenario: Opening the Experience tab shows the bank

- **WHEN** the user selects the `Experience` tab in the left panel on a wide viewport
- **THEN** the panel shows the candidate's experience bank (employments and achievements), identical in content to the bank shown on `/my/profile`

#### Scenario: A pending reset error surfaces on the Experience tab too

- **WHEN** a reset-from-seed attempt triggered elsewhere in the workspace has failed and the user is on the Experience tab
- **THEN** the same reset error message is shown on the Experience tab, not only on the tab that triggered the reset

### Requirement: Editing the experience bank inside Tailor offers to reseed the tailored CV

Mutating the experience bank (add, update, merge, confirm, or remove an achievement or
employment) from the Tailor workspace's Experience tab SHALL offer the candidate a
confirm/decline prompt to rebuild the current tailored CV from its seed, unless a
whole-document reset is already in flight. The offer and the reset it triggers on
acceptance SHALL be the same ones the workspace's History-tab "Reset" control uses — not a
separate code path — so the two stay behaviorally identical by construction.

#### Scenario: A bank edit offers a reseed

- **WHEN** the user adds, updates, or removes an entry in the experience bank from the Experience tab
- **THEN** a confirm prompt asks whether to rebuild the tailored CV from the current seed

#### Scenario: Accepting the offer reseeds the tailored CV

- **WHEN** the user accepts the reseed offer shown after a bank edit
- **THEN** the tailored CV is rebuilt from the current seed, the same way the History tab's "Reset" control rebuilds it

#### Scenario: The offer is skipped while a reset is already running

- **WHEN** a bank edit happens while a whole-document reset triggered elsewhere in the workspace is already in flight
- **THEN** no reseed offer is shown for that edit

## MODIFIED Requirements

### Requirement: The workspace collapses to a single tabbed view on mobile

The workspace SHALL, below the `lg` breakpoint, collapse its three columns into
a single full-screen view selected by one flat, horizontally-scrollable tab bar
offering every view: Chat, Editor, Experience, Settings, Preview, Templates, Job, Job
Match, and Score. Selecting a tab SHALL show that view full-width and hide the others.
At `lg` and up the workspace SHALL render all three columns side by side as
before, and the flat mobile tab bar SHALL NOT be shown. The per-column tab bars
(Editor/Chat/Experience/Settings, Templates/Job/Job Match/Score) SHALL be desktop-only so
mobile navigation is not duplicated.

#### Scenario: The flat tab bar switches views on mobile

- **WHEN** the workspace renders on a narrow (below `lg`) viewport and the user taps a tab (e.g. Preview or Job Match)
- **THEN** that single view fills the screen and the other views are hidden, with the tab bar offering Chat, Editor, Experience, Settings, Preview, Templates, Job, Job Match, and Score

#### Scenario: Mobile selection stays consistent with the columns

- **WHEN** the user taps a mobile tab that corresponds to a column sub-tab (Editor, Chat, Experience, Settings, Templates, Job, Job Match, or Score)
- **THEN** the matching column's own tab is selected too, so switching to a wide viewport shows the same content selected

#### Scenario: The desktop layout is unchanged at lg

- **WHEN** the workspace renders at `lg` or wider
- **THEN** the three columns show side by side with their own tab bars and splitters, and the flat mobile tab bar is not shown
