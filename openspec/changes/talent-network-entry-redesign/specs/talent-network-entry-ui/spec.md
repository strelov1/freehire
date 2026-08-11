## ADDED Requirements

### Requirement: Entry point gated to beta testers
The Talent Network entry button, its overlay panel, and the visibility
fetch that seeds the button's state SHALL be present only for a caller
whose account has `beta_tester` set; a non-beta caller SHALL see none of
them.

#### Scenario: Non-beta caller sees no entry point
- **WHEN** a signed-in caller whose account does not have `beta_tester` set
  views `my/profile`
- **THEN** no Talent Network button, panel, or `GET /me/talent-network`
  request is present

#### Scenario: Beta caller sees the entry point
- **WHEN** a signed-in caller whose account has `beta_tester` set views
  `my/profile`
- **THEN** the status-aware entry button is shown and behaves per the
  requirements below

### Requirement: Status-aware entry button
On `my/profile`, the Talent Network entry point SHALL be a button whose
appearance reflects the caller's current `talent_network_visibility`.

#### Scenario: Visibility off
- **WHEN** the caller's current visibility is `off`
- **THEN** the button renders as a solid, filled call-to-action reading
  "Join Talent Network"

#### Scenario: Visibility public or anonymous
- **WHEN** the caller's current visibility is `public` or `anonymous`
- **THEN** the button renders as an outlined status pill showing the
  current mode's icon and name (e.g. "🌐 Talent Network: Public")

#### Scenario: Button always opens the same panel
- **WHEN** the button is clicked, in any visibility state
- **THEN** the same overlay panel opens

### Requirement: Overlay panel entry point
Clicking the entry button SHALL open an overlay panel on top of the
current page content, rather than navigating to a new route or switching
to a tab in the existing settings tab strip.

#### Scenario: Panel opens over the current tab
- **WHEN** the caller clicks the entry button while any Settings-tab-strip
  tab is active
- **THEN** the panel opens as an overlay and the underlying tab remains
  the active tab underneath it

### Requirement: Public-link card always visible in the panel
The panel SHALL display a card showing the caller's public profile link,
positioned above the visibility mode picker, and this card SHALL be
visible and its link actionable regardless of the caller's current
visibility setting — including `off`.

#### Scenario: Link card visible when off
- **WHEN** the panel is open and the caller's visibility is `off`
- **THEN** the public-link card is shown with the caller's
  `talent_network_public_id`-derived URL, and a "View" action is present

#### Scenario: Link card visible when public or anonymous
- **WHEN** the panel is open and the caller's visibility is `public` or
  `anonymous`
- **THEN** the public-link card is shown identically, unchanged in
  position or content shape from the `off` case

### Requirement: Icon-bearing mode picker
The panel's Off/Public/Anonymous picker SHALL display a distinct icon
alongside each option: 🚫 for Off, 🌐 for Public, 🕶️ for Anonymous.

#### Scenario: All three icons present
- **WHEN** the panel's mode picker renders
- **THEN** each of the three options shows its designated icon next to its
  label and description

### Requirement: Public profile page header-and-single-column layout
The public profile page SHALL render as a header block (avatar-or-initials,
name where applicable, headline, location, summary, skill chips) followed
by a single-column list of experience entries and then education entries,
rather than a flat, unstructured field list.

#### Scenario: Public mode header includes name
- **WHEN** the public page renders for a `public`-visibility profile
- **THEN** the header block includes the candidate's name

#### Scenario: Anonymous mode header omits name
- **WHEN** the public page renders for an `anonymous`-visibility profile
- **THEN** the header block omits the name (per the existing
  `talent-network-profile` anonymous-mode requirement — unchanged by this
  capability)

#### Scenario: No photo, no unavailable-data placeholders
- **WHEN** the public page renders, in either mode
- **THEN** no photo is rendered and no work-authorization or availability
  indicator is rendered, since neither is collected by this product
