## REMOVED Requirements

### Requirement: Dedicated fit analysis page

**Reason**: The standalone fit-analysis page is removed; the Tailor workspace's Job Match tab
(`/tailor/[slug]`) becomes the sole surface for viewing and triggering the analysis, so a candidate
never leaves the tailoring workspace to produce a first analysis.

**Migration**: Requests to the former page path (`/match/[slug]`, and its earlier legacy alias
`/jobs/[slug]/fit`) redirect (308) to `/tailor/[slug]`. See the new "Fit analysis surfaces in the
tailoring workspace" requirement below for the replacement behavior.

## ADDED Requirements

### Requirement: Fit analysis surfaces in the tailoring workspace

The SPA SHALL NOT provide a dedicated, standalone fit-analysis page. The Tailor workspace's Job
Match tab (`/tailor/[slug]`) is the sole surface presenting the full analysis (overall score +
verdict, the six dimensions with their rationale, the ATS requirement match, strengths, gaps,
recommendation). When the tailoring bootstrap returns an existing cached analysis, the tab renders
it immediately from that response. When the bootstrap reports no cached analysis exists (`409 "run
the fit analysis first"`), the workspace SHALL open the fit-analysis stream itself and render the
stage progress, the thinking panel, and each section progressively as it resolves, retrying the
bootstrap once the stream completes — the candidate is never sent to a separate page to produce a
first analysis.

Requests to the former standalone page path (`/match/[slug]`) and its earlier legacy alias
(`/jobs/[slug]/fit`) SHALL redirect (308) to `/tailor/[slug]`.

#### Scenario: Cached analysis renders from the bootstrap response

- **WHEN** the user opens `/tailor/[slug]` for a vacancy with a fresh cached analysis
- **THEN** the Job Match tab shows the full analysis immediately, with no separate fetch or stream

#### Scenario: No cached analysis streams inline

- **WHEN** the user opens `/tailor/[slug]` for a vacancy with no cached analysis
- **THEN** the workspace opens the fit-analysis stream in place, shows the stage stepper and fills
  the overall/dimensions/requirements/verdict sections as each stage resolves, then retries the
  tailoring bootstrap so the workspace loads with the completed analysis

#### Scenario: Old links redirect into the workspace

- **WHEN** a request is made to `/match/[slug]` or `/jobs/[slug]/fit`
- **THEN** the system responds with a 308 redirect to `/tailor/[slug]`

## MODIFIED Requirements

### Requirement: Sidebar reduced to a summary linking to the page

The Profile-match sidebar block SHALL show only a short fit summary — the overall percentage, the
verdict label, and the single most important gap — with a link to the dedicated page. It MUST NOT
run the analysis inline. When no analysis is cached it MUST show an action that navigates to the
page (which starts the stream) rather than computing in the sidebar.

The "dedicated page" this links to is the Tailor workspace (`/tailor/[slug]`), not a separate
fit-analysis page — see "Fit analysis surfaces in the tailoring workspace".

#### Scenario: Sidebar summarizes and links

- **WHEN** a cached analysis exists and the user views the job
- **THEN** the sidebar shows the overall %, the verdict, and the top gap, with a link to `/tailor/[slug]`

#### Scenario: Sidebar with no analysis links to the workspace

- **WHEN** no analysis is cached
- **THEN** the sidebar shows an action that navigates to `/tailor/[slug]` instead of computing inline
