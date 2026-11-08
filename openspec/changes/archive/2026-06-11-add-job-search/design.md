## Context

`GET /api/v1/jobs` today is `ListJobs` — `ORDER BY posted_at DESC, id DESC` with
`LIMIT/OFFSET`, no query, no filtering. The data worth searching is split across
the raw `jobs` row (title, company, location, description, source, remote,
company_slug, posted_at) and the typed `enrichment` JSONB
(`internal/enrich.Enrichment` — controlled vocabularies for work_mode,
employment_type, seniority, category, domains, countries, company_type,
company_size, visa_sponsorship, salary, skills, experience). The repo already has
a batch-worker pattern (`cmd/enrich`: `config.Load` → `database.Connect` →
`Runner.Run`) and a reference-queue pattern (`enrichment_outbox`). The sibling
project telagon (`internal/meili`, `cmd/meili-sync`, catalog search handler) is a
proven blueprint: full documents stored in the engine, batch sync, hybrid search,
PG fallback.

Constraints: Go + Fiber v2 + sqlc (generated DB layer, no hand edits) + Postgres;
list responses use `{"data": ..., "meta": {...}}`, single `{"data": ...}`, errors
`{"error": msg}` via the central `handler.ErrorHandler`. Migrations apply only at
Postgres initdb. MVP stage — architecture is fluid.

## Goals / Non-Goals

**Goals:**

- A public `GET /api/v1/jobs/search` with full-text query, facet filters, sort,
  pagination, and a hybrid keyword/semantic blend — leaving `GET /api/v1/jobs`
  untouched.
- One Meilisearch document per job carrying everything needed to render a result
  (no DB rehydrate), with facets sourced from both the raw row and enrichment.
- A scheduled batch reindex (`cmd/reindex`) that ensures index settings and
  syncs jobs from Postgres idempotently.
- Idiomatic use of the official `meilisearch-go` SDK behind a thin
  `internal/search` package, so callers never touch the SDK directly.

**Non-Goals:**

- Real-time indexing on write. A `search_outbox` table (mirroring
  `enrichment_outbox`) is the documented seam; not built. Until ingest exists,
  scheduled reindex is sufficient.
- Ranking-rule tuning beyond Meilisearch defaults, search analytics, query
  suggestions/autocomplete.
- A versioned migration runner (unchanged; no schema change is required by this
  change — the index lives in Meilisearch, not Postgres).
- A rich SPA search UX — only a thin search box wired to the endpoint.

## Decisions

### 1. `internal/search` wraps `meilisearch-go`; callers never see the SDK

A small package owns: client construction from config, `EnsureIndex` (create
index + apply settings + embedder, idempotent), a `JobDocument` type + a
`FromJob(db.Job)` mapper, `IndexJobs(batch)`, `Search(params) -> Result`, and
`DeleteJob(id)` (for the future outbox/delete path). Handlers and `cmd/reindex`
depend on `internal/search`, not on `meilisearch-go`. *Why:* keeps the SDK
swappable and the wire/document mapping in one place; matches how `internal/enrich`
hides the LLM `Provider`. *Alternative — hand-rolled HTTP client (telagon):*
rejected per the project principle "prefer a library's intended API over a clever
shim"; the SDK gives typed settings/embedder/hybrid structs.

### 2. Document shape: store the full job, key by `id`, never expose `id`

`JobDocument` = primary key `id` (int64) + the public display fields + flattened
enrichment facets. Enrichment is decoded from the JSONB into typed fields at map
time so facets are first-class filterable/sortable attributes (not nested JSON).
`posted_at` is stored as a unix int for `sortableAttributes`. Unenriched jobs
index fine — enrichment facets are simply absent/zero. *Why store the full doc:*
telagon-style, results render without a DB round-trip; the search path stays
independent of Postgres availability. *Alternative — store ids, rehydrate from
PG:* rejected (extra latency + couples read path back to the DB the engine was
meant to offload).

**Consistency with the already-merged public-slug convention.** `add-job-public-slug`
is live on main: the internal `id` is never exposed and jobs are addressed by
`public_slug` (the canonical `handler.jobResponse` wire shape omits `id`). The
search path honors the same contract. `id` is the Meilisearch primary key
(natural unique key, valid id charset — unlike a slug, which may carry Unicode),
so the stored document carries it as `JobDocument{ID; JobView}` — the embedded
`JobView` flattens into the document JSON, and the `GET /api/v1/jobs/search`
response returns the `JobView` alone, so `id` is **stripped structurally**, not
by a mapping step.

**One job wire shape across the whole API.** `JobView` is byte-for-byte the same
shape the list/detail endpoints serve: the source/display fields, RFC3339 UTC
timestamps, and the **nested `enrichment` object** (not flattened facets). This
matters because the SPA renders search hits with the very same components as the
list (e.g. `JobRow` reads `job.enrichment` and `job.posted_at`). To filter and
sort on the nested facets, Meilisearch uses **dot paths** (`enrichment.seniority`,
`enrichment.salary_min`); `posted_at` is an RFC3339 UTC string, which sorts
chronologically as text (verified in the integration test), so no separate unix
field is needed. *Alternative — flat facet fields on the document + handler
re-nesting:* rejected; nesting keeps the index document identical to the API job
and the mapping trivial.

### 3. Index settings + hybrid embedder configured in `EnsureIndex`

