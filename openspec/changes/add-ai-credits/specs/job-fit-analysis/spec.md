## MODIFIED Requirements

### Requirement: Per-user monthly fit-analysis quota

The system SHALL charge **1 point** (configurable, `CREDITS_COST_MATCH`) per AI
fit analysis against the caller's unified points balance (see the `ai-credits`
capability), enforced BEFORE the LLM prompt-chain runs on both the synchronous
`POST /api/v1/jobs/:slug/fit` endpoint and the streaming
`GET /api/v1/jobs/:slug/fit/stream` endpoint. The system MUST pre-check that the
caller has at least the match cost remaining before invoking the LLM, and MUST
debit only on successful persistence of the analysis. The debit is idempotent by
`job_id`: only the FIRST analysis of a distinct `(user, job)` pair consumes
points; a recompute of a pair the user has already analysed, and re-running the
same job, MUST be allowed without a further debit. An analysis that fails or is
never persisted MUST NOT consume points. The limit applies to every role — there
is no staff exemption.

#### Scenario: New job with sufficient balance

- **WHEN** a user with at least the match cost remaining requests an analysis for a job they have not analysed
- **THEN** the system runs the chain, persists the result, and debits the match cost from the caller's balance

#### Scenario: New job with insufficient balance

- **WHEN** a user whose remaining points are below the match cost requests an analysis for a job they have not analysed
- **THEN** the system responds `402`, never invokes the LLM, and persists nothing

#### Scenario: Recompute is always free

- **WHEN** a user with insufficient points requests an analysis for a job they have already analysed (a recompute of an existing `(user, job)` pair)
- **THEN** the system runs the chain and does not debit points or reject on balance grounds

#### Scenario: Streaming endpoint enforces the same cost

- **WHEN** a user with insufficient points opens the SSE stream for a job they have not analysed
- **THEN** the system responds `402` before opening the event stream and never invokes the LLM

#### Scenario: Failed analysis does not consume points

- **WHEN** an analysis is attempted with sufficient balance but the LLM is unconfigured or errors, so no row is persisted
- **THEN** the user's remaining points are unchanged

### Requirement: Quota state on the read endpoint

The read endpoint `GET /api/v1/jobs/:slug/fit` SHALL return a `credits` object
carrying `remaining` (the caller's current points balance) and `resets_at` (the
date the current period's grant resets), computed without invoking the LLM, so
the client can display the balance and pre-block a new-job analysis when the
remaining points are below the match cost.

#### Scenario: Credits reported on read

- **WHEN** a signed-in caller reads `GET /api/v1/jobs/:slug/fit`
- **THEN** the response includes `credits` with `remaining` and `resets_at`, and no LLM call is made

#### Scenario: Remaining never negative

- **WHEN** a caller has exhausted their points for the period
- **THEN** `remaining` is reported as `0`, not a negative number
