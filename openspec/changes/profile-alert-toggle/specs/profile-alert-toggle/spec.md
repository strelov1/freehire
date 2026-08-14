## ADDED Requirements

### Requirement: Profile alert toggle

`/my/notifications/searches` (the Search alerts page) SHALL present a
toggle, "Notify me about jobs matching my profile," shown only for an
account that has a candidate profile to derive filters from. Enabling it
SHALL create a saved search from the current profile (using the same
filter-derivation the "Apply my profile" filters control already uses)
marked as profile-derived, and subscribe it on the email channel — always
deliverable, unlike telegram/push, which the account's notification
settings may list as preferred without the user ever having linked them.
Disabling it SHALL remove that saved search (and its subscription with it).
The toggle's shown state SHALL reflect whether a profile-derived saved
search currently exists for the account, not a separately stored flag.

#### Scenario: Enabling creates a search and subscribes it

- **WHEN** a signed-in user with a candidate profile and no profile-derived
  saved search enables the toggle
- **THEN** a saved search is created from their current profile's filters,
  marked profile-derived, and subscribed on the email channel

#### Scenario: Disabling removes the search

- **WHEN** a signed-in user with an active profile-derived saved search
  disables the toggle
- **THEN** that saved search (and its subscription) is deleted

#### Scenario: Toggle reflects manual deletion

- **WHEN** a signed-in user manually deletes their profile-derived saved
  search from the search-alerts list
- **THEN** the toggle on the same page shows as off

#### Scenario: Hidden without a candidate profile

- **WHEN** a signed-in user with no candidate profile visits the Search
  alerts page
- **THEN** the toggle is not shown, since there is no profile to derive
  filters from

#### Scenario: Subscribing over an unlinked preferred channel is avoided

- **WHEN** a signed-in user enables the toggle while their account's
  notification settings prefer telegram or push
- **THEN** the created search is still subscribed on the email channel, not
  the unlinked preferred channel

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
