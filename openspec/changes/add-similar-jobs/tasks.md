## 1. Search client: SimilarJobs

- [x] 1.1 Add a failing integration test in `internal/search/search_integration_test.go` that indexes several semantic docs, calls `SimilarJobs` for one job's id, and asserts: neighbours are returned, the source job's id is absent, and the result honours the limit.
- [x] 1.2 Implement `SimilarJobs(ctx, id int64, limit int) ([]JobDocument, error)` in `internal/search/client.go` over `SearchSimilarDocuments` against `c.semantic` with embedder `"default"`; request `limit+1`, drop the hit whose id equals `id`, truncate to `limit`, decode via `resp.Hits.DecodeInto`.

## 2. HTTP endpoint

- [x] 2.1 Add a failing handler integration test (build tag `integration`) covering: known slug returns `{"data":[...]}` with `public_slug` and no internal `id`; `?limit=N` bounds the count; unknown slug returns 404.
- [x] 2.2 Implement the `SimilarJobs` handler: resolve slug→id via `GetJobIDBySlug` (return `err` on `ErrNoRows` → 404 via central `ErrorHandler`), parse+clamp `limit` with a default, call `search.SimilarJobs`, respond with the list envelope of `jobview.Job`.
- [x] 2.3 Wire `GET /api/v1/jobs/:slug/similar` (public) in `handler.Register` beside the existing `/jobs/:slug` route.

## 3. SPA job-detail section

- [x] 3.1 Load `/api/v1/jobs/<slug>/similar` for the detail route (in its `load`/data path), tolerating failure as empty.
- [x] 3.2 Render a "Similar jobs" section on `web/src/routes/jobs/[slug]/+page.svelte` that lists neighbours linking to their detail pages, and renders nothing when the list is empty.
- [x] 3.3 Regenerate web contracts if any shared shape changed, then verify the frontend with `svelte-check` (no new errors against the red baseline). [No shared shape changed — reuses the existing Job contract; svelte-check: 0 errors.]

## 4. Verify

- [x] 4.1 `go build ./... && go vet ./...`; run the new search + handler integration tests; confirm no settings/reindex change was needed. [build+vet clean; TestIntegration_SimilarJobs PASS (29s), TestSimilarJobsEndToEnd PASS (4.6s); no index settings or reindex touched.]
