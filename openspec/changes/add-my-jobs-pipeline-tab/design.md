## Context

`/my/jobs` (SvelteKit SPA) already has Board and History tabs over the per-user interaction data in `user_jobs` (`internal/handler/me_jobs.go` → `GET /api/v1/me/jobs`). Each `user_jobs` row holds a single **current** `stage` from the controlled vocabulary `applied, screening, responded, interview, offer, accepted, rejected, withdrawn` (`internal/userjob/stages.go`), plus `viewed_at/saved_at/applied_at`. There is **no stage transition history and no per-stage timestamps** — we know where a job is now, not the path it took. The frontend has no charting dependency (hand-built SVG only).

## Goals / Non-Goals

**Goals:**
- An at-a-glance Pipeline tab: current application distribution as a Sankey snapshot + Interview/Offer rate donuts.
- Correct aggregates over **all** of the user's applications (not a paginated page).
- Keep the stage→bucket mapping and rate definitions as a single source of truth in Go.
- Zero new dependencies; reuse the existing `/me/jobs` auth and response conventions.

**Non-Goals:**
- Historical funnel / true conversion (would need transition tracking).
- Import/Export/Template/Columns/Add-job from the reference mockup; Board redesign; date-range filters; codegen for the bucket vocabulary.

## Decisions

**1. Snapshot semantics (each application counted in exactly one current bucket).**
We only persist the current `stage`, so a true historical funnel is impossible without new data. Alternative (add a transition log) was rejected as out of scope and far larger. Consequence: terminal stages (`rejected`/`withdrawn`) erase how far a job got, so rate cards are an **honest lower bound** ("reached interview/offer now"), not historical conversion. This is acceptable and documented in the UI sub-text.

**2. Stage → 7 buckets (mapping lives in Go).**
`applied`(no further stage)→`no_answer`; `screening,responded`→`in_progress`; `interview`→`interviewing`; `offer`→`offer`; `accepted`→`accepted`; `rejected`→`rejected`; `withdrawn`→`declined`. Applications denominator = rows where `applied_at IS NOT NULL OR stage IS NOT NULL` (saved-only excluded). Rates: Interview = (interviewing+offer+accepted)/applications; Offer = (offer+accepted)/applications. A pure Go function owns this mapping (unit-testable, no DB), mirroring how the project keeps controlled vocabularies in Go.

**3. Dedicated aggregate endpoint `GET /api/v1/me/jobs/pipeline` (not extending `/me/jobs` meta).**
The tab needs only aggregates, not the job list, and aggregation must run server-side over **all** applications — the board list is paginated, so client-side grouping over a page would be wrong. A dedicated endpoint keeps the contract small and avoids bloating the list response. A new sqlc query `CountMyJobsByStage` does `SELECT stage, count(*) ... GROUP BY stage` (NULL stage = applied-but-unstaged is its own group); the Go function folds rows into buckets. Response: `{"data": {"applications": N, "buckets": {…7 keys…}}}`. The frontend derives the two rates from bucket counts (trivial arithmetic; the bucket mapping — the real domain logic — stays in Go).

**4. Frontend: container + two presentational SVG components.**
`PipelineView.svelte` (fetch + loading/error/empty) composes `PipelineFunnel.svelte` (Sankey, props `applications`+`buckets`) and `RateDonut.svelte` (props `percent`+`label`). Bucket labels/colors/order are a static map in the frontend (7 stable buckets). Tab added to `MyJobsView.svelte` (order Board / Pipeline / History; default stays Board); auth gating is inherited from `MyJobsView`.

## Risks / Trade-offs

- **Lower-bound rates may confuse users** → UI sub-text states it's a current-status snapshot; a rejected-after-interview job shows only as rejected.
- **Bucket vocabulary drift between Go and the frontend static map** → buckets are few and stable; documented as a known seam (codegen deferred, YAGNI). If a stage is added to `stages.go`, the mapping function and frontend map must be updated together — covered by the mapping unit test.
- **Empty state** (no applications) → component renders a friendly "You haven't applied to any jobs yet" rather than a zero-width Sankey.

## Migration Plan

No schema change. Deploy backend + frontend together. Rollback is removing the tab and route — no data migration. After merge, regenerate sqlc is already committed (`make sqlc` output checked in).

## Open Questions

None — all design decisions were settled during brainstorming.
