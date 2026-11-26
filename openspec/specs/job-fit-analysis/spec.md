# job-fit-analysis Specification

## Purpose
TBD - created by archiving change job-fit-analysis. Update Purpose after archive.
## Requirements
### Requirement: On-demand LLM fit analysis

The system SHALL provide an authenticated `POST /api/v1/jobs/:slug/match-analysis` endpoint that runs a fixed
three-stage LLM prompt-chain comparing the job (title + description), the company context, and the
caller's stored CV text, and returns a structured fit analysis. The chain MUST be a deterministic,
server-orchestrated sequence of typed calls — not an autonomous, self-directing agent. The analysis
MUST be scoped to the calling user and the job addressed by `:slug`.

#### Scenario: Signed-in user with a CV requests analysis

- **WHEN** a signed-in user with a stored CV and a saved profile POSTs to `/api/v1/jobs/:slug/match-analysis` for an open job
- **THEN** the system runs the three-stage chain and responds `200` with `{ "data": { "has_cv": true, "analysis": <verdict> } }`

#### Scenario: Unknown job slug

- **WHEN** the caller POSTs to `/api/v1/jobs/:slug/match-analysis` for a slug that does not exist
- **THEN** the system responds `404`

#### Scenario: Unauthenticated caller

- **WHEN** an unauthenticated request hits the fit endpoint
- **THEN** the system responds `401` and never invokes the LLM

### Requirement: Deterministic match as grounding anchor

The LLM prompt SHALL include the deterministic skills match (exact/adjacent/missing slugs from
`internal/candidate/jobmatch`) so the model explains and augments it rather than recomputing skills from
scratch. The Skills coverage dimension MUST be consistent with the deterministic coverage.

#### Scenario: Anchor passed into the prompt

- **WHEN** the fit analysis is computed for a job with a non-empty skills list
- **THEN** the prompt carries the exact/adjacent/missing classification and the coverage percent from the deterministic match

### Requirement: Per-(user, job) cache with staleness invalidation

The system SHALL cache each analysis per `(user_id, job_id)`, stamped with the CV's upload time,
the job's `content_hash`, and the model that produced it at analysis time. `GET /api/v1/jobs/:slug/match-analysis`
MUST return a cached analysis only when all three stamps still equal the current CV upload time, job
`content_hash`, and model; when any differs it MUST report the cached analysis as stale rather than
serving it as current. A `content_hash` absent on both the stored stamp and the live job (a non-board
job that is never re-crawled) counts as unchanged, so it does not force an endless recompute.

#### Scenario: Fresh cache hit

- **WHEN** a user GETs the fit for a job they already analyzed, and neither their CV, the job, nor the model has changed since
- **THEN** the system returns the cached analysis with `stale: false` and makes no LLM call

#### Scenario: Model upgraded since analysis

- **WHEN** a user GETs the fit for a job analyzed under a previous `LLM_MODEL`
- **THEN** the cached analysis is reported with `stale: true` so the improved model can re-analyze on request

#### Scenario: CV changed since analysis

- **WHEN** a user GETs the fit after re-uploading their CV
- **THEN** the cached analysis is reported with `stale: true` so the SPA can offer a recompute, and it is not served as current

#### Scenario: Job re-ingested with changed content

- **WHEN** a user GETs the fit for a job whose `content_hash` changed since the analysis
- **THEN** the cached analysis is reported with `stale: true`

#### Scenario: No analysis yet

- **WHEN** a user GETs the fit for a job they have never analyzed
- **THEN** the system responds `200` with `has_cv` reflecting CV presence and a null analysis (no LLM call)

### Requirement: Best-effort degradation

The feature SHALL degrade gracefully: when the LLM is unconfigured or the call fails, the endpoint
MUST NOT error the request and MUST leave the deterministic profile-match unaffected. When the
caller has no stored CV, the response MUST indicate `has_cv: false` and prompt an upload instead of
running the LLM. When the caller has a CV but no current structured résumé, the chain MUST degrade
to no analysis (the structured résumé is the fit input, and it is what carries the de-identified
signal). Fit analysis SHALL NOT depend on a PII detector; it consumes the existing
contact-free derived projection.

#### Scenario: LLM unconfigured

- **WHEN** a user POSTs the fit endpoint while the LLM is not configured
- **THEN** the system responds `200` with no analysis and does not persist a cache row

#### Scenario: Caller has no stored CV

- **WHEN** a user without a stored CV requests the fit
- **THEN** the system responds `200` with `has_cv: false` and no analysis, and does not invoke the LLM

