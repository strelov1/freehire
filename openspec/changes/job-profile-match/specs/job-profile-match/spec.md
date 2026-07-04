## ADDED Requirements

### Requirement: Per-job match endpoint

The system SHALL expose `GET /api/v1/jobs/:slug/match` behind `RequireAuthOrKey` (session cookie or API key), addressed by a job's public slug. It SHALL classify each skill of the open job against the caller's profile skills and return the classification plus a coverage percent in the standard `{"data": ...}` envelope. The computation SHALL be deterministic and MUST NOT call an LLM.

#### Scenario: Authenticated caller with a profile

- **WHEN** an authenticated caller requests the match for a job whose skills are `[react, typescript, graphql, nodejs, aws]` and their profile skills are `[react, typescript, gcp]`
- **THEN** the response `data` reports `total: 5`, `exact_count: 2` (`react`, `typescript`), `adjacent_count: 1` (`aws` via `gcp`), the `missing` list `[graphql, nodejs]`, and `coverage_percent: 50`

#### Scenario: Unauthenticated caller

- **WHEN** a caller without a valid session cookie or API key requests the match endpoint
- **THEN** the system SHALL respond `401` and MUST NOT return any match data

#### Scenario: Unknown job slug

- **WHEN** an authenticated caller requests the match for a slug that resolves to no job
- **THEN** the system SHALL respond `404`

### Requirement: Skill classification and coverage formula

Each job skill SHALL be classified as **exact** when the profile contains that canonical skill, else **adjacent** when the profile contains a neighbour of it per the curated adjacency dictionary (`internal/verdict/adjacent.go`), else **missing**. An adjacent classification SHALL carry the `via` skill — the specific held neighbour that satisfied it. Coverage percent SHALL be `round((exact_count + 0.5 × adjacent_count) / total × 100)`, where an exact match weighs 1 and an adjacent match weighs one half.

#### Scenario: Exact takes precedence over adjacent

- **WHEN** a job skill is present exactly in the profile and also has a held neighbour
- **THEN** it SHALL be classified `exact`, not `adjacent`

#### Scenario: Adjacent names its via skill

- **WHEN** a job requires `aws`, the profile lacks `aws` but holds `gcp`, and the dictionary treats them as neighbours
- **THEN** `aws` SHALL be classified `adjacent` with `via: "gcp"`

#### Scenario: Percent rounds half-weighted adjacents

- **WHEN** a job has 5 skills with 2 exact and 1 adjacent
- **THEN** `coverage_percent` SHALL be `50` (`round((2 + 0.5) / 5 × 100)`)

#### Scenario: Job with no recognised skills

- **WHEN** the job's skill list is empty
- **THEN** the endpoint SHALL report `total: 0` with empty `matched`, `adjacent`, and `missing` lists, and the sidebar SHALL render a "not enough data" state rather than a match block

### Requirement: Match response contract

The match response `data` SHALL contain `total`, `exact_count`, `adjacent_count`, `coverage_percent`, `matched` (list of skill names), `adjacent` (list of `{name, via}`), and `missing` (list of skill names). A corresponding TypeScript type SHALL be generated via `cmd/gen-contracts` so the SPA consumes a typed shape.

#### Scenario: Response envelope shape

- **WHEN** the endpoint returns a successful match
- **THEN** the body SHALL be `{"data": {total, exact_count, adjacent_count, coverage_percent, matched, adjacent, missing}}` with `adjacent` entries carrying both `name` and `via`

### Requirement: Sidebar match block states

The job-detail sidebar SHALL render a match block at its top with exactly four mutually exclusive states, choosing the state without redundant network calls. The guest and no-profile states SHALL show a lightly-blurred teaser (static, non-real figures) with a single footer call-to-action, and MUST NOT call the match endpoint.

#### Scenario: Not-enough-data state

- **WHEN** the open job has no recognised skills
- **THEN** the block SHALL show a "not enough data" card and SHALL NOT call the match endpoint

#### Scenario: Guest state

- **WHEN** the viewer is not authenticated and the job has skills
- **THEN** the block SHALL show a lightly-blurred teaser with a static percentage and a footer "Войти" button, and SHALL NOT call the match endpoint

#### Scenario: No-profile state

- **WHEN** the viewer is authenticated but has no profile or an empty profile skill list
- **THEN** the block SHALL show a lightly-blurred teaser with a footer "Загрузить CV" button, and SHALL NOT call the match endpoint

#### Scenario: Real match state

- **WHEN** the viewer is authenticated with a non-empty profile skill list and the job has skills
- **THEN** the block SHALL call the match endpoint and render the percentage, a two-colour progress bar (exact segment plus a half-weight adjacent segment), and three chip groups — Есть (exact), Близкие (adjacent, each hinting its `via` skill), and Не хватает (missing)
