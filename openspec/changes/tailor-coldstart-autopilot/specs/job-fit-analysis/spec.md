## MODIFIED Requirements

### Requirement: Fit analysis surfaces in the tailoring workspace

The SPA SHALL NOT provide a dedicated, standalone fit-analysis page. The Tailor workspace's Job
Match tab (`/tailor/[slug]`) is the sole surface presenting the full analysis (overall score +
verdict, the six dimensions with their rationale, the ATS requirement match, strengths, gaps,
recommendation). When the tailoring bootstrap returns an existing cached analysis, the tab renders
it immediately from that response. When the bootstrap flags a first-time cold start (no cached
analysis existed yet — see `cv-tailoring`'s "The bootstrap response flags a first-time cold start"),
the workspace SHALL NOT open a separate fit-analysis stream or block on one — the analysis is
computed as a precondition step inside the automatically-triggered autopilot run itself (see
`tailor-autopilot`'s "An autopilot run satisfies its own analysis precondition"), on the same
request/stream the workspace is already consuming for the live build. The Job Match tab SHALL show
an in-progress state until the analysis lands, then render it, without the candidate taking any
action or being sent to a separate screen.

Requests to the former standalone page path (`/match/[slug]`) and its earlier legacy alias
(`/jobs/[slug]/fit`) SHALL redirect (308) to `/tailor/[slug]`.

#### Scenario: Cached analysis renders from the bootstrap response

- **WHEN** the user opens `/tailor/[slug]` for a vacancy with a fresh cached analysis
- **THEN** the Job Match tab shows the full analysis immediately, with no separate fetch or stream

#### Scenario: A cold-start analysis populates the tab once it lands

- **WHEN** the user opens `/tailor/[slug]` for a vacancy with no cached analysis, triggering a
  cold-start background sequence
- **THEN** the Job Match tab shows an in-progress state, and renders the full analysis once the
  background sequence's analysis step completes, without the workspace blocking on it or the
  candidate triggering anything

#### Scenario: Old links redirect into the workspace

- **WHEN** a request is made to `/match/[slug]` or `/jobs/[slug]/fit`
- **THEN** the system responds with a 308 redirect to `/tailor/[slug]`