#### Scenario: No structured résumé degrades to no analysis

- **WHEN** a user POSTs the fit endpoint with a CV but no current structured résumé
- **THEN** the system responds `200` with no analysis and does not persist a cache row

### Requirement: Profile-match UI shows the AI analysis on demand

The Profile match block SHALL keep the fast deterministic bar on top and render the LLM fit
analysis in an expandable section driven by the fit endpoint. The AI analysis MUST be shown only
after an explicit user action (it is not fetched automatically on page open), and a stale cached
analysis MUST offer a recompute.

#### Scenario: User expands the AI analysis

- **WHEN** a signed-in profiled user with a CV clicks the "AI fit analysis" action on a job page
- **THEN** the block requests the analysis (cached or freshly computed) and renders the five-dimension verdict with the overall score and label

#### Scenario: Stale analysis offers recompute

- **WHEN** the expanded section loads a cached analysis reported as stale
- **THEN** the block surfaces that it is outdated and offers a recompute action rather than silently showing stale scores

### Requirement: Location & work-mode fit dimension

The fit analysis SHALL score a sixth dimension, Location & work-mode fit, judging whether the
candidate can actually take the role given the job's geography (work mode, remote flag, regions,
countries, free-text location) and the caller's profile location preferences (accepted work modes,
remote reach, current base, relocation willingness). The weighted `overall_score` MUST include this
dimension with all six weights summing to 100, and title alignment and experience relevance MUST
remain the two heaviest. When the profile carries no location preferences, the dimension MUST still
resolve (scored on the job geography alone) rather than erroring.

#### Scenario: Onsite job far from a remote-only candidate

- **WHEN** the job is onsite in a country outside the candidate's base and relocation set, and the candidate prefers remote only
- **THEN** the Location & work-mode fit dimension scores low and the mismatch surfaces in the gaps/recommendation

#### Scenario: Remote job within the candidate's remote reach

- **WHEN** the job is remote and its region is within the candidate's declared remote reach
- **THEN** the Location & work-mode fit dimension scores high

#### Scenario: Profile with no location preferences

- **WHEN** a candidate with no location preferences set requests the analysis
- **THEN** the analysis still returns six dimensions (location scored on the job geography alone), never an error

### Requirement: Location signals in the prompt-chain

The prompt-chain SHALL carry the job's geography and the caller's location preferences into the
recruiter and audit stages so their reasoning and the `gaps`/`recommendation` reflect geographic and
work-mode fit, not only skills and title.

#### Scenario: Location gap explained

- **WHEN** the location dimension is a genuine mismatch
- **THEN** the recommendation names the geographic/work-mode reason rather than leaving it implicit

### Requirement: Fuller fit-result presentation

The SPA SHALL present the fit result in fuller detail: each dimension's score and its one-line
rationale visible (not only the bar), the six dimensions including Location & work-mode fit, the
ATS requirement match, and the strengths/gaps/recommendation, in a clear visual hierarchy. The
`recommendation` (the verdict card) MUST render as multi-paragraph prose through the same
untrusted-model markup sanitizer used elsewhere on the site, so paragraph breaks and ordinary
emphasis survive without collapsing into a single block or admitting request-triggering markup. In
the narrow stacked panel the verdict card MUST use a body size that keeps a three-paragraph verdict
readable rather than a wall of large type.

#### Scenario: Dimension rationale is visible

- **WHEN** the analysis renders
- **THEN** each dimension shows its score and its rationale comment, so the user sees why, not just a number

#### Scenario: Multi-paragraph recommendation keeps its breaks

- **WHEN** the analysis carries a recommendation with two or three paragraphs separated by blank lines
- **THEN** the verdict card renders those as separate paragraphs, not one collapsed run of text

#### Scenario: Hostile markup in the recommendation never fires a request

- **WHEN** a recommendation contains a markdown image or a non-http link
- **THEN** the rendered verdict card contains no request-triggering element for that payload

### Requirement: Live-streaming fit computation

The system SHALL provide an authenticated Server-Sent Events endpoint
`GET /api/v1/jobs/:slug/match-analysis/stream` that runs the three-stage chain and emits, in order, events for
each stage's start and completion, the model's thinking tokens when available, each stage's
structured result (requirements after Stage 1, dimensions after Stage 2, the final verdict after
Stage 3), and a terminal completion event carrying the full analysis. On successful completion it
MUST cache the analysis exactly as the synchronous compute does (same per-(user, job) row and
staleness stamps). The synchronous `POST /jobs/:slug/match-analysis` MUST remain for non-browser clients, and
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

