## ADDED Requirements

### Requirement: Profile alert toggle

`/my/profile` SHALL present a toggle, "Notify me about jobs matching my
profile." Enabling it SHALL create a saved search from the current profile
(using the same filter-derivation the "Apply my profile" filters control
already uses) marked as profile-derived, and subscribe it on the account's
default notification channel. Disabling it SHALL remove that saved search
(and its subscription with it). The toggle's shown state SHALL reflect
whether a profile-derived saved search currently exists for the account,
not a separately stored flag.

#### Scenario: Enabling creates a search and subscribes it

- **WHEN** a signed-in user with a candidate profile and no profile-derived
  saved search enables the toggle
- **THEN** a saved search is created from their current profile's filters,
  marked profile-derived, and subscribed on the account's default channel

#### Scenario: Disabling removes the search

- **WHEN** a signed-in user with an active profile-derived saved search
  disables the toggle
- **THEN** that saved search (and its subscription) is deleted

#### Scenario: Toggle reflects manual deletion

- **WHEN** a signed-in user manually deletes their profile-derived saved
  search from the search-alerts list
- **THEN** the profile page's toggle shows as off

#### Scenario: Default channel when none configured

- **WHEN** a signed-in user enables the toggle with no notification
  channels ever configured
- **THEN** the created search is subscribed on the email channel

### Requirement: Profile-derived search stays in sync

Every time the account's candidate profile is saved, if a profile-derived
saved search exists, its query SHALL be recomputed from the just-saved
profile and updated in place.

#### Scenario: Profile edit updates the derived search

- **WHEN** a signed-in user with the toggle enabled edits and saves their
  profile (e.g. adds a specialization)
- **THEN** the profile-derived saved search's query is updated to reflect
  the new profile, without the user taking any additional action

#### Scenario: Sync is a no-op with the toggle off

- **WHEN** a signed-in user without a profile-derived saved search saves
  their profile
- **THEN** no saved search is created or modified as a side effect
