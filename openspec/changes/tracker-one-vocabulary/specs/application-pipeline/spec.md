## MODIFIED Requirements

### Requirement: Pipeline aggregate endpoint

The system SHALL expose `GET /api/v1/me/tracking/pipeline`, authenticated with `RequireAuthOrKey` (session cookie or API key), returning the signed-in user's application counts aggregated server-side over **all** of their tracked applications. The response envelope SHALL be `{"data": {"applications": <int>, "stages": {"applied","screening","responded","interview","offer","accepted","rejected","withdrawn"}}}` where every stage key from the `internal/userjob` vocabulary is always present (zero when empty). The response SHALL NOT carry a `buckets` object: grouping is static and belongs to the vocabulary owner, so returning it would place the mapping in two places.

#### Scenario: Authenticated user with applications

- **WHEN** an authenticated user requests `GET /api/v1/me/tracking/pipeline`
- **THEN** the response is `200` with `data.applications` equal to the number of their tracked applications and `data.stages` carrying the per-stage counts that sum to `data.applications`

#### Scenario: Unauthenticated request

- **WHEN** a request without a valid session or API key hits `GET /api/v1/me/tracking/pipeline`
- **THEN** the response is `401`

#### Scenario: User with no applications

- **WHEN** an authenticated user who has never applied to or staged any job requests the endpoint
- **THEN** the response is `200` with `data.applications` equal to `0` and every stage count `0`

#### Scenario: Every stage key is present

- **WHEN** the response is serialized for any caller
- **THEN** it carries one key per value in the `internal/userjob` stage vocabulary, including those with a count of zero

### Requirement: Application counting and stage membership

The system SHALL count as an application every `applications` row where `applied_at IS NOT NULL OR stage IS NOT NULL`, and SHALL exclude saved-only rows (saved but never applied and with no stage). An application row whose `applied_at` is set but whose `stage` is null SHALL be counted under `applied`. Each counted application SHALL belong to exactly one group, and the stage→group membership SHALL be owned by a single table in `internal/userjob`, read by every surface rather than restated by any of them: `applied`, `screening` and `responded` → `applied`; `interview` → `interview`; `offer` → `offer`; `accepted`, `rejected` and `withdrawn` → `closed`.

#### Scenario: Each application is counted once

- **WHEN** the per-stage counts are computed for a set of applications
- **THEN** the sum of all stage counts equals `applications`

#### Scenario: Saved-only jobs are excluded

- **WHEN** a user has a job that is saved but never applied to and carries no stage
- **THEN** that job is not counted in `applications` and contributes to no stage

#### Scenario: Applied without an explicit stage counts as applied

- **WHEN** an application has `applied_at` set and `stage` null
- **THEN** it is counted under the `applied` stage

#### Scenario: Every stage belongs to exactly one group

- **WHEN** the stage vocabulary is enumerated
- **THEN** each stage appears in exactly one group, and a stage added to the vocabulary without a group fails the build

### Requirement: Interview and offer rates are an honest snapshot

The Pipeline SHALL present an **Interview Rate** equal to `(interview + offer + accepted) / applications` and an **Offer Rate** equal to `(offer + accepted) / applications`, both derived from the per-stage counts and presented as a current-status snapshot. Because only each job's current stage is stored, these rates SHALL be treated as a lower bound (an application rejected after interviewing appears only as `rejected`), and the UI SHALL communicate that the view is a current-status snapshot rather than historical conversion.

#### Scenario: Rates derived from stages

- **WHEN** a user has 100 applications with 20 at `interview`, 2 at `offer`, and 1 `accepted`
- **THEN** the Interview Rate is `23%` and the Offer Rate is `3%`

#### Scenario: Zero applications yields zero rates without division error

- **WHEN** a user has zero applications
- **THEN** both rates render as `0%` and no division-by-zero occurs

### Requirement: Pipeline tab in the tracking section

The tracking section SHALL offer a **Pipeline** tab at `/my/tracking/pipeline` alongside the Board, List and Calendar tabs, with Board remaining the default. The Pipeline tab SHALL render the application distribution as a single-level Sankey diagram of the four groups, each band carrying a per-stage breakdown so a settled group shows what settled it, together with Interview Rate and Offer Rate donut cards, using hand-built SVG with no new frontend dependency. The group labels SHALL be the generated ones, identical to the board's column labels. The tab SHALL be available only to signed-in users, inheriting the section's existing authentication gating.

#### Scenario: Signed-in user opens the Pipeline tab

- **WHEN** a signed-in user selects the Pipeline tab
- **THEN** the Sankey diagram and the two rate donuts render from the aggregate endpoint's data

#### Scenario: A settled group shows what settled it

- **WHEN** a user has 28 rejected applications and no accepted or withdrawn ones
- **THEN** the `Closed` band reads 28 and its breakdown names `Rejected` as the whole of it

#### Scenario: Group labels match the board

- **WHEN** the Pipeline tab and the Board tab are open on the same data
- **THEN** every group carries the same label in both, read from the generated vocabulary

#### Scenario: Empty state

- **WHEN** a signed-in user with no applications opens the Pipeline tab
- **THEN** a friendly empty message is shown instead of a zero-width diagram

#### Scenario: Default tab unchanged

- **WHEN** a signed-in user opens `/my/tracking` without selecting a tab
- **THEN** the Board tab is shown, not the Pipeline tab
