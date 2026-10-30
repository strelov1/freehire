# account-navigation Specification

## Purpose

The in-section navigation and shared chrome for the signed-in `my/*` account
area: a single shell that owns the width container, the `noindex` robots tag, a
unified auth-gate, and the section navigation (a vertical sidebar on wide
viewports, a horizontal tab strip on narrow ones), plus the bare-`/my` redirect.
## Requirements
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

The account area SHALL be divided into four workspaces — Job search, Mentor, Referrer and
Employer —
and the shell SHALL present navigation to the sections of the CURRENT workspace only, each
linking to its `my/*` route. The current workspace SHALL be derived from the path: the
workspace of the most specific section whose route equals the path or is an ancestor of it,
and Job search when no section matches. Sections SHALL be grouped:

- Job search: Profile, Activity, Tracking, Inbox, Market Pulse, Agent, Tailor; then a group
  labelled as anonymous ("Get found · anonymous") holding Talent Network and Interview
  buddies; then "Get help" holding Referral requests and Mentorship.
- Mentor: Booked with you, Schedule, Mentor profile.
- Referrer: Offers to refer, Incoming requests.
- Employer: Company profile, Vacancies.
- Every workspace SHALL end with the same Settings group: Notifications, Integrations,
  Contributions, Plan, Account.

Exactly one item SHALL be marked active: the most specific section whose route equals the
current path or is an ancestor of it. Mentorship and every Mentor section SHALL be visible
only to a beta tester. Create actions and non-account links (e.g. Submit a job,
Moderation) SHALL NOT appear in this navigation.

#### Scenario: Active item reflects the current route

- **WHEN** a user is on `/my/tracking/pipeline`
- **THEN** the Tracking navigation item is marked active and the others are not

#### Scenario: Job search groups its sections

- **WHEN** a signed-in user opens `/my/profile`
- **THEN** the navigation shows the Job search sections, with Talent Network and Interview
  buddies under a group labelled as anonymous, and the Settings group last

#### Scenario: A mentor pane opens the Mentor workspace

- **WHEN** a beta tester opens `/my/mentorship/schedule`
- **THEN** the navigation shows the Mentor sections and the Settings group, with Schedule
  marked active and no Job search sections

#### Scenario: A mentor opening a booking stays in the Mentor workspace

- **WHEN** a mentor opens one of their bookings from Booked with you
- **THEN** the session opens at `/my/mentorship/bookings/<id>`, the navigation shows the
  Mentor sections with Booked with you marked active, and the page's back link returns to
  Booked with you

#### Scenario: The member's own sessions stay in Job search

- **WHEN** a beta tester opens `/my/mentorship`
- **THEN** the navigation shows the Job search sections with Mentorship marked active

#### Scenario: The company pages open the Employer workspace

- **WHEN** a signed-in user opens `/my/employer/vacancies`
- **THEN** the navigation shows Company profile and Vacancies, with Vacancies marked
  active, and the Settings group

#### Scenario: Asking for a referral and offering one live in different workspaces

- **WHEN** a signed-in user opens `/my/referrals`
- **THEN** the navigation shows the Job search sections with Referral requests marked active
- **WHEN** the same user opens `/my/referrals/incoming`
- **THEN** the navigation shows the Referrer sections with Incoming requests marked active

#### Scenario: An old referral tab link still lands

- **WHEN** a user follows `/my/referrals?tab=incoming` from an earlier email
- **THEN** they are redirected to `/my/referrals/incoming`

#### Scenario: Navigating between sections

- **WHEN** a user selects a navigation item
- **THEN** the app navigates to that section's route without unmounting the shell
  or its navigation

#### Scenario: Notifications item covers all its sub-routes

- **WHEN** a user is on `/my/notifications/searches` or
  `/my/notifications/settings`
- **THEN** the Notifications navigation item is marked active

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

### Requirement: Full-width surface navigation rail

A full-width workspace that opts out of the account shell SHALL present the
account section navigation as a compact, icon-only rail pinned to the left edge,
so it is not left without section navigation. This covers the Agent page
(`/my/assistant`) and the Tailor CV workspace (`/tailor/<slug>`). The rail
SHALL list the sections of the Job search workspace followed by the Settings group, with the
same order and beta gating as the account sidebar, and SHALL NOT include create actions or
non-account links. Each rail item SHALL render its section's icon with no text label, expose
its section label as a hover tooltip, link to its `my/*` route, and be marked active by the
same rule as the account sidebar. The rail SHALL be shown only to a signed-in user.

#### Scenario: Agent page shows the icon-only rail

- **WHEN** a signed-in user opens `/my/assistant`
- **THEN** a compact icon-only navigation rail is pinned to the left edge, listing
  the Job search sections and the Settings group

#### Scenario: Tailor CV workspace shows the icon-only rail

- **WHEN** a signed-in user opens a `/tailor/<slug>` CV workspace
- **THEN** the same compact icon-only navigation rail is pinned to the left edge,
  and it remains present across the workspace's loading, error, and ready states

#### Scenario: Rail item reflects the active section

- **WHEN** a signed-in user is on `/my/assistant`
- **THEN** the Agent item in the rail is marked active and the other items are not

#### Scenario: Rail item exposes its label on hover

- **WHEN** a user hovers a rail item that shows only an icon
- **THEN** the item's section label is available as a tooltip

#### Scenario: Signed-out visitor sees no rail

- **WHEN** a signed-out visitor opens `/my/assistant`
- **THEN** the navigation rail is not rendered

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
existing Board, Pipeline, History, and Matches sub-views SHALL remain reachable
as sub-tabs within the Tracking content, unchanged by the shell.

#### Scenario: Tracking retains its sub-tabs under the shell

- **WHEN** a signed-in user opens `/my/tracking`
- **THEN** the shell marks Tracking active and the Board/Pipeline/History/Matches
  sub-tabs are shown within the content column

### Requirement: Account index redirect

Requesting the bare `/my` path SHALL redirect to `/my/tracking` rather than
returning a not-found response.

#### Scenario: Bare /my redirects

- **WHEN** a user navigates to `/my`
- **THEN** the app redirects to `/my/tracking`

