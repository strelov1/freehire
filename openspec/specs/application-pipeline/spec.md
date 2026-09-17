# application-pipeline Specification

## Purpose
Reading where the caller's applications stand: the per-stage aggregate behind the tracking
section's Pipeline tab, and the two rates derived from it. The stage vocabulary and the group
membership it is drawn with belong to `tracking-stage-vocabulary`; this capability is the
aggregate over them.
## Requirements
### Requirement: Pipeline aggregate endpoint

The system SHALL expose `GET /api/v1/me/tracking/pipeline`, authenticated with `RequireAuthOrKey` (session cookie or API key), returning the signed-in user's application counts aggregated server-side over **all** of their tracked applications. The response envelope SHALL be `{"data": {"applications": <int>, "stages": {"applied","screening","responded","interview","offer","accepted","rejected","withdrawn"}}}` where every stage key from the `internal/application/userjob` vocabulary is always present (zero when empty). The response SHALL NOT carry a `buckets` object: grouping is static and belongs to the vocabulary owner, so returning it would place the mapping in two places.

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
- **THEN** it carries one key per value in the `internal/application/userjob` stage vocabulary, including those with a count of zero

### Requirement: Application counting and stage membership

The system SHALL count as an application every `applications` row where `applied_at IS NOT NULL OR stage IS NOT NULL`, and SHALL exclude saved-only rows (saved but never applied and with no stage). An application row whose `applied_at` is set but whose `stage` is null SHALL be counted under `applied`. Each counted application SHALL belong to exactly one group, and the stage→group membership SHALL be owned by a single table in `internal/application/userjob`, read by every surface rather than restated by any of them: `applied`, `screening` and `responded` → `applied`; `interview` → `interview`; `offer` → `offer`; `accepted`, `rejected` and `withdrawn` → `closed`.

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

### Requirement: Personal reply-rate benchmark on the pipeline endpoint

`GET /api/v1/me/tracking/pipeline` SHALL, for a caller with at least one connected mailbox,
compute the caller's own observable/answered application counts using the same definitions
`company-hiring-signal` uses (observable = the application belongs to a user with a connected
mailbox; answered = a non-retracted `employer_reply` event exists for it) and include them
alongside the global response rate `company-hiring-signal` publishes, gated by the same
ten-application sample floor applied to the caller's own count.

The global figure served here SHALL exclude the caller's own contribution to it — the comparison
means "you against everyone else," and a caller whose own applications are a non-trivial share of
the platform's observable total must not be partly compared against themselves. Because the
global figure is a periodic rollup while the caller's own count is read live, a caller's very
recent application may not yet be reflected in the total it is subtracted from; the excluded
result SHALL clamp at zero on each side rather than go negative.

The field SHALL be absent — never zero, never an estimate — when any of the following holds: the
caller has no connected mailbox, the caller's own observable count is under ten, or the global
response rate itself is not currently available. A personal rate with nothing to compare it
against is not the benchmark this requirement serves.

#### Scenario: Caller clears both gates

- **WHEN** the caller has fifteen observable applications, six of them answered, and the global
  response rate is available
- **THEN** the response includes both the caller's own rate and the global rate

#### Scenario: Caller below their own sample gate

- **WHEN** the caller has four observable applications
- **THEN** the reply-rate field is absent, regardless of whether the global rate is available

#### Scenario: Caller has no connected mailbox

- **WHEN** the caller has twenty applied jobs but no connected mailbox
- **THEN** the reply-rate field is absent

#### Scenario: Global rate not yet available

- **WHEN** the caller clears their own sample gate but the global response rate has not yet been
  published (e.g. before the rollup's first run after this feature ships)
- **THEN** the reply-rate field is absent

#### Scenario: The caller's own contribution is excluded from the global figure

- **WHEN** the platform-wide observable/answered totals include the caller's own twelve
  observable applications and four answers
- **THEN** the served global figure is computed over the platform total minus those twelve and
  four, not the raw platform total

#### Scenario: A stale rollup does not produce a negative global figure

- **WHEN** the caller's own live count includes an application more recent than the last
  completed rollup, such that subtracting it would make either side negative
- **THEN** that side of the global figure is zero, never negative

### Requirement: Pipeline tab in the tracking section

The tracking section SHALL offer a **Pipeline** tab at `/my/tracking/pipeline` alongside the Board, List and Calendar tabs, with Board remaining the default. The Pipeline tab SHALL render the application distribution as a single-level Sankey diagram of the four groups, each band carrying a per-stage breakdown so a settled group shows what settled it, together with Interview Rate and Offer Rate donut cards, using hand-built SVG with no new frontend dependency. The group labels SHALL be the generated ones, identical to the board's column labels. The tab SHALL be available only to signed-in users, inheriting the section's existing authentication gating. When the pipeline endpoint's personal reply-rate benchmark field is present, the tab SHALL also render a reply-rate comparison card (the caller's own rate beside the global rate); when the field is absent, the tab SHALL render exactly as before, with no partial or placeholder card.

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

#### Scenario: Reply-rate card renders when the benchmark is available

- **WHEN** the pipeline response carries the personal reply-rate benchmark field
- **THEN** the Pipeline tab shows the comparison card beside the existing donut cards

#### Scenario: No reply-rate card when the benchmark is absent

- **WHEN** the pipeline response carries no personal reply-rate benchmark field
- **THEN** the Pipeline tab renders exactly as it did before this change, with no card and no
  loading placeholder in its place
