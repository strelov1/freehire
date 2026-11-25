## ADDED Requirements

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
