## Why

Users tracking many applications on `/my/jobs` can see individual jobs on the Board and History tabs, but have no at-a-glance view of where their applications stand overall or how they convert. A Pipeline view answers "where are my applications right now, and what share reached an interview or an offer?"

## What Changes

- Add a new **Pipeline** tab to the `/my/jobs` page, alongside the existing Board and History tabs (default tab stays Board).
- The tab renders a **snapshot** of the signed-in user's current application distribution as a single-level Sankey-ribbon diagram (Applications → 7 status buckets), plus two donut cards: **Interview Rate** and **Offer Rate**.
- Add a dedicated aggregate endpoint `GET /api/v1/me/jobs/pipeline` (auth `RequireAuthOrKey`) that returns per-bucket application counts computed server-side over **all** the user's applications.
- Visualizations are hand-built SVG — **no new frontend dependencies**.
- **Snapshot semantics, not historical flow:** because only each job's current `stage` is stored (no transition history), each application is counted in exactly one current bucket; rate cards are an honest lower bound.

## Capabilities

### New Capabilities
- `application-pipeline`: the user's job-application pipeline snapshot — the server-side per-bucket aggregation of their tracked applications, the stage→bucket mapping and rate definitions, the `GET /me/jobs/pipeline` endpoint contract, and the Pipeline tab visualization (Sankey + rate donuts).

### Modified Capabilities
<!-- None: the aggregate endpoint and tab are self-contained in the new capability; user-job-tracking's existing endpoints and web-frontend's existing tabs are unchanged in behavior. -->

## Impact

- **Backend:** new sqlc query `CountMyJobsByStage` (`internal/db/queries/`, regenerate via `make sqlc`); a pure stage→bucket aggregation function (the mapping/rate source of truth, in Go); a new handler method near `internal/handler/me_jobs.go`; one new route under the existing `/me/jobs` group.
- **Frontend (`web/`):** new `PipelineView.svelte` (container) composing `PipelineFunnel.svelte` (Sankey SVG) and `RateDonut.svelte`; `getMyPipeline()` in `web/src/lib/api.ts`; `PipelineStats` type in `web/src/lib/types.ts`; a new tab wired into `MyJobsView.svelte`.
- **Data/schema:** none — read-only aggregate over the existing `user_jobs` table; no migration.
- **Dependencies:** none added.
