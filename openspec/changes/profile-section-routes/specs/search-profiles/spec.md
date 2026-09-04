## ADDED Requirements

### Requirement: Profile sections have dedicated routes
Each section of the `/my/profile` view (Profile, Contacts, Location, Skills,
Experience, Education, Screening answers, Settings) SHALL be reachable at its
own URL under `/my/profile/*` (Profile at the bare `/my/profile`), rather than
through in-page state alone. Navigating directly to a section's URL, reloading
on it, or following a link to it SHALL show that section, not the default. A
section's URL SHALL NOT change what that section does or how it saves — this
requirement governs only how each section is reached. While a profile does not
yet exist, every one of these URLs SHALL show the same one-time set-up view
(per the "Profile management UI" requirement) rather than a section's normal
content, matching today's behavior of hiding the section navigation until a
profile exists.

#### Scenario: Deep link opens the right section
- **WHEN** a signed-in user with an existing profile navigates directly to
  `/my/profile/experience` (fresh navigation, not a client-side tab click)
- **THEN** the Experience section renders, with the Experience tab shown as
  selected

#### Scenario: Reload preserves the section
- **WHEN** a signed-in user on `/my/profile/skills` reloads the page
- **THEN** the Skills section renders again, not the default Profile section

#### Scenario: Old query-param links still resolve
- **WHEN** a signed-in user follows a previously shared link to
  `/my/profile?tab=contacts`
- **THEN** the app redirects to `/my/profile/contacts` and the Contacts section
  renders

#### Scenario: Section routes are inert before a profile exists
- **WHEN** a signed-in user with no profile yet navigates directly to
  `/my/profile/education`
- **THEN** the one-time set-up view renders instead of the Education section,
  the same as navigating to bare `/my/profile` would show
