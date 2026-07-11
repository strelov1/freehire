## ADDED Requirements

### Requirement: Live-streaming fit computation

The system SHALL provide an authenticated Server-Sent Events endpoint
`GET /api/v1/jobs/:slug/fit/stream` that runs the three-stage chain and emits, in order, events for
each stage's start and completion, the model's thinking tokens when available, each stage's
structured result (requirements after Stage 1, dimensions after Stage 2, the final verdict after
Stage 3), and a terminal completion event carrying the full analysis. On successful completion it
MUST cache the analysis exactly as the synchronous compute does (same per-(user, job) row and
staleness stamps). The synchronous `POST /jobs/:slug/fit` MUST remain for non-browser clients, and
both paths MUST produce the same final analysis for the same inputs.

#### Scenario: Stream emits stages then the final analysis

- **WHEN** a signed-in user with a CV opens the fit stream for a job
- **THEN** the connection emits stage progress and section events in order and ends with a completion event carrying the six-dimension analysis, which is then cached

#### Scenario: Thinking is best-effort

- **WHEN** the configured model emits no reasoning tokens
- **THEN** the stream still emits stage progress and the structured results, with the thinking events simply absent (never an error)

#### Scenario: Stream failure is reported, not fatal

- **WHEN** a stage fails mid-stream
- **THEN** the stream emits an error event and closes without caching a partial analysis

### Requirement: Dedicated fit analysis page

The SPA SHALL provide a dedicated page at `/jobs/[slug]/fit` presenting the full analysis in a
detailed, full-width layout (overall score + verdict, the six dimensions with their rationale, the
ATS requirement match, strengths, gaps, recommendation). When a fresh cached analysis exists it MUST
be server-rendered on first paint; otherwise (or on explicit recompute) the page MUST open the stream
and render the stage progress, the thinking panel, and each section progressively as it resolves.

#### Scenario: Fresh cache server-rendered

- **WHEN** the user opens the page for a job whose analysis is cached and fresh
- **THEN** the full analysis is in the server-rendered HTML with no client stream needed

#### Scenario: Cold page streams progressively

- **WHEN** the user opens the page with no fresh cached analysis
- **THEN** the page shows the stage stepper and fills the overall/dimensions/requirements/verdict sections as each stage resolves, ending on the complete analysis

### Requirement: Sidebar reduced to a summary linking to the page

The Profile-match sidebar block SHALL show only a short fit summary — the overall percentage, the
verdict label, and the single most important gap — with a link to the dedicated page. It MUST NOT run
the analysis inline. When no analysis is cached it MUST show an action that navigates to the page
(which starts the stream) rather than computing in the sidebar.

#### Scenario: Sidebar summarizes and links

- **WHEN** a cached analysis exists and the user views the job
- **THEN** the sidebar shows the overall %, the verdict, and the top gap, with a link to the full analysis page

#### Scenario: Sidebar with no analysis links to the page

- **WHEN** no analysis is cached
- **THEN** the sidebar shows an action that navigates to `/jobs/[slug]/fit` instead of computing inline
