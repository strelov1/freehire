## ADDED Requirements

### Requirement: Entry point gated to beta testers
The Talent Network entry button and the visibility fetch that seeds the
button's state SHALL be present only for a caller whose account has
`beta_tester` set; a non-beta caller SHALL see none of them.

#### Scenario: Non-beta caller sees no entry point
- **WHEN** a signed-in caller whose account does not have `beta_tester` set
  views `my/profile`
- **THEN** no Talent Network button and no `GET /me/talent-network`
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
  current mode's icon and name (e.g. "Talent Network: Public")

#### Scenario: Button always links to the same settings page
- **WHEN** the button is clicked, in any visibility state
- **THEN** the caller is taken to `/my/talent-network`

### Requirement: Dedicated settings page entry point
The Talent Network entry button SHALL navigate to a dedicated page at
`/my/talent-network`, rather than opening an overlay or switching to a tab
in the existing settings tab strip.

#### Scenario: Button navigates to the settings page
- **WHEN** the caller clicks the entry button on `my/profile`
- **THEN** the browser navigates to `/my/talent-network`

### Requirement: View-public-page button, shown only when there is something to view
The settings page SHALL show a "View your public page" button in its
header, styled as a primary call-to-action, only when the caller's
current visibility is `public` or `anonymous` — not when `off`, since no
public page resolves in that state. It SHALL NOT display the raw public
URL as text, and SHALL NOT offer a copy-to-clipboard action.

#### Scenario: Button hidden when off
- **WHEN** the settings page renders and the caller's visibility is `off`
- **THEN** no "View your public page" button is shown

#### Scenario: Button shown when public or anonymous
- **WHEN** the settings page renders and the caller's visibility is
  `public` or `anonymous`
- **THEN** a "View your public page" button is shown in the page header,
  opening the public page in a new tab; no raw URL text or copy action is
  present anywhere on the page

### Requirement: Icon-bearing mode picker
The settings page's Off/Public/Anonymous picker SHALL display a distinct
icon component alongside each option — `EyeOff` for Off, `Globe` for
Public, `VenetianMask` for Anonymous (all from `@lucide/svelte`) — not an
emoji character.

#### Scenario: All three icons present
- **WHEN** the settings page's mode picker renders
- **THEN** each of the three options shows its designated icon component
  next to its label and description

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
