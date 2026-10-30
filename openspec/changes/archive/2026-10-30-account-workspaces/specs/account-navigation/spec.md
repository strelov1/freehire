## MODIFIED Requirements

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
