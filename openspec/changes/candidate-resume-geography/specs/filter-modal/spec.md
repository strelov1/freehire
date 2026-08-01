## MODIFIED Requirements

### Requirement: The filter modal can apply the signed-in user's profile

The jobs filter modal SHALL offer, in its header, an **Apply my profile** action for a
signed-in user who has a saved profile. Activating it SHALL reset the staged filters and
then seed them from the user's profile: each profile specialization SHALL be staged as a
`category` value and each profile skill SHALL be staged as an included `skills` value.
Each profile excluded skill SHALL be staged as an **excluded** `skills` value (into the
`skills` facet's exclude set, so it commits as `?skills_exclude=…`). When the
profile carries a `location_preferences` block, the action SHALL additionally seed the
location facets by flattening the three blocks: `work_mode` from `work_modes`; `regions`
from the union of `remote.regions` and `relocation.regions`; `countries` from the union of
`remote.countries`, `base.country`, and `relocation.countries`; `cities` from the union of
`base.city` and `relocation.cities`; and `relocation` staged as `supported` and `required`
when `relocation.open` is true. Empty or absent parts contribute nothing.

`base` SHALL contribute to the seeded `countries` and `cities` facets **only for a user
who accepts on-site or hybrid work**. `base` states where the user lives, not where they
want the work to be; for a user who accepts only remote work those are different places,
and seeding their home country as a job-country filter would silently narrow their search
to the one country they least need the work to be in. For a user who accepts physical
work the two coincide — the job must be commutable — so the contribution is kept.

The action SHALL only stage — it SHALL NOT change the live job list; the seeded selection
is applied through the existing **Show results** commit, so the user previews (and MAY
adjust) the profile-derived filters before applying.

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

#### Scenario: Applying a profile with excluded skills seeds the skills exclude set
- **WHEN** a signed-in user whose profile has skills `[go]` and excluded skills `[php]`
  activates **Apply my profile**
- **THEN** the `skills` facet is staged with `go` included and `php` excluded, and on
  **Show results** the committed filter carries `?skills=go` and `?skills_exclude=php`

#### Scenario: Applying a profile with location preferences seeds the location facets
- **WHEN** a signed-in user whose profile has `work_modes` `[remote, onsite]`,
  `remote.regions` `[latam]`, `base` `{country: br, city: "Florianópolis"}`, and
  `relocation` `{open: true, cities: ["Berlin"]}` activates **Apply my profile**
- **THEN** the staged filters include `work_mode` `[remote, onsite]`, `regions` `[latam]`,
  `countries` `[br]`, `cities` `["Florianópolis", "Berlin"]`, and `relocation`
  `[supported, required]`, and the job list is unchanged until **Show results** is activated

#### Scenario: A remote-only user's base does not narrow their search
- **WHEN** a signed-in user whose `work_modes` are `[remote]` alone, with `base`
  `{country: co, city: "Manizales"}` and `remote.regions` `[latam]`, activates
  **Apply my profile**
- **THEN** the staged `regions` are `[latam]` and the staged `countries` and `cities` are
  empty — their home country is not staged as a job-country filter

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
