## Context

The AI fit analysis (`internal/jobfit`, three-stage prompt-chain) is triggered by two compute endpoints — `POST /api/v1/jobs/:slug/fit` (`PostJobFit`) and the SSE `GET /api/v1/jobs/:slug/fit/stream` (`StreamJobFit`) — both persisting the result via `cacheAnalysis` → `UpsertUserJobAnalysis`. `GET /api/v1/jobs/:slug/fit` (`GetJobFit`) reads the cache and never calls the LLM. Results are cached one row per `(user, job)` in `user_job_analysis`, keyed by the composite PK, with a `created_at` column that today is re-bumped to `now()` on every recompute.

There is no per-user rate limit, so one account can run the chain indefinitely and drain the shared LLM budget.

## Goals / Non-Goals

**Goals:**

- Cap each user at 10 *distinct-job* AI analyses per rolling 30-day window.
- Enforce the cap before any LLM call, on both compute paths.
- Keep recompute of an already-analysed job free.
- Surface usage (`used`/`limit`/`remaining`) on the read endpoint for the UI.
- No schema migration.

**Non-Goals:**

- A hard, race-free reservation (see Risks — the limit is intentionally soft).
- Per-plan / configurable-per-user limits, billing, or an env-tunable limit (noted as future seams).
- Counting the deterministic `jobmatch` bar — that stays free and instant.
- Any change to the analysis payload, staleness stamps, or the LLM chain itself.

## Decisions

### Reuse `user_job_analysis` as the usage ledger (no new table)

Under "distinct jobs, rolling 30 days", the quota is exactly *the number of distinct jobs a user first analysed in the last 30 days* — which is the count of that user's `user_job_analysis` rows whose first-analysis timestamp is within the window. The table already holds one row per `(user, job)`, written only on a successful compute. So it is the ledger; no separate events table is warranted (YAGNI).

Required change: `UpsertUserJobAnalysis` currently sets `created_at = now()` in its `ON CONFLICT DO UPDATE`, which would make a recompute look like a fresh analysis and mis-count the window. Drop `created_at` from the `SET` list so it keeps the *first*-analysis time; the analysis blob, model, and both staleness stamps still update. `created_at` is not read by any current logic (only returned), so preserving it is safe and is the more correct meaning for a "first analysed" marker.

New query: `CountRecentUserJobAnalyses(user_id, since) :one` → `bigint`, counting rows with `created_at >= since`.

### Enforce before the LLM call; recompute detected by row existence

In both `PostJobFit` and `StreamJobFit`, before building/running the chain:

1. Look up the existing `(user, job)` row (`GetUserJobAnalysis`). A hit ⇒ recompute ⇒ **allow** (skip the quota check entirely).
2. A miss (`pgx.ErrNoRows`) ⇒ new job ⇒ `used := CountRecentUserJobAnalyses(user, now-30d)`; if `used >= limit` ⇒ reject.

Rejection is `fiber.NewError(fiber.StatusTooManyRequests, msg)` rendered by the central `ErrorHandler`. For the SSE handler the check runs while the fiber ctx is still valid (before `SetBodyStreamWriter`), so it can return a real `429` before the stream body opens.

The limit and window are package constants in `handler` (`fitAnalysisLimit = 10`, `fitAnalysisWindow = 30 * 24h`). The `jobFitStore` interface gains `CountRecentUserJobAnalyses`; the DB-less handler-test fake implements it.

### Quota on the read response

`jobFitResponse` gains a `Quota` field `{ used, limit, remaining }` (`remaining = max(0, limit-used)`), populated in `GetJobFit` from `CountRecentUserJobAnalyses`. This read stays LLM-free. The frontend `JobFitResponse` type gains the matching `quota` object; the fit sidebar and page render "N/10" and block a *new*-job compute (never a recompute) when `remaining == 0`. The server-side check remains the authority; the client block is UX only.

## Risks / Trade-offs

- **Soft limit (accepted):** the check-then-compute pair is not atomic across the seconds-long LLM call, so several concurrent *new-job* requests at the boundary can each pass the check and overshoot 10. Acceptable for an MVP budget guard — the persisted rows are the source of truth and self-correct; a race-free cap would need a reservation row and is out of scope.
- **Recompute is free by design:** a user can refresh a stale verdict for the same 10 jobs without limit, which does spend LLM budget. This matches the agreed product intent ("10 distinct jobs / month") and is the friendlier reading; if abuse appears, tightening to "every LLM call counts" is a one-line predicate change.
- **`created_at` semantics shift:** existing prod rows carry a *last-recompute* `created_at` (possibly recent), so just after deploy some older analyses may transiently count toward the window. One-time and self-healing within 30 days; not worth a data backfill.
- **Cascade deletes free slots:** deleting a job or user removes its rows and frees quota — correct and harmless.
