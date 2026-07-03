## ADDED Requirements

### Requirement: Profile identity affordance

For a signed-in user the menu SHALL show an identity row consisting of an avatar
placeholder and the user's email. The avatar SHALL be a circle showing the first
character of the email, with a background colour deterministically derived from
the email (stable per user), and SHALL NOT require an uploaded image. The
identity row (avatar + email together) SHALL be a link to `/my/profile` and
selecting it SHALL close the menu and navigate there. For a signed-out user no
identity row SHALL be shown.

#### Scenario: Signed-in identity row links to the profile
- **WHEN** a signed-in user opens the menu
- **THEN** it shows an avatar (initial-in-a-circle) next to the user's email as a single clickable row, and selecting it closes the menu and navigates to `/my/profile`

#### Scenario: Avatar placeholder without an image
- **WHEN** a signed-in user with no uploaded picture opens the menu
- **THEN** the avatar shows the first character of their email on a colour derived from the email, and no broken/empty image is shown

#### Scenario: No identity row when signed out
- **WHEN** a signed-out user opens the menu
- **THEN** no avatar or email identity row is shown

## MODIFIED Requirements

### Requirement: Consolidated menu

The single menu SHALL contain the site nav links (Jobs, Companies, Collections,
Analytics, CLI, For recruiters, For companies), the theme toggle, and the auth
action. For a signed-in user the menu SHALL additionally contain the account
items (My jobs, Profile, Notifications, API keys, Submit a job, My submissions)
with a Log out action; the Moderation item SHALL appear only for a moderator.
The Profile item SHALL target `/my/profile`. For a signed-out user the menu
SHALL show a Sign in action instead of the account items. The menu SHALL close
after an item is selected, on `Escape`, and on outside click.

#### Scenario: Signed-out menu

- **WHEN** a signed-out user opens the menu
- **THEN** it shows the nav links, the theme toggle, and a Sign in action, and
  no account items

#### Scenario: Signed-in menu

- **WHEN** a signed-in non-moderator user opens the menu
- **THEN** it shows the nav links, the account items (My jobs, Profile,
  Notifications, API keys, Submit a job, My submissions), the theme toggle, and
  Log out, but not Moderation

#### Scenario: Profile item targets the single profile

- **WHEN** a signed-in user selects the Profile account item
- **THEN** the app navigates to `/my/profile`

#### Scenario: Moderator sees moderation

- **WHEN** a signed-in moderator opens the menu
- **THEN** the menu additionally shows the Moderation item

#### Scenario: Selecting an item closes the menu

- **WHEN** the menu is open and the user selects any link or action
- **THEN** the menu closes
