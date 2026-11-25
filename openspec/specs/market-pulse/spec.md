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

### Requirement: Weekly skill-demand snapshots are retained

The system SHALL persist one snapshot row per canonical skill per ISO week,
recording that skill's catalogue-wide open-job count at the time of the
snapshot, retained for approximately 6 months (26 weeks) and pruned beyond
that window. Snapshots SHALL be derived from the existing skill-demand rollup
rather than a separate aggregation over the `jobs` table.

#### Scenario: A new week's snapshot is recorded
- **WHEN** the rollup worker runs and no snapshot exists yet for the current
  ISO week for a given skill
- **THEN** a row is written recording that skill's current open-job count
  against the current week

#### Scenario: Repeated runs within the same week do not duplicate
- **WHEN** the rollup worker runs again later in the same ISO week
- **THEN** no additional row is written for that skill and week, and the
  existing row is left unchanged

#### Scenario: Snapshots older than the retention window are pruned
- **WHEN** the rollup worker runs
- **THEN** snapshot rows more than approximately 6 months old are removed

### Requirement: Personal skill-demand trend is readable by the signed-in user

The system SHALL expose an authenticated `GET /api/v1/me/market-pulse`
endpoint that returns, for each skill in the caller's own profile that has at
least one retained snapshot, the most recent open-job count, the percent
change since the earliest retained snapshot, and the full retained weekly
series for that skill. A profile skill with no retained snapshot yet SHALL be
omitted from the result rather than reported with a fabricated count. No
other user's data SHALL be readable through this endpoint. The response
SHALL use the standard list envelope `{"data": [...], "meta": {...}}`.

#### Scenario: A profile with skills returns their trends
- **WHEN** a signed-in user with one or more profile skills, each with at
  least one retained snapshot, requests `GET /api/v1/me/market-pulse`
- **THEN** the response is `200` with one `data` entry per profile skill,
  each carrying the skill name, the latest open-job count, a percent-change
  figure, and a series of `{week_start, open_count}` points

#### Scenario: A skill with exactly one snapshot has no percent change yet
- **WHEN** the caller has a profile skill for which exactly one weekly
  snapshot has been recorded
- **THEN** that skill appears in `data` with a one-point series and a `null`
  (not a fabricated) percent-change figure

#### Scenario: A skill with zero snapshots is omitted
- **WHEN** the caller has a profile skill for which no snapshot has ever been
  recorded (it has never appeared in an open job)
- **THEN** that skill does not appear in `data` at all

#### Scenario: An empty profile returns an empty result, not an error
- **WHEN** a signed-in user with no skills in their profile requests the
  endpoint
- **THEN** the response is `200` with an empty `data` array

#### Scenario: An unauthenticated request is rejected
- **WHEN** a request to `GET /api/v1/me/market-pulse` carries no valid session
- **THEN** the response is `401` and no data is returned

