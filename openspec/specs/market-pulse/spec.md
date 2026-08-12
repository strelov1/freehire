# market-pulse Specification

## Purpose
TBD - created by archiving change market-coverage-into-pulse. Update Purpose after archive.
## Requirements
### Requirement: Tabbed market page with Coverage and Skill trend

The `/my/market-pulse` page SHALL present two tabs, **Coverage** and **Skill
trend**, using the same tab-strip component `/my/profile` uses. The **Coverage**
tab SHALL be the default active tab on page load. The page SHALL require an
authenticated session; a signed-out visitor SHALL see a sign-in prompt instead
of either tab's content.

#### Scenario: Default tab on load
- **WHEN** a signed-in user navigates to `/my/market-pulse`
- **THEN** the Coverage tab is active and its content is shown

#### Scenario: Switching tabs
- **WHEN** the user selects the Skill trend tab
- **THEN** the Coverage tab's content and filter controls are hidden and the
  Skill trend content is shown

#### Scenario: Signed-out visitor
- **WHEN** a signed-out visitor opens `/my/market-pulse`
- **THEN** neither tab's content is fetched or rendered; a sign-in prompt is
  shown instead

### Requirement: Coverage tab computes the verdict against a filterable role

The Coverage tab SHALL compute and display the caller's market-coverage
verdict (`GET /me/profile/verdict`) for a role/region/seniority selection that
defaults to the caller's profile specializations and can be refined via a
filter summary sidebar, a mobile edge-tab, and a two-pane filter modal —
carrying the coverage percent, the ranked gap skills, and each gap's unlock
count.

#### Scenario: Default role from the profile
- **WHEN** a signed-in user with a profile opens the Coverage tab with no
  filter override
- **THEN** the verdict is computed for the union of the profile's saved
  specializations

#### Scenario: Refining the comparison role
- **WHEN** the user changes the role, region, or seniority filter
- **THEN** the verdict recomputes for the new selection without altering the
  stored profile

### Requirement: Coverage tab empty state without a profile

When the caller has no saved profile, the Coverage tab SHALL NOT attempt to
compute a verdict. It SHALL show an empty state explaining that a profile is
required, with a call-to-action linking to `/my/profile`.

#### Scenario: No profile yet
- **WHEN** a signed-in user with no saved profile opens the Coverage tab
- **THEN** no verdict request is made; an empty state with a link to
  `/my/profile` is shown instead

### Requirement: Filter controls scoped to the Coverage tab only

The filter summary sidebar, mobile edge-tab, and filter modal SHALL be shown
only while the Coverage tab is active. They SHALL NOT appear while the Skill
trend tab is active.

#### Scenario: Filters hidden on Skill trend
- **WHEN** the user is on the Skill trend tab
- **THEN** no filter summary, edge-tab, or modal is rendered

### Requirement: Skill trend tab shows personalized skill-demand history

The Skill trend tab SHALL show one card per profile skill that has at least
one retained weekly demand snapshot, each with its current open-role count, a
week-over-week percent change, and a sparkline of its retained history. The
tab SHALL offer a substring search over the caller's skill names. When no
profile skill has any retained history, the tab SHALL show an empty state
with a call-to-action linking to `/my/profile`.

#### Scenario: Cards for skills with history
- **WHEN** a signed-in user's profile skills have retained weekly snapshots
- **THEN** the Skill trend tab shows one card per such skill with its open
  count, percent change, and sparkline

#### Scenario: Filtering by name
- **WHEN** the user types into the skill search input
- **THEN** only cards whose skill name matches the query (case-insensitive
  substring) remain visible

#### Scenario: No trend data yet
- **WHEN** none of the profile's skills have a retained weekly snapshot
- **THEN** an empty state is shown with a call-to-action linking to
  `/my/profile`

#### Scenario: Drilling into one skill
- **WHEN** the user opens a skill's card
- **THEN** they navigate to `/my/market-pulse/[skill]`, that skill's full
  retained history detail view

