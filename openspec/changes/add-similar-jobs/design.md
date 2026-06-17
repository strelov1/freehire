## Context

Hybrid search already runs against a second Meilisearch index, `jobs_semantic`,
configured with an in-engine huggingFace embedder (`internal/search/client.go`).
The reindex command indexes **open jobs only** into both indexes
(`cmd/reindex/main.go` `splitJobs`), so closed jobs are physically absent from
`jobs_semantic`. The pinned SDK (`meilisearch-go v0.36.3`) exposes
`SearchSimilarDocuments` / `SearchSimilarDocumentsWithContext`
(`SimilarDocumentQuery{ Id, Embedder, Limit, Filter, ... }`), and the engine
(`v1.13`) supports the similar-documents endpoint as a stable feature.

Slug→id resolution already has a lightweight query, `GetJobIDBySlug`
(`internal/db/queries/jobs.sql`), used as the hot-path read precedent by the
view-tracking endpoint. The job detail page is a SvelteKit route under
`web/src/routes/jobs/[slug]/`.

## Goals / Non-Goals

**Goals:**
- Surface semantically nearest open jobs for a given job through a public
  per-job endpoint and a job-detail section.
- Reuse the existing semantic index and embedder with zero indexing/settings
  changes.

**Non-Goals:**
- No change to the embedder, document template, index settings, or reindex.
- No personalization, no cross-filtering by category/region (a plain nearest-
  neighbour list, per the agreed scope).
- No new persistence; no caching layer (revisit only if latency demands it).

## Decisions

**1. Query via `SearchSimilarDocuments` against `c.semantic`, embedder
`"default"`.** This is the purpose-built primitive: it ranks by the source
document's already-computed vector — no query text to synthesize and no
re-embedding. Alternative considered: reuse `Search` with `SemanticRatio=1` and
the job's title as the query. Rejected — that re-embeds a lossy text proxy of the
job instead of using its stored vector, and would rank the source job itself
highly.

**2. Exclude the source job defensively in Go, not via a Meili filter.**
Meilisearch's similar endpoint excludes the source document by design, but rather
than depend on that (and rather than make the primary key `id` filterable just to
express `id != X`), request `limit+1` and drop any hit whose `id` equals the
requested id, then truncate to `limit`. Keeps the index settings untouched.

**3. No "open jobs" filter.** `jobs_semantic` already holds open jobs only
(reindex contract). Adding a `closed_at` filter would require making it a
filterable attribute and a reindex — unnecessary given the index invariant.

**4. New `search.Client.SimilarJobs(ctx, id int64, limit int) ([]JobDocument,
error)`.** Mirrors `Search`: callers never touch the SDK. Decodes hits into
`[]JobDocument` via `resp.Hits.DecodeInto`, the same path `Search` uses.

**5. Handler resolves slug→id with `GetJobIDBySlug`, then calls `SimilarJobs`.**
Unknown slug → `pgx.ErrNoRows` → 404 via the central `ErrorHandler` (no per-
handler error JSON). Response is `{"data": [...]}` of `jobview.Job` (the embedded
view inside each `JobDocument`), `id` omitted — identical shape to search hits.
Route `GET /api/v1/jobs/:slug/similar`, public, wired in `handler.Register`
beside the existing `/jobs/:slug`.

**6. SPA loads `/similar` for the detail page and renders a "Similar jobs"
section.** Failure or empty → render nothing (must not break the page), matching
the existing silent-failure convention for the view-tracking call.

## Risks / Trade-offs

- **Semantic index staleness** → A job closed since the last `reindex --semantic`
  may still appear as a neighbour until the next pass. Mitigation: accepted — this
  is the same staleness every semantic surface already has, and the detail page
  still renders a closed job; no new guarantee is promised.
- **Newly ingested jobs have no vector yet** → Their `/similar` is empty until the
  next semantic reindex, and they won't appear as neighbours. Mitigation: accepted
  inherited seam (the semantic pass runs on its own looser schedule); the section
  degrades silently to nothing.
- **Embedder latency per request** → similar queries hit the embedder index.
  Mitigation: bounded `limit`, no fan-out; revisit caching only if measured
  latency warrants it (Non-Goal for now).
