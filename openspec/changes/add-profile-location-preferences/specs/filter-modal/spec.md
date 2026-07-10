# filter-modal Specification (delta)

## MODIFIED Requirements

### Requirement: The filter modal can apply the signed-in user's profile

The jobs filter modal SHALL offer, in its header, an **Apply my profile** action for a
signed-in user who has a saved profile. Activating it SHALL reset the staged filters and
then seed them from the user's profile: each profile specialization SHALL be staged as a
`category` value and each profile skill SHALL be staged as a `skills` value. When the
profile carries a `location_preferences` block, the action SHALL additionally seed the
location facets by flattening the three blocks: `work_mode` from `work_modes`; `regions`
from the union of `remote.regions` and `relocation.regions`; `countries` from the union of
`remote.countries`, `base.country`, and `relocation.countries`; `cities` from the union of
`base.city` and `relocation.cities`; and `relocation` staged as `supported` and `required`
when `relocation.open` is true. Empty or absent parts contribute nothing. The action
SHALL only stage — it SHALL NOT change the live job list; the seeded selection is applied
through the existing **Show results** commit, so the user previews (and MAY adjust) the
profile-derived filters before applying.

The action SHALL appear only on the full jobs filter modal (not on reuses that restrict
the rail to a facet subset, such as the profile-comparison modal). When the signed-in
user has no saved profile, the header SHALL instead present a link to create one at
`/my/profile`. When no user is signed in, neither the action nor the link SHALL appear.

#### Scenario: Applying the profile resets and seeds the staged filters

- **WHEN** a signed-in user with a saved profile (specializations `A`, `B`; skills `x`,
  `y`) has some unrelated staged filters and activates **Apply my profile**
- **THEN** the previously staged filters are cleared, the `category` facet is staged with
  `A` and `B`, the `skills` facet is staged with `x` and `y`, and the job list is
  unchanged until **Show results** is activated

#### Scenario: Applying a profile with location preferences seeds the location facets

- **WHEN** a signed-in user whose profile has `work_modes` `[remote, onsite]`,
  `remote.regions` `[latam]`, `base` `{country: br, city: "Florianópolis"}`, and
  `relocation` `{open: true, cities: ["Berlin"]}` activates **Apply my profile**
- **THEN** the staged filters include `work_mode` `[remote, onsite]`, `regions` `[latam]`,
  `countries` `[br]`, `cities` `["Florianópolis", "Berlin"]`, and `relocation`
  `[supported, required]`, and the job list is unchanged until **Show results** is activated

#### Scenario: Applying a profile without location preferences seeds no location facets

- **WHEN** a signed-in user whose profile has no `location_preferences` block activates
  **Apply my profile**
- **THEN** only the `category` and `skills` facets are staged and the location facets
  (`work_mode`, `regions`, `countries`, `cities`, `relocation`) remain empty

#### Scenario: Show results commits the profile-derived filters

- **WHEN** the user has applied their profile in the modal and then activates **Show
  results**
- **THEN** the staged selections become the live (URL-synced) filter state and the modal
  closes

#### Scenario: No profile shows a create-profile link instead

- **WHEN** a signed-in user who has no saved profile opens the jobs filter modal
- **THEN** the header shows a link to create a profile at `/my/profile` and no
  **Apply my profile** action

#### Scenario: Signed-out users see neither action nor link

- **WHEN** a signed-out user opens the jobs filter modal
- **THEN** the header shows neither the **Apply my profile** action nor the
  create-profile link
