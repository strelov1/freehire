## ADDED Requirements

### Requirement: Pipeline aggregate endpoint

The system SHALL expose `GET /api/v1/me/jobs/pipeline`, authenticated with `RequireAuthOrKey` (session cookie or API key), returning the signed-in user's application counts aggregated server-side over **all** of their tracked applications. The response envelope SHALL be `{"data": {"applications": <int>, "buckets": {"no_answer","in_progress","interviewing","offer","accepted","rejected","declined"}}}` where every bucket key is always present (zero when empty).

#### Scenario: Authenticated user with applications

- **WHEN** an authenticated user requests `GET /api/v1/me/jobs/pipeline`
- **THEN** the response is `200` with `data.applications` equal to the number of their tracked applications and `data.buckets` carrying the per-bucket counts that sum to `data.applications`

#### Scenario: Unauthenticated request

- **WHEN** a request without a valid session or API key hits `GET /api/v1/me/jobs/pipeline`
- **THEN** the response is `401`

#### Scenario: User with no applications

- **WHEN** an authenticated user who has never applied to or staged any job requests the endpoint
- **THEN** the response is `200` with `data.applications` equal to `0` and every bucket count `0`

### Requirement: Application counting and stage-to-bucket mapping

The system SHALL count as an application every `user_jobs` row where `applied_at IS NOT NULL OR stage IS NOT NULL`, and SHALL exclude saved-only rows (saved but never applied and with no stage). Each counted application SHALL fall into exactly one bucket determined by its current stage: `applied` (no further stage) → `no_answer`; `screening` and `responded` → `in_progress`; `interview` → `interviewing`; `offer` → `offer`; `accepted` → `accepted`; `rejected` → `rejected`; `withdrawn` → `declined`. An application row whose `applied_at` is set but whose `stage` is null SHALL map to `no_answer`. The mapping SHALL be owned by a single pure function in Go.

#### Scenario: Each application is counted once

- **WHEN** the buckets are computed for a set of applications
- **THEN** the sum of all seven bucket counts equals `applications`

#### Scenario: Saved-only jobs are excluded

- **WHEN** a user has a job that is saved but never applied to and carries no stage
- **THEN** that job is not counted in `applications` and contributes to no bucket

#### Scenario: Applied without an explicit stage maps to no_answer

- **WHEN** an application has `applied_at` set and `stage` null
- **THEN** it is counted in the `no_answer` bucket

### Requirement: Interview and offer rates are an honest snapshot

The Pipeline SHALL present an **Interview Rate** equal to `(interviewing + offer + accepted) / applications` and an **Offer Rate** equal to `(offer + accepted) / applications`, both as a current-status snapshot. Because only each job's current stage is stored, these rates SHALL be treated as a lower bound (an application rejected after interviewing appears only as `rejected`), and the UI SHALL communicate that the view is a current-status snapshot rather than historical conversion.

#### Scenario: Rates derived from buckets

- **WHEN** a user has 100 applications with 20 interviewing, 2 offer, and 1 accepted
- **THEN** the Interview Rate is `23%` and the Offer Rate is `3%`

#### Scenario: Zero applications yields zero rates without division error

- **WHEN** a user has zero applications
- **THEN** both rates render as `0%` and no division-by-zero occurs

### Requirement: Pipeline tab on the My Jobs page

The `/my/jobs` page SHALL offer a **Pipeline** tab alongside the existing Board and History tabs, with Board remaining the default tab. The Pipeline tab SHALL render the application distribution as a single-level Sankey diagram (Applications fanning into the seven status buckets, ribbon widths proportional to counts) together with Interview Rate and Offer Rate donut cards, using hand-built SVG with no new frontend dependency. The tab SHALL be available only to signed-in users, inheriting the page's existing authentication gating.

#### Scenario: Signed-in user opens the Pipeline tab

- **WHEN** a signed-in user selects the Pipeline tab
- **THEN** the Sankey diagram and the two rate donuts render from the aggregate endpoint's data

#### Scenario: Empty state

- **WHEN** a signed-in user with no applications opens the Pipeline tab
- **THEN** a friendly empty message is shown instead of a zero-width diagram

#### Scenario: Default tab unchanged

- **WHEN** a signed-in user opens `/my/jobs` without selecting a tab
- **THEN** the Board tab is shown, not the Pipeline tab