- **WHEN** the user opens `/tailor/[slug]` for a vacancy with no cached analysis, triggering the
  automatic cold-start autopilot run
- **THEN** the Job Match tab shows an in-progress state, and renders the full analysis once that
  run's inline analysis step completes, without the workspace blocking on it or the candidate
  triggering anything

#### Scenario: Old links redirect into the workspace

- **WHEN** a request is made to `/match/[slug]` or `/jobs/[slug]/fit`
- **THEN** the system responds with a 308 redirect to `/tailor/[slug]`

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

### Requirement: Fit analysis is a paid, daily-metered feature

Running a fit analysis on the synchronous `POST /api/v1/jobs/:slug/match-analysis` endpoint or the streaming `GET /api/v1/jobs/:slug/match-analysis/stream` endpoint SHALL require a live Pro or Ultra plan and SHALL draw on the caller's daily `match` allowance (`plan.FeatureFit`), both checked BEFORE the LLM prompt-chain runs. The day is the UTC calendar day; the allowance resets at midnight UTC. Both paid plans are unlimited for a person: the daily figure in `plan.DefaultConfig` (60 on Pro, 180 on Ultra) is a fair-use guard against automation, never shown as a plan ceiling. Only the FIRST analysis of a distinct `(user, job)` pair is charged: a recompute of a pair the caller has already analysed is free. The charge is taken before the chain starts and given back when the run produces no analysis. A metering fault lets the run through uncharged (fail open); a failure to read the caller's plan refuses it with `503` (fail closed).

#### Scenario: Caller without a paid plan

- **WHEN** a caller on no paid plan requests a new analysis on either endpoint
- **THEN** the system responds `402`, never invokes the LLM, and persists nothing

#### Scenario: New job within the allowance

- **WHEN** a paid caller below today's fair-use guard requests an analysis for a job they have not analysed
- **THEN** the system runs the chain, persists the result, and today's `match` usage grows by one

#### Scenario: Fair-use guard reached

- **WHEN** a paid caller who has reached today's fair-use guard requests an analysis for a job they have not analysed
- **THEN** the system responds `402` with the `allowance` and its `resets_at`, never invokes the LLM, and persists nothing

#### Scenario: Recompute is free

- **WHEN** a paid caller at the guard requests an analysis for a job they have already analysed
- **THEN** the system runs the chain and does not refuse it on allowance grounds

#### Scenario: Streaming endpoint enforces the same gate

- **WHEN** a caller who would be refused opens the SSE stream for a job they have not analysed
- **THEN** the system responds `402` before opening the event stream and never invokes the LLM

#### Scenario: Failed analysis gives the charge back

- **WHEN** a new-job analysis is charged but the LLM is unconfigured or errors, so no analysis is produced
- **THEN** the charge is released and today's usage is unchanged

### Requirement: Allowance state on the read endpoint

