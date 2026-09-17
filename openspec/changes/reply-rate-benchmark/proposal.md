## Why

Candidates tracking applications on freehire have no way to tell whether their own outcomes are
typical. The catalogue already computes an objective, mail-observed employer response rate per
company (`insights_company_response`, gated at ten tracked applications — see
`company-hiring-signal`), but that ledger is never surfaced to the one person it would help most:
the candidate looking at their own pipeline.

A stage-based "applied → interview" conversion was considered first and measured against
production data before being ruled out: only 12 of 1350 `applied` events ever had a `stage_set`
event reaching interview or beyond (~0.9%), against 97 of 287 observable applications receiving an
`employer_reply` (~34%) for the same population. The stage figure is dominated by candidates not
bothering to update their tracker, not by market outcomes — it would report tracker diligence as
if it were a market signal. The reply-rate figure is mail-observed, not self-reported, and already
has a validated, sample-gated computation. This change surfaces it, rather than building a new,
weaker metric.

## What Changes

- Extend the existing per-company response-rate rollup (`cmd/rollup-company`) to also publish one
  **global** response rate (summed across all companies), under the same ten-application sample
  gate the per-company figure already uses.
- Add a **personal** response-rate computation for the signed-in caller — same definitions
  (observable = connected mailbox, answered = live `employer_reply` event), same ten-application
  gate, computed live per request since it is scoped to one user.
- Expose personal-vs-global on `GET /api/v1/me/tracking/pipeline`, present only when both the
  caller clears the sample gate and has a connected mailbox; absent otherwise (never a misleading
  0%/estimate).
- Render the comparison in the existing Pipeline tab, alongside the current Interview Rate / Offer
  Rate donut cards — no new page, no new endpoint.
- Explicitly NOT building the stage-based "applied → interview" benchmark that motivated this
  investigation — ruled out per the measurement above.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `company-hiring-signal`: the response-rate rollup gains a global (all-companies) figure
  alongside the existing per-company one, computed from the same ledger under the same sample
  gate.
- `application-pipeline`: the pipeline endpoint and its Pipeline-tab UI gain a personal-vs-global
  reply-rate comparison, gated on the caller's own sample size and mailbox connection.

## Impact

- **Backend:** `internal/platform/db/queries/insights.sql` (global aggregate query),
  `cmd/rollup-company` (compute and store the global scalar in the same transaction as the
  per-company rebuild), a new query scoped to one user mirroring
  `RebuildInsightsCompanyResponse`'s observable/answered CTEs, and the pipeline handler in
  `internal/api/handler` (name TBD in design.md) for the live per-user computation and gating.
- **Frontend:** the Pipeline tab component in `web/` gains a comparison card, rendered only when
  the field is present in the response.
- **Data:** no new migration for per-company data; the global figure needs at most one small
  scalar store — precompute-vs-read-live is decided in design.md. No change to
  `application_events` itself.
