## Why

A prior attempt to default the main jobs search to a hybrid (keyword+semantic) blend was rejected after code review: `jobs_semantic` is gated on `is_tech=TRUE` at the embed-enqueue level, so it silently excludes real, not-yet-classified jobs from any query that touches it — a materially different, smaller document set than the keyword `jobs` index. Investigating alternatives surfaced that the *only* legitimate remaining consumers of semantic similarity are `/jobs/:slug/similar` and CV-based `/recommendations`, and that `/similar` doesn't need a live index at all: "which jobs are similar to job X" is answerable once per job and cached, exactly like the sibling project `../telagon` already does for "similar channels" (`cmd/similar-backfill` + a precomputed lookup table). `jobs.semantic_embedding` (Postgres) is already the source of truth for job vectors and already has *more* coverage (1,610,951 rows) than the live `jobs_semantic` Meili index (1,021,471 docs, since it inherited the is_tech gate and Postgres never did).

Given that, keeping a second, expensive-to-rebuild, drifting Meili index alive is no longer justified. A local (non-prod) spike this session confirmed `pgvector` is installable on prod's self-managed Postgres 18 and answers nearest-neighbor queries correctly and fast — both for the batch precompute path `/similar` needs and the live per-request path `/recommendations` still genuinely needs (its query vector is a specific user's CV, combined with arbitrary facet filters chosen at request time — not precomputable the same way).

## What Changes

- **New**: `similar_jobs` table (or equivalent) populated by a new run-once-and-exit worker, `cmd/similar-backfill`, mirroring `../telagon/server/cmd/similar-backfill` — for every open job with a Postgres-stored embedding but no (or stale) precomputed neighbours, runs one pgvector nearest-neighbor query and upserts the top-N job IDs. Idempotent, incremental (catches up only what's missing/stale), re-runnable on a cron.
- **Changed**: `GET /api/v1/jobs/:slug/similar` reads from `similar_jobs` (a plain indexed lookup) instead of querying the live `jobs_semantic` Meili index. Same route, same response envelope, same public contract — only the server-side data source changes.
- **Changed**: `GET /api/v1/me/recommendations` (CV-based) queries `jobs.semantic_embedding` directly via pgvector (`ORDER BY embedding <=> $1` combined with a `WHERE` clause built from the same facet-filter translation it uses today) instead of Meili's hybrid search. This stays a live, per-request query — it is the one legitimate remaining live use of vector search, since the query vector (a specific user's CV) and the facet filters are both only known at request time.
- **Removed** (**BREAKING** for internal ops tooling, not for any public API): the `jobs_semantic` Meili index and everything that builds or serves it —
  - `internal/search/client.go`: `semanticSettings`, `EnsureSemanticIndex`, `NewSemanticRebuild`, `IndexSemanticJobs`, `IndexSemanticJobsFromPG`, `ResetSemanticIndex`, `SimilarJobs` (Meili-backed), `RecommendByVector` (Meili-backed) — replaced by the pgvector-backed equivalents.
  - `cmd/reindex --semantic` (and its `--from-pg` / `--posted-within` flags) — the whole semantic swap-rebuild path.
  - The Meili-write half of `cmd/embed`'s `Indexer` port — the Postgres-write half (`jobs.semantic_embedding`, the stamps) stays; it becomes the pipeline's only remaining side effect and the sole source of truth.
  - `semantic_ratio` from the public search API (`internal/handler/search.go`, `GET /api/v1/jobs/search`) and every SPA caller of it (`web/src/lib/api.ts`) — the general keyword+facet search stays keyword-only, as it already effectively was.
  - `pgvector` extension gets installed on prod Postgres (new operational dependency, replacing Meili's embedder infrastructure for this purpose — TEI embedding itself is unaffected, only where the resulting vectors are queried).

## Capabilities

### New Capabilities

(none — this extends existing capabilities rather than introducing a new one)

### Modified Capabilities

- `similar-jobs`: "Similar-documents query over the semantic index" requirement changes from a live Meili similar-documents call to a precomputed-table lookup; adds the backfill worker's behavior as a new requirement.
- `semantic-embedding`: drops every Meili-index-write/delete requirement (`jobs_semantic` upsert/delete, the `reindex --semantic` reconciler) — the pipeline's only remaining effect is persisting/clearing `jobs.semantic_embedding` in Postgres.
- `cv-recommendations`: "Recommendations endpoint ranks jobs by the CV vector" and "CV embedding is persisted..." requirements change their vector-query backend from Meili's hybrid search to a live pgvector query; the embedding computation itself (TEI) is unaffected.
- `job-search`: removes the "Hybrid keyword and semantic search" requirement entirely (superseding the change that would have defaulted it on) — keyword search over the `jobs` index is unaffected.

## Impact

- **Code**: `internal/search` (client.go — large reduction), `internal/embed` (Indexer port loses its Meili half), `cmd/reindex` (drops `--semantic`/`--from-pg`/`--posted-within`), `internal/handler/search.go` + `similar.go`, `web/src/lib/api.ts`, a new `cmd/similar-backfill` + its `internal/similarjobs`-equivalent package, a new migration (pgvector extension + `similar_jobs` table + a `vector(768)` column or index on `jobs.semantic_embedding`).
- **Data**: `jobs.semantic_embedding` (`real[]`) needs a pgvector-compatible representation (`vector(768)`) and an ANN index (HNSW) to make the backfill worker's per-job query fast at ~1.6-2M rows — exact migration mechanics (new column vs. type change, index build timing/locking) belong in design.md.
- **Ops**: `postgresql-18-pgvector` needs installing on prod; `freehire-reindexw`-adjacent cron/timers tied to `--semantic` go away; a new `cmd/similar-backfill` cron cadence needs adding, mirroring how `cmd/embed` is scheduled.
- **Docs**: `internal/search/AGENTS.md`, `internal/embed/AGENTS.md` need updates reflecting the removed Meili semantic index; a new module doc for the similar-jobs backfill (or folded into `internal/embed/AGENTS.md`) is warranted per this repo's per-package AGENTS.md convention.
