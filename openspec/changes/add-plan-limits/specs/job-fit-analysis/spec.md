## ADDED Requirements

### Requirement: A fit analysis draws on the daily analysis allowance

The system SHALL consume one job-fit-analysis allowance before the prompt chain runs, on
both `POST /api/v1/jobs/:slug/fit` and the streaming `GET /api/v1/jobs/:slug/fit/stream`.
Only a NEW `(user, job)` pair SHALL consume allowance: recomputing a pair already
analysed MUST be allowed regardless of what remains, because the cost of a recompute is
already paid for and refusing it would punish the user for looking again. An analysis
that produces nothing usable SHALL return its allowance, so a failed run leaves the user
where they started. The limit applies to every role — there is no staff exemption.

#### Scenario: New job within the allowance

- **WHEN** a user with fit-analysis allowance remaining requests an analysis for a job
  they have not analysed
- **THEN** the chain runs, the result is persisted, and one allowance is consumed

#### Scenario: New job with the allowance spent

- **WHEN** a user who has spent today's fit-analysis allowance requests an analysis for a
  job they have not analysed
- **THEN** the system responds `402`, never invokes the LLM, and persists nothing

#### Scenario: Recompute is always free

- **WHEN** a user with no allowance remaining requests an analysis for a job they have
  already analysed
- **THEN** the chain runs and the request is not refused on allowance grounds

#### Scenario: Streaming endpoint refuses with a real status

- **WHEN** a user with no allowance remaining opens the SSE stream for a job they have
  not analysed
- **THEN** the system responds `402` before opening the event stream and never invokes
  the LLM

#### Scenario: Failed analysis returns the allowance

- **WHEN** an analysis is attempted within allowance but the LLM is unconfigured or
  errors, so no row is persisted
- **THEN** the consumed allowance is returned and the user may retry

### Requirement: Allowance state on the read endpoint

The read endpoint `GET /api/v1/jobs/:slug/fit` SHALL report the caller's fit-analysis
allowance for the current day — what has been used, what is allowed, and when it resets —
without invoking the LLM, so the client can display it and pre-block a new-job analysis
that would be refused. For a pro-plan caller it SHALL report the feature as unlimited
rather than reporting a remaining number.

#### Scenario: Allowance reported on read

- **WHEN** a signed-in free-plan caller reads `GET /api/v1/jobs/:slug/fit`
- **THEN** the response reports today's used, allowed and reset instant, and no LLM call
  is made

#### Scenario: Used never exceeds allowed in the report

- **WHEN** a caller has consumed their whole allowance
- **THEN** the report shows nothing remaining rather than a negative number

#### Scenario: Pro caller reads the allowance

- **WHEN** a signed-in pro-plan caller reads the endpoint
- **THEN** the response reports the feature as unlimited and no remaining count

## REMOVED Requirements

### Requirement: Per-user monthly fit-analysis quota

**Reason**: Superseded by the daily fit-analysis allowance. The rolling-30-day, 10-analysis
quota answering `429` was already replaced in code by a credit debit answering `402`; this
spec was the last place it survived. A rolling window also cannot express "today", which is
the unit the plan is now sold in.

**Migration**: The rule it carried — only a new `(user, job)` pair is charged, a recompute
is free, a failed run charges nothing, no staff exemption — is preserved verbatim in
"A fit analysis draws on the daily analysis allowance". Callers handling `429` from these
two endpoints must handle `402` instead.

### Requirement: Quota state on the read endpoint

**Reason**: The `quota` object reported `used`, `limit` and `remaining` over a rolling
30-day window with a hard-coded limit of 10. Neither the window nor the limit survives.

**Migration**: Replaced by "Allowance state on the read endpoint", which reports the same
three facts over the current UTC day plus the reset instant, and reports a pro-plan caller
as unlimited rather than as a number.
