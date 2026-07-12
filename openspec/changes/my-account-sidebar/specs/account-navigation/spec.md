## ADDED Requirements

### Requirement: Account section shell

The signed-in account area SHALL render every `my/*` route inside a single shared
shell owning the width container, the `noindex` robots tag, and the section
navigation. Individual `my/*` pages SHALL NOT set their own outer width
container, `noindex` tag, or per-page sign-in gate for these concerns; they
render only their own `<title>`, header, and body inside the shell's content
column.

#### Scenario: Every account page renders inside the shell

- **WHEN** a signed-in user opens any `my/*` page
- **THEN** the page content appears in the shell's content column beside the
  section navigation, within the shared width container

#### Scenario: Account pages are excluded from search indexing

- **WHEN** any `my/*` page is loaded
- **THEN** the response head carries a `noindex` robots directive set once by the
  shell

### Requirement: Section navigation items

The shell SHALL present navigation to the six account sections — Profile,
Tracking, Recommendations, Saved searches & alerts, API keys, and My submissions
— each linking to its `my/*` route. The item matching the current path SHALL be
marked active, where a section is active when the path equals its route or is a
descendant of it. Create actions and non-account links (e.g. Submit a job,
Moderation) SHALL NOT appear in this navigation.

#### Scenario: Active item reflects the current route

- **WHEN** a user is on `/my/tracking/pipeline`
- **THEN** the Tracking navigation item is marked active and the others are not

#### Scenario: Navigating between sections

- **WHEN** a user selects a navigation item
- **THEN** the app navigates to that section's route without unmounting the shell
  or its navigation

### Requirement: Responsive navigation form

The section navigation SHALL render as a vertical sidebar beside the content on
wide (`lg` and up) viewports and as a horizontal, scrollable tab strip above the
content on narrower viewports. Exactly one of the two forms SHALL be visible at
any viewport width.

#### Scenario: Wide viewport shows the sidebar

- **WHEN** a signed-in user views an account page on a `lg`-or-wider viewport
- **THEN** the navigation renders as a vertical sidebar and the horizontal strip
  is hidden

#### Scenario: Narrow viewport shows the tab strip

- **WHEN** a signed-in user views an account page below the `lg` breakpoint
- **THEN** the navigation renders as a horizontal scrollable strip above the
  content and the vertical sidebar is hidden

### Requirement: Unified account auth gate

The shell SHALL render the navigation and page content only for a signed-in
user. For a signed-out visitor it SHALL render a single sign-in prompt for the
whole section — with an action that opens the sign-in dialog — in place of the
navigation and content.

#### Scenario: Signed-out visitor sees one sign-in prompt

- **WHEN** a signed-out visitor opens any `my/*` page
- **THEN** a single sign-in prompt with a sign-in action is shown instead of the
  navigation and page body

### Requirement: Tracking sub-navigation preserved

The shell navigation SHALL be the top navigation level for Tracking; Tracking's
existing Board, Pipeline, History, and AI-fit sub-views SHALL remain reachable as
sub-tabs within the Tracking content, unchanged by the shell.

#### Scenario: Tracking retains its sub-tabs under the shell

- **WHEN** a signed-in user opens `/my/tracking`
- **THEN** the shell marks Tracking active and the Board/Pipeline/History/AI-fit
  sub-tabs are shown within the content column

### Requirement: Account index redirect

Requesting the bare `/my` path SHALL redirect to `/my/tracking` rather than
returning a not-found response.

#### Scenario: Bare /my redirects

- **WHEN** a user navigates to `/my`
- **THEN** the app redirects to `/my/tracking`
