## MODIFIED Requirements

### Requirement: Section navigation items

The shell SHALL present navigation to the account sections — Profile, Tracking, Activity, Search notifications, API keys, and My submissions — each linking to its `my/*` route. The item matching the current path SHALL be marked active, where a section is active when the path equals its route or is a descendant of it. Create actions and non-account links (e.g. Submit a job, Moderation) SHALL NOT appear in this navigation.

#### Scenario: Active item reflects the current route

- **WHEN** a user is on `/my/tracking/pipeline`
- **THEN** the Tracking navigation item is marked active and the others are not

#### Scenario: Activity is active on its sub-tabs

- **WHEN** a user is on `/my/activity/history`
- **THEN** the Activity navigation item is marked active and the others are not

#### Scenario: Navigating between sections

- **WHEN** a user selects a navigation item
- **THEN** the app navigates to that section's route without unmounting the shell
  or its navigation

### Requirement: Tracking sub-navigation preserved

The shell navigation SHALL be the top navigation level for sections that own sub-tabs; within their content columns Tracking SHALL render **Board** and **Pipeline** sub-tabs, and Activity SHALL render **Saved**, **History**, and **Matches** sub-tabs, unchanged by the shell. The History and Matches (AI fit) views SHALL live under Activity, not Tracking.

#### Scenario: Tracking retains its Board and Pipeline sub-tabs under the shell

- **WHEN** a signed-in user opens `/my/tracking`
- **THEN** the shell marks Tracking active and the Board and Pipeline sub-tabs are
  shown within the content column, with no History or Matches sub-tab

#### Scenario: Activity shows its Saved, History, and Matches sub-tabs

- **WHEN** a signed-in user opens `/my/activity`
- **THEN** the shell marks Activity active and the Saved, History, and Matches
  sub-tabs are shown within the content column
