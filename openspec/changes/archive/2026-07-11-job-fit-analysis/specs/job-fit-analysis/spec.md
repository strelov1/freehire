## ADDED Requirements

### Requirement: On-demand LLM fit analysis

The system SHALL provide an authenticated `POST /api/v1/jobs/:slug/fit` endpoint that runs a fixed
three-stage LLM prompt-chain comparing the job (title + description), the company context, and the
caller's stored CV text, and returns a structured fit analysis. The chain MUST be a deterministic,
server-orchestrated sequence of typed calls — not an autonomous, self-directing agent. The analysis
MUST be scoped to the calling user and the job addressed by `:slug`.

#### Scenario: Signed-in user with a CV requests analysis

- **WHEN** a signed-in user with a stored CV and a saved profile POSTs to `/api/v1/jobs/:slug/fit` for an open job
- **THEN** the system runs the three-stage chain and responds `200` with `{ "data": { "has_cv": true, "analysis": <verdict> } }`

#### Scenario: Unknown job slug

- **WHEN** the caller POSTs to `/api/v1/jobs/:slug/fit` for a slug that does not exist
- **THEN** the system responds `404`

#### Scenario: Unauthenticated caller

- **WHEN** an unauthenticated request hits the fit endpoint
- **THEN** the system responds `401` and never invokes the LLM

### Requirement: Five-dimension scored verdict

The analysis payload SHALL contain five dimensions — Title & role alignment, Experience
relevance, Seniority fit, Skills coverage, and Company & role context — each with an integer
score clamped to 0–100, plus a weighted `overall_score`, a `verdict` label drawn from the
controlled set {Strong Fit, Good Fit, Moderate Fit, Weak Fit, Poor Fit}, a `strengths` array,
a `gaps` array, and a single `recommendation` string. All model output MUST be sanitized: scores
clamped, the verdict coerced to the controlled set, and free-text fields trimmed and length/count
bounded before it is persisted or served.

#### Scenario: Out-of-range or invalid model output

- **WHEN** the LLM returns a dimension score above 100 or a verdict outside the controlled set
- **THEN** the score is clamped to 0–100 and the verdict is derived from `overall_score`, so no out-of-vocabulary value is ever persisted or served

#### Scenario: Overall score is the weighted dimensions

- **WHEN** the five dimension scores are known
- **THEN** `overall_score` equals the deterministic weighted average of the dimensions, computed server-side rather than trusting the model's own overall

### Requirement: Deterministic match as grounding anchor

The LLM prompt SHALL include the deterministic skills match (exact/adjacent/missing slugs from
`internal/jobmatch`) so the model explains and augments it rather than recomputing skills from
scratch. The Skills coverage dimension MUST be consistent with the deterministic coverage.

#### Scenario: Anchor passed into the prompt

- **WHEN** the fit analysis is computed for a job with a non-empty skills list
- **THEN** the prompt carries the exact/adjacent/missing classification and the coverage percent from the deterministic match

### Requirement: ATS-style requirement match (Stage 1)

The first stage SHALL extract the vacancy's explicit requirements together with its role-title and
seniority signals, and classify each requirement against the CV text as one of `covered`,
`synonym-only`, `missing-but-have`, or `missing-gap`, carrying a required/preferred priority. This
requirement-match table MUST be included in the served analysis and MUST NOT fabricate a skill the
CV does not evidence — a genuine gap is reported as `missing-gap`, never hidden.

#### Scenario: Requirement present only under a synonym

- **WHEN** the vacancy requires a skill the CV evidences under a different but equivalent term
- **THEN** that requirement is classified `synonym-only`, not `missing`

#### Scenario: Genuine gap is reported honestly

- **WHEN** the vacancy requires a skill absent from the CV with no close equivalent held
- **THEN** that requirement is classified `missing-gap` and is never presented as covered

### Requirement: Adversarial audit (Stage 3)

The final stage SHALL challenge the recruiter verdict — flagging inflated dimension scores,
strengths not supported by the CV evidence, and gaps that were glossed over — and return a
corrected verdict that the served analysis is built from. If the audit stage fails or does not
parse, the system MUST fall back to the un-audited recruiter verdict rather than error the request.

#### Scenario: Audit prunes an unsupported strength

- **WHEN** the recruiter stage lists a strength the CV does not actually evidence
- **THEN** the audit removes or downgrades it and the served analysis reflects the corrected verdict

#### Scenario: Audit stage fails

- **WHEN** the adversarial audit call fails or returns unparseable output
- **THEN** the system serves the recruiter-stage verdict and still responds `200`

### Requirement: Per-(user, job) cache with staleness invalidation

The system SHALL cache each analysis per `(user_id, job_id)`, stamped with the CV's upload time,
the job's `content_hash`, and the model that produced it at analysis time. `GET /api/v1/jobs/:slug/fit`
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
running the LLM.

#### Scenario: LLM unconfigured

- **WHEN** a user POSTs the fit endpoint while the LLM is not configured
- **THEN** the system responds `200` with no analysis and does not persist a cache row

#### Scenario: Caller has no stored CV

- **WHEN** a user without a stored CV requests the fit
- **THEN** the system responds `200` with `has_cv: false` and no analysis, and does not invoke the LLM

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
