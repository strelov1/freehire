## 1. Backend — bucket aggregation (pure domain)

- [x] 1.1 Define the pipeline bucket vocabulary + a pure `Buckets`/`Aggregate` function (in `internal/userjob`, or a new `internal/pipeline` package) that folds `(stage, count)` rows — including a null-stage row — into the seven buckets and an `applications` total, per the stage→bucket mapping in design.md. Source of truth for the mapping lives here.
- [x] 1.2 Unit-test the function (RED first): each application counted once (buckets sum to applications), `screening`/`responded` → `in_progress`, null-stage → `no_answer`, terminal stages map correctly, empty input → all zeros, unknown/out-of-vocab stage handled deterministically.

## 2. Backend — query + handler

- [x] 2.1 Add sqlc query `CountMyJobsByStage` in `internal/db/queries/` (`SELECT stage, count(*) FROM user_jobs WHERE user_id=$1 AND (applied_at IS NOT NULL OR stage IS NOT NULL) GROUP BY stage`); run `make sqlc` and commit generated output.
- [x] 2.2 Add a handler method near `internal/handler/me_jobs.go` that runs the query for the current user (`c.Locals`), feeds rows into the bucket function, and returns `{"data": {"applications": N, "buckets": {…}}}`; return errors for the central `ErrorHandler`.
- [x] 2.3 Wire `GET /me/jobs/pipeline` into the `/me/jobs` route group under `RequireAuthOrKey`.
- [x] 2.4 Integration test (`//go:build integration`, testcontainers): seed `user_jobs` rows (various stages, applied-no-stage, saved-only) and assert the endpoint returns the correct `applications` total and bucket counts, and `401` when unauthenticated.

## 3. Frontend — data layer

- [x] 3.1 Add `PipelineStats` type to `web/src/lib/types.ts` (applications + the seven bucket counts).
- [x] 3.2 Add `getMyPipeline()` to `web/src/lib/api.ts` calling `GET /api/v1/me/jobs/pipeline`.

## 4. Frontend — presentational components

- [x] 4.1 `RateDonut.svelte` — props `percent`, `label`, `sublabel`; hand-built SVG donut; zero-safe.
- [x] 4.2 `PipelineFunnel.svelte` — props `applications`, `buckets`; single-level Sankey SVG with proportional ribbons; static bucket label/color/order map; handles the all-zero case gracefully.

## 5. Frontend — container + tab

- [x] 5.1 `PipelineView.svelte` — fetch via `getMyPipeline()`; loading, error, and empty ("no applications yet") states; compute Interview/Offer rates from buckets; compose `PipelineFunnel` + two `RateDonut`s; include the snapshot sub-text.
- [x] 5.2 Add the **Pipeline** tab to `MyJobsView.svelte` (order Board / Pipeline / History; default stays Board); inherits the existing auth gating.

## 6. Verification

- [x] 6.1 `go build ./... && go vet ./... && go test ./...` green; integration test green with `-tags=integration`.
- [x] 6.2 Frontend checks: `npm run check` (svelte-check) clean for the new files; run the app and confirm the Pipeline tab renders the Sankey + rate donuts against real `/me/jobs/pipeline` data, including the empty state.
- [x] 6.3 Update any generated contracts if touched; confirm no new npm dependency was added.