`EnsureIndex` sets searchable/filterable/sortable attributes, ranking rules
(`words, sort, typo, proximity, attribute, exactness`), typo tolerance,
pagination `maxTotalHits`, and **one embedder named `default`** with source
`huggingFace`, model `sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2`
(multilingual, BERT-family — supported by the huggingFace embedder; runs inside
Meilisearch, no external key), and a `documentTemplate` over title/company/
description. Search always sets `Hybrid.Embedder = "default"` explicitly —
meilisearch-go sends `""` (→ `invalid_embedder`) if left zero. *Why MiniLM:*
small, multilingual, CPU-friendly; matches telagon. The embedder downloads the
model on first settings apply and embeds during indexing (CPU cost — acceptable
at MVP volumes).

### 4. `cmd/reindex`: batch sync mirroring `cmd/enrich`

`config.Load` → `database.Connect` → `search.NewClient` → `Runner{DB, Search}`.
The runner paginates `jobs` from Postgres (reuse `ListJobs` or add a
`ListJobsForIndex` cursor query if offset paging proves heavy), maps each to a
`JobDocument`, and pushes in batches via `IndexJobs`; `EnsureIndex` runs once at
the start. Idempotent: Meilisearch upserts by primary key, so re-running with
unchanged data yields the same document set. *Why a separate command:* same
operational model as enrichment (cron/scheduled), no coupling to the server
process. A `make reindex` target runs it.

### 5. Endpoint: `GET /api/v1/jobs/search`, public, standard envelope

A new `SearchJobs` handler parses `q`, facet filters (mapped to Meilisearch
filter expressions — each multi-value facet ORed internally, facets ANDed
together), `sort`/`order`, `limit`/`offset` (reuse the existing page-param
clamping), and optional `semantic_ratio` (default a balanced value; 0 = pure
keyword). It calls `search.Search` and returns `{"data": hits, "meta": {total,
limit, offset}}` where `total` is Meilisearch's `estimatedTotalHits`. Registered
as `api.Get("/jobs/search", h.SearchJobs)` — placed before `/jobs/:id` so the
literal route wins over the param route. *Why a dedicated endpoint:* keeps the
plain list (`ListJobs`) and search (different backend, different response
semantics — estimated totals, relevance order) cleanly separated.

### 6. Config + infra

`internal/config` gains `MeiliURL` (default `http://localhost:7700`) and
`MeiliKey` (`MEILI_MASTER_KEY`). `docker-compose.yml` gains a `meilisearch`
service (pinned `getmeili/meilisearch` image, `MEILI_MASTER_KEY` env, a
persistent volume, port 7700) that the `app` and `reindex` reach over the compose
network. Search is **optional**: if `MEILI_MASTER_KEY`/URL are unset, the server
still starts and the search endpoint returns a clear 503-style error rather than
crashing — the rest of the API is unaffected.

## Risks / Trade-offs

- **huggingFace embedder may require the `vectorStore` experimental feature.**
  It was experimental from Meilisearch v1.3 and stabilized around v1.13. → Pin a
  meilisearch image at a version where AI search is stable; if the pinned version
  still gates it, `EnsureIndex` issues an idempotent PATCH to
  `/experimental-features {"vectorStore": true}` before applying the embedder.
  The integration test against the real container is the check — resolve the
  exact version/flag there.
- **Local embedding is CPU-heavy and slow on first model download.** → Acceptable
  at MVP volumes; the model downloads once and is cached in the meilisearch
  volume. Reindex runs off the request path. If it ever hurts, swap the embedder
  source to a hosted `rest`/`openAi` embedder (config-only change in
  `EnsureIndex`).
- **Index drift between Postgres and Meilisearch** (a job updated/enriched after
  the last reindex). → Scheduled reindex bounds staleness; the `search_outbox`
  seam is the real-time fix when ingest lands. Documented, not built.
- **Empty `Hybrid.Embedder` → `invalid_embedder` error** (meilisearch-go quirk).
  → Always set `Embedder: "default"` explicitly in every search call; covered by
  a test.
- **`estimatedTotalHits` is approximate and capped** by `maxTotalHits`. → `meta.total`
  is documented as an estimate; fine for a "results" count, not for exact
  accounting.
- **New service in the dev/prod footprint.** → Optional-by-config keeps the core
  API runnable without Meilisearch; compose wires it for the full stack.

## Migration Plan

1. Add the `meilisearch` service + volume to compose; `make up` pulls/starts it.
2. Add config + `internal/search` + `cmd/reindex`; `make reindex` populates the
   index from existing rows.
3. Deploy the server with the search endpoint; if Meili config is absent the
   endpoint is disabled and nothing else changes (safe partial rollout).
4. Rollback: stop calling `/jobs/search` / hide the SPA search box; the index and
   service can be left running or removed — no Postgres schema change to revert.

## Open Questions

- Exact `getmeili/meilisearch` version to pin, and whether it still needs the
  `vectorStore` experimental PATCH — settle against the integration test.
- Default `semantic_ratio` for the endpoint when the client omits it (balanced
  ~0.5 vs keyword-leaning) — pick during handler implementation; cheap to tune.
- Whether reindex needs a dedicated cursor query (`ListJobsForIndex`) or plain
  `ListJobs` offset paging suffices at current volumes — decide when wiring the
  runner.