The read endpoint `GET /api/v1/jobs/:slug/match-analysis` SHALL return the caller's standing on today's fit-analysis allowance as `allowance`, and on the tailoring-session allowance as `tailor_allowance`, without invoking the LLM, so the client can show usage and pre-block a run the server would refuse. Each carries `feature`, `used`, `unlimited`, `enforced`, `resets_at`, and `limit` only when the plan has a real ceiling (an unlimited plan's fair-use guard is never exposed as a limit). The standing is best-effort: when it cannot be read the field is omitted and the read still succeeds.

#### Scenario: Allowance reported on read

- **WHEN** a signed-in paid caller reads `GET /api/v1/jobs/:slug/match-analysis`
- **THEN** the response includes `allowance` with `feature` `match`, today's `used`, `unlimited` true, and the next midnight UTC as `resets_at`, and no LLM call is made

#### Scenario: Meter unavailable

- **WHEN** the plan meter cannot be read during a read
- **THEN** the response omits `allowance` and still returns the cached analysis

### Requirement: Structured résumé augments the fit input

The fit prompt-chain SHALL additionally consume the caller's current structured résumé, when present, as pre-normalized candidate context supplied beside the existing CV text — never replacing it. The raw CV text remains the ground truth for requirement matching; the structured résumé is additive signal. When the caller has no current structured résumé (unconfigured LLM, not yet extracted, or stale), the chain MUST run exactly as it does today on the CV text alone, with no error.

#### Scenario: Structured résumé is provided to the chain when present

- **WHEN** the caller has a current structured résumé and requests a fit analysis
- **THEN** the fit input includes the structured résumé as pre-normalized context in addition to the CV text

#### Scenario: Analysis degrades to text-only when the structure is absent

- **WHEN** the caller has a CV but no current structured résumé
- **THEN** the fit analysis runs on the CV text alone, exactly as before, with no error

### Requirement: Hard-constraint blockers ground the prompt chain

The prompt chain SHALL include the deterministic hard-constraint blockers as known, already-established constraints so the model explains and respects them rather than re-deriving degree, years, license, or work-authorization requirements. The served analysis MUST expose the blockers alongside the verdict.

#### Scenario: Blockers passed into the prompt and surfaced

- **WHEN** the fit analysis is computed for a caller with an unmet hard constraint
- **THEN** the prompt carries the blocker as a known constraint and the served analysis exposes it beside the dimensions

### Requirement: Fit is scored from the de-identified structured résumé

The fit chain SHALL score the candidate from the **contact-free projection** of the structured
résumé — the typed seam defined by "The contact-free projection is the one typed seam to a
model" — and SHALL NOT send the raw CV text to the model. The chain SHALL take that projection as
a typed value rather than a serialization it re-projects, so the guarantee is carried by the
input's type rather than by each producer's discipline. The structured résumé is produced once at
upload, so the fit analysis carries no direct identifier to the provider by construction — no
per-analysis masking is performed.

#### Scenario: Provider never sees CV PII

- **WHEN** a fit analysis runs for a user whose CV contains name/email/phone/links
- **THEN** the text sent to the model is the contact-free projection of the structured résumé, and the raw CV is not sent

#### Scenario: No structured résumé, no analysis

- **WHEN** a user has a stored CV but no current structured résumé (extraction absent or stale)
- **THEN** the fit analysis does not run and the endpoint responds `200` with no analysis

### Requirement: Hidden signals from posting wording

The fit analysis SHALL include a `hidden_signals` field: an array of up to 5 objects, each
carrying a verbatim `quote` excerpted from the job description and a short `insight`
interpreting what that wording implies about pace, ownership expectations, team stage, or
culture. The signals MUST be extracted as part of the existing Stage 1 (Extract & Match) LLM
call — no additional model call or stage. Both `quote` and `insight` MUST be non-empty,
length-bounded, and drawn only from the job description text; the field MUST NOT require or
depend on the candidate's résumé/CV content, even though the analysis as a whole (and therefore
this field) is only produced for a candidate with banked/structured experience.

#### Scenario: Posting wording implies pace and ownership expectations

- **WHEN** a job description repeatedly uses phrasing like "thrive in ambiguity" and "high
  ownership, fast-paced"
- **THEN** the served analysis's `hidden_signals` includes an entry quoting that wording with an
  insight naming the implied self-driven/fast-pace expectation

#### Scenario: Generic posting yields no signals

- **WHEN** a job description is short or carries no distinctive wording to interpret
- **THEN** `hidden_signals` is an empty array, and the analysis is served normally with no error
  and no fabricated signal

#### Scenario: Malformed model output is dropped, not coerced

- **WHEN** the model returns a signal entry with a blank `quote` or a blank `insight`
- **THEN** that entry is dropped from the served `hidden_signals` rather than served with a
  placeholder value

#### Scenario: Signals are cached with the rest of the analysis

- **WHEN** an analysis with `hidden_signals` is served from cache on a subsequent request for the
  same (user, job) with unchanged CV upload time, job `content_hash`, and model
- **THEN** the same `hidden_signals` are served without a new LLM call, under the analysis's
  existing cache stamp

### Requirement: Recommendation has a stated length contract

The recruiter and audit stages SHALL instruct the model that `recommendation` is two or three
short prose paragraphs of hiring judgement — not a requirement-by-requirement recap of statuses the
page already shows — with no headings and no lists. The server's sanitizer length bound on
`recommendation` MUST sit well clear of that budget so it acts as a safety ceiling on runaway or
hostile model text, not as a routine shear of ordinary answers. The two numbers keep distinct jobs:
the prompt governs style; the bound governs safety.

#### Scenario: A normal-length verdict is served intact

- **WHEN** the model returns a recommendation of two or three short paragraphs within the stated budget
- **THEN** the served `recommendation` is the full text, not truncated mid-word

#### Scenario: An oversized recommendation is still bounded

- **WHEN** the model returns a recommendation longer than the sanitizer's safety ceiling
- **THEN** the stored and served value is truncated to that ceiling before persistence

#### Scenario: Both stages name the same budget

- **WHEN** Stage 2 writes a recommendation and Stage 3 rewrites it
- **THEN** both stages' prompts state the same length and shape contract for the field

