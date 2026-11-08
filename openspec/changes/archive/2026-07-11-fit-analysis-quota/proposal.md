## Why

The on-demand "AI fit analysis" runs a three-stage LLM prompt-chain per request — the most expensive user-triggered action in the product. Nothing currently caps how often a user can trigger it, so a single account can drain the shared LLM budget by analysing job after job (or repeatedly recomputing). We need a per-user ceiling that protects the budget while staying invisible to normal use.

## What Changes

- Cap each user at **10 AI fit analyses per rolling 30-day window**.
- Only the **first** analysis of a unique job consumes quota; recompute of an already-analysed `(user, job)` and re-running the same job are **free** (they cost the same LLM budget, but the product intent is "10 distinct jobs analysed per month", and letting users refresh a stale verdict for free is the friendlier reading).
- Enforce the cap **before** the LLM call in both compute paths — `POST /jobs/:slug/fit` and the SSE `GET /jobs/:slug/fit/stream` — returning **429** when a *new* job would exceed the limit.
- Reuse `user_job_analysis` as the usage ledger (no new table, no migration): stop re-bumping `created_at` on conflict so it records the *first*-analysis time, and add a windowed count query.
- `GET /jobs/:slug/fit` returns a `quota { used, limit, remaining }` object so the UI can show usage and pre-block a new-job analysis.
- Frontend surfaces "N/10 used" in the fit sidebar and page, and blocks a *new*-job analysis (not a recompute) when `remaining == 0`.
- The limit applies to **all roles** (no staff exemption).

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities

- `job-fit-analysis`: adds a per-user monthly quota (10 unique-job analyses / rolling 30 days) enforced before the LLM call on both the sync and streaming compute endpoints, and exposes quota state on the read endpoint.

## Impact

- **Code**: `internal/handler/job_fit.go`, `internal/handler/job_fit_stream.go` (enforcement + quota in the GET response); `internal/db/queries/user_job_analysis.sql` (preserve `created_at`, new `CountRecentUserJobAnalyses`) + regenerated `internal/db`.
- **API**: `GET /jobs/:slug/fit` response gains `quota`; `POST /jobs/:slug/fit` and the SSE stream can now return `429`.
- **Frontend** (`web/`): `JobFitResponse` type gains `quota`; `JobFitAnalysis.svelte` and `jobs/[slug]/fit/+page.svelte` render usage and block over-limit new analyses.
- **No schema migration** — `user_job_analysis` already exists; only a query behaviour change.
- **Trade-off**: the limit is *soft* — the check-then-compute pair is not atomic across the seconds-long LLM call, so concurrent new-job requests at the boundary can slightly overshoot 10. Accepted for MVP; the persisted rows remain the source of truth.
