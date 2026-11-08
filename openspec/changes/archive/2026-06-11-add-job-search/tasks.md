## 1. Dependency, config, and infra

- [x] 1.1 Add `github.com/meilisearch/meilisearch-go` (recent version with
  `Embedder` settings + `SearchRequestHybrid`) via `go get`; `go mod tidy`.
- [x] 1.2 Add Meilisearch settings to `internal/config`: `MeiliURL`
  (default `http://localhost:7700`) and `MeiliKey` (`MEILI_MASTER_KEY`, no
  default). Keep search optional (absent key ⇒ search disabled, server still
  starts).
- [x] 1.3 Add a `meilisearch` service to `docker-compose.yml` (pinned
  `getmeili/meilisearch` image, `MEILI_MASTER_KEY` env, persistent volume, port
  7700) and wire `MEILI_URL`/`MEILI_MASTER_KEY` into the `app` service. Add a
  `make reindex` target.

## 2. internal/search package

- [x] 2.1 Test-first: `JobDocument` + `FromJob(db.Job)` mapping — decodes the
  `enrichment` JSONB into flat facet fields (work_mode, employment_type,
  seniority, category, domains, countries, company_type, company_size,
  visa_sponsorship, salary_currency, salary_period, skills, salary_min,
  salary_max, experience_years_min), maps title/company/description/location and
  posted_at→unix, keys by `id`. Unenriched job ⇒ document with empty/zero facets.
- [x] 2.2 Implement `NewClient(url, key)` and `EnsureIndex(ctx)` that creates the
  `jobs` index and applies settings: searchable, filterable, sortable attributes,
  ranking rules (`words, sort, typo, proximity, attribute, exactness`), typo
  tolerance, pagination `maxTotalHits`, and the `default` huggingFace embedder
  (multilingual MiniLM). Idempotent (safe to call repeatedly). Enable the
  `vectorStore` experimental feature here only if the pinned image requires it.
- [x] 2.3 Implement `IndexJobs(ctx, []JobDocument)` (batched upsert by primary
  key) and `DeleteJob(ctx, id)`.
- [x] 2.4 Implement `Search(ctx, SearchParams) (SearchResult, error)`: builds the
  Meilisearch request (q, filter expression from facet groups — OR within a
  facet, AND across facets — sort, limit/offset, `Hybrid{Embedder:"default",
  SemanticRatio}`). Always set `Embedder` explicitly. `SearchResult` carries hits
  - `estimatedTotalHits`. Unit-test the filter-expression builder.

## 3. cmd/reindex batch worker

- [x] 3.1 Add `cmd/reindex/main.go` mirroring `cmd/enrich`: `config.Load` →
  `database.Connect` → `search.NewClient` → a `Runner` that calls `EnsureIndex`
  once, paginates `jobs` from Postgres, maps to `JobDocument`, and pushes in
  batches via `IndexJobs`. Log counts (indexed, batches).
- [x] 3.2 If offset paging over `jobs` is insufficient, add a `ListJobsForIndex`
  query in `queries/jobs.sql` and `make sqlc`; otherwise reuse `ListJobs`. Verify
  `go build ./...` and `go vet ./...`.

## 4. Search endpoint

- [x] 4.1 Test-first (handler test, httptest + fiber): `SearchJobs` parses `q`,
  facet filters, `sort`/`order`, `limit`/`offset` (reuse page-param clamping),
  optional `semantic_ratio`; returns `{"data": hits, "meta": {total, limit,
  offset}}`. With search disabled (no client) it returns a clear error status,
  not a panic.
- [x] 4.2 Wire the search client into the `Handler` struct and `Register`; add
  `api.Get("/jobs/search", h.SearchJobs)` registered BEFORE `/jobs/:id` so the
  literal route wins. Pass the client from `cmd/server` (nil when unconfigured).

## 5. SPA

- [x] 5.1 Add a thin search box to the Svelte SPA under `web/` that calls
  `GET /api/v1/jobs/search?q=...` and renders the results list (reuse the
  existing job card/list rendering).

## 6. Verification

- [x] 6.1 `go build ./...`, `go vet ./...`, `go test ./...`.
- [x] 6.2 Integration check against a real Meilisearch container (testcontainers
  or `make up`): `EnsureIndex` + `IndexJobs` + a keyword search and a
  hybrid (`semantic_ratio>0`) search return expected hits; confirm whether the
  pinned image needs the `vectorStore` experimental flag and lock it in.
- [x] 6.3 End-to-end smoke: `make up`, `make reindex`, then
  `GET /api/v1/jobs/search?q=...` returns indexed jobs in the standard envelope.
