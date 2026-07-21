## MODIFIED Requirements

### Requirement: Section navigation items

The shell SHALL present navigation to the account sections — Profile, Tracking,
Search notifications, API keys, My submissions, and Credits — each linking to its
`my/*` route. The item matching the current path SHALL be marked active, where a
section is active when the path equals its route or is a descendant of it. Create
actions and non-account links (e.g. Submit a job, Moderation) SHALL NOT appear in
this navigation.

#### Scenario: Active item reflects the current route

- **WHEN** a user is on `/my/tracking/pipeline`
- **THEN** the Tracking navigation item is marked active and the others are not

#### Scenario: Navigating between sections

- **WHEN** a user selects a navigation item
- **THEN** the app navigates to that section's route without unmounting the shell
  or its navigation

#### Scenario: Credits section is reachable from the navigation

- **WHEN** a signed-in user selects the Credits navigation item
- **THEN** the app navigates to `/my/credits` and marks that item active
