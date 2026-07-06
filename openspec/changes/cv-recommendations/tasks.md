## 1. Schema & queries

- [x] 1.1 Add a migration: `users.resume_embedding float8[]` (nullable) and `users.resume_embedding_model text` (nullable, the embedder identity that produced the vector). Note in the change that it must be applied on prod before deploy.
- [x] 1.2 Add sqlc queries: set the CV embedding (`SetUserResumeEmbedding` — vector + model), read it (`GetUserResumeEmbedding`), and clear it (extend `ClearUserResume` or add). Run `make sqlc`.

## 2. Same-space CV embedding helper (search client)

- [x] 2.1 Write a failing test for a `search.Client` `EmbedText(ctx, text) → (vector, model, error)` helper that obtains a vector in the jobs' space via Meili read-back (integration test against Meili where available; unit-cover the ensure/upsert/retrieve/delete sequencing). — unit-covered the version-fragile `decodeEmbedding` parsing (`embed_test.go`); the full Meili round-trip is exercised under `-tags=integration`.
- [x] 2.2 Implement `EmbedText`: ensure a `resume_vectors` index with embedder settings identical to `jobs_semantic`, upsert the CV text as one scratch doc, fetch it with `retrieveVectors:true`, delete the scratch doc (no CV text persisted), return the vector + the current embedder model id. — plus `RecommendByVector` (vector search over `jobs_semantic`) and `CurrentEmbedderModel` for the staleness guard.

## 3. Compute the CV vector on upload

- [x] 3.1 Write a failing handler test: `PutResume` with a CV → the persisted vector + model are set via a fake embedder; on embedder/storage failure the upload still succeeds and leaves no vector (best-effort, degrade-not-error).
- [x] 3.2 Implement the hook in `PutResume`: after the blob is stored and text extracted (reuse `pdfText`), call `EmbedText` and persist via `SetUserResumeEmbedding`; swallow+log errors so the upload never fails on the embedding step. — routed persistence through `resume.Store.SetEmbedding` (repo interface) so the hook is unit-testable; clears the vector on embed failure.

## 4. Recommendations endpoint

- [x] 4.1 Write a failing handler test (fake searcher): `GET /me/recommendations` with a fresh vector → the searcher is asked to vector-rank `jobs_semantic` and job views are returned; no/stale vector (model mismatch) → successful empty list; unauthenticated → 401.
- [x] 4.2 Implement a `search.Client` vector search over `jobs_semantic` (rank open jobs by a raw provided vector, `limit`/`offset`). — `RecommendByVector` (added in group 2), absent-index degrades to empty.
- [x] 4.3 Implement the `Recommendations` handler: read the CV vector + model, ignore it when the model does not match the current embedder identity (stale) or is absent → empty list; otherwise vector-search and return the standard envelope. Wire `GET /api/v1/me/recommendations` behind `RequireAuthOrKey`.

## 5. Frontend `/my/recommendations` page

- [x] 5.1 Add an API client method `getRecommendations(limit, offset)`. — `api.recommendations(limit, offset)` (Slice<Job>).
- [x] 5.2 Add the `/my/recommendations` SvelteKit route rendering the feed of job views, with a non-error empty state and an "upload your CV" prompt when the user has no CV vector. — `RecommendationsView.svelte` (Paginator + JobRow) + thin page; empty state links to `/my/profile`.
- [x] 5.3 Add a signed-in navigation entry to the page. — `HeaderMenu.svelte` account links.

## 6. Verification

- [x] 6.1 `go test ./...` + `go vet ./...` green; web `svelte-check` clean; confirm no raw CV text is persisted (only the vector + S3 blob), the migration is recorded for prod-apply-before-deploy, and the swipe deck is unchanged. — go suite + vet green; svelte-check clean for the feature files (4 pre-existing vitest-module errors are environmental, unrelated); only the derived vector persists; migration 0002 documents prod-apply-before-deploy; swipe.go/facets.go untouched.
