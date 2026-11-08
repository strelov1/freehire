## Context

`search_jobs` (OJCP, REST + MCP, `internal/api/handler/ojcp.go` `SearchJobs`) runs the same
Meilisearch keyword query the public search runs. The jobs index already carries one
`userProvided` embedder, `skills`, written by three document writers (`cmd/reindex`,
`cmd/search-drain`, `internal/ingest/linkimport`) via `JobDocument.Vectors`
(`map[string][]float32`, `internal/search/search/document.go`).

Description embeddings exist outside the index: `job_semantic_chunks` (pgvector, e5-base,
768 dims, HTML-stripped full description split into ~2000-rune chunks with a
`passage: {title} at {company}.` prefix). Two processes write it: `cmd/embed`
(`internal/ai/embed.Store.CompleteOpen`) and the volunteer broker
(`freehire-volunteer-embed`, `internal/promote/promote.go`, raw SQL in another repository).
Coverage is `is_tech` postings only.

Measured on prod, 2026-10-18:

| What | Value |
|---|---|
| Open jobs with chunks | 861,802 |
| Their chunk rows | 2,124,913 (avg ~2.5, max 28 per job) |
| `semantic_outbox` backlog | 22,987 |
| Meili volume | 42 GB used, 849 GB free |
| TEI query embed (`10.0.1.2:8081`) | 0.09s idle; 8–10s or >10s timeout while `freehire-embed-bf` drains on it |

The query-side embed (`query: ` prefix) was deleted with
`drop-hybrid-search-pgvector-similar`; nothing embeds a query today.

## Goals / Non-Goals

**Goals:**

- An agent can opt into meaning-based ranking per request; everyone else sees no change.
- Every filter, the visibility rules and pagination apply to a semantic search unchanged.
- A slow or absent embedding backend never fails or stalls an agent's search.
- No vector is recomputed to populate the index.

**Non-Goals:**

- Semantic search on the public `/jobs/search`, the SPA, or the ChatGPT app.
- Embedding non-`is_tech` postings. They stay keyword-reachable inside a hybrid query.
- A dedicated query-only TEI. The fallback makes it optional; add it if the `unavailable` rate
  says so (see Open Questions).
- Tuning `semanticRatio` per request. One server constant.

## Decisions

**1. Vectors live in the main jobs index as a second `userProvided` embedder `desc`.**
Alternative: two-stage (Meili filters → top-N ids → pgvector rerank). Rejected: recall is
capped by the keyword stage, so a posting that matches the meaning but not the words never
reaches the reranker — the exact case this feature exists for. A separate semantic index was
the design removed in `drop-hybrid-search-pgvector-similar`: it held a different document set
than the keyword index and needed its own filters. In the main index the existing filter
applies as is.

**2. Multi-vector documents, not mean pooling.** `_vectors.desc` is an array of the posting's
chunk vectors; Meilisearch scores a multi-vector document by its nearest vector, the same
"nearest passage wins" rule `/similar` uses. Disk is not the constraint (est. 15–20 GB against
849 GB free), and pooling would bring back the dilution chunking was introduced to fix.
`JobDocument.Vectors` changes from `map[string][]float32` to a type that can hold one vector
(`skills`) and many (`desc`) — e.g. `map[string]any` built by two typed helpers, or a small
struct with `json` tags. The `skills` wire shape must stay byte-identical.

**3. Writers read chunks in batch; no per-document query.** A new sqlc query
`ListSemanticChunksForJobs(job_ids bigint[])` returns `(job_id, chunk_index, embedding)`
ordered by job and chunk; each writer groups it into a `map[int64][][]float32` and passes it to
the document builder. The rebuild already pages jobs in batches, so this is one extra query per
batch. `embedding` is read as `pgvector.Vector` (already a dependency).

**4. Re-queue through a database trigger.** An `AFTER INSERT` statement-level trigger on
`job_semantic_chunks` (transition table `new_rows`) inserts `DISTINCT job_id`s into
`search_outbox` with `ON CONFLICT (job_id) DO NOTHING`, filling `job_posted_at` from `jobs`
and only for open, canonical jobs. Alternative: one more statement in each writer's
transaction. Rejected because one writer is raw SQL in another repository and the two would
drift silently — a missed enqueue looks exactly like a posting without vectors. Statement-level,
not row-level, because a job contributes ~2.5 rows per write. Delete-only paths (closed jobs)
do not fire it.

**5. Query embed: one call, short timeout, no retry — and not a `Client` method.** A new
`search.QueryEmbedder` sends one `query: `-prefixed input to the TEI `/embed` route with its
own deadline (`EMBED_QUERY_TIMEOUT_MS`, default 400 ms). It shares the wire code with the
bulk path (`embedCall`) but must NOT reuse `embedChunk`, whose 5 attempts with exponential
backoff are right for a batch drain and wrong for a request. It is its own type because a
`Client` falls back to `embedderURL` when nothing is configured, while the request path must
know it has no backend: `NewQueryEmbedder("")` returns nil. Any error, timeout or missing
`EMBED_URL` → keyword search + `semantic: "unavailable"`. So does the engine refusing the
hybrid with a 400 (`ErrBadQuery`) — the shape a binary querying `desc` before the live
settings declare it gets — which turns the rollout-order hazard into a fallback rather than
an outage.

**6. Hybrid request: `semanticRatio` 0.5 plus a configured `rankingScoreThreshold`.** Vector
search ranks every embedded document, so without a threshold a hybrid query pads every page
and `total_results` approaches the embedded catalogue
(`TestIntegration_HybridSearchFindsByMeaning` fails without it). The threshold is
`SEMANTIC_SCORE_THRESHOLD` (0–1, default 0 = none), set by hand against a sample of real
agent queries before the flag is announced (task 6.6) — not guessed in code. `SearchParams`
gains `SemanticVector` and `ScoreThreshold`; `buildSearchRequest` emits the `desc` hybrid
beside the existing `skills` one, and the match sort's `Vector` wins if both are set.

**7. Bulk embedding shares the query TEI (revised 2026-10-19).** The plan was to move bulk
embedding off `10.0.1.2:8081` to the volunteer broker. The owner kept the server-side
`cmd/embed` instead: the keyword fallback (Decision 5) already turns contention into
`semantic: "unavailable"` rather than a slow search, and the large backfill
(`freehire-embed-supervisor`) drains and exits on its own. A dedicated query TEI stays the
lever if the `unavailable` rate says so.

**8. Extension fields named plainly.** Input `semantic` (bool), response `semantic`
(`"applied"` / `"unavailable"`, omitted when not requested), following the `ignored_params`
precedent. The MCP SDK infers the output schema from the Go type and validates every response
against it, so the new response field is a plain `string` with `omitempty` — no custom
marshaller.

## Risks / Trade-offs

- **[Risk] Volunteers stop and fresh postings get no vectors** (it happened 2026-10-14..18). →
  Hybrid still returns them through the keyword half; `queue-metrics` already exposes outbox
  depth, so a growing `semantic_outbox` is visible. `cmd/embed` can be run against a different
  endpoint for catch-up.
- **[Risk] A partial-update push drops the `desc` vectors.** The drain's push is a field-level
  update; if `_vectors` is replaced as a whole object, a writer that sends only `skills` would
  erase `desc`. → All three writers always send both; an integration test pushes a document
  through the drain path and asserts both embedders survive.
- **[Checked] Documents carrying `desc` before the live index declares it.** Between the
  deploy and the settings patch, the drain pushes `_vectors.desc` to an index that has only
  `skills`. Probed 2026-10-18 against Meilisearch 1.13 and 1.49.0 (prod's version): both
  accept the document, with `null` and with vectors, so the window costs nothing.
- **[Risk] Rebuild gets slower.** HNSW construction over ~2.1M more 768-dim vectors lands on
  the rebuild, which already carries the skill vectors. → Measure on the first rebuild;
  Meilisearch binary quantization on `desc` is the lever if it does not fit the window.
- **[Risk] Trigger adds write load to `search_outbox`.** Volunteer throughput (~2.6 jobs/s) →
  ~9k enqueues/hour, against a drain that pushes 200-document batches in ~8s. Acceptable; and a
  rebuild needs no trigger traffic because it reads chunks directly.
- **[Risk] A dead-lettered `search_outbox` entry blocks the re-queue** (`ON CONFLICT DO
  NOTHING` against a `failed_at` row). → Same behaviour as every other enqueue today; the next
  rebuild picks the vectors up from Postgres.
- **[Trade-off] Non-tech postings have no vectors.** Accepted: the agent audience searches IT
  roles, and they remain keyword-reachable.

## Migration Plan

Order matters — an embedder queried before the live index declares it is a 400 → 500 for
every agent search that sets the flag.

1. Merge code + migration. `release.sh` applies the trigger migration. With `EMBED_URL`
   unset on the API server, the flag answers `unavailable` until step 4 (see Open Questions).
2. Ops: stop `freehire-embed-supervisor`; confirm the volunteer broker is draining.
3. Patch the live jobs index settings with the `desc` embedder (dimensions 768), then run a
   full rebuild with the `reindex` timer stopped. Verify `search-settings-drift` reports 0.
4. Set `EMBED_URL`/`EMBED_API_KEY` on the API server, restart. Verify by hand: the same query
   with and without `semantic: true`, with filters, and with TEI blocked (expect
   `unavailable` and keyword results).
5. Tune `rankingScoreThreshold` on sampled agent queries; then announce the field in the
   manifest / docs.

Rollback: unset `EMBED_URL` on the server — every request answers keyword-only with
`unavailable`. The embedder and vectors can stay in the index; removing them is a settings
change plus rebuild.

## Open Questions

- Starting `rankingScoreThreshold` and `semanticRatio` — decided by hand on real queries in
  step 5, not guessed here.
- Whether the server should refuse `applied` until the live index declares `desc` (a cheap
  settings check at boot) instead of relying on `EMBED_URL` being unset during rollout.
  Default: rely on `EMBED_URL` plus `search-settings-drift`.
- Whether a query-only TEI is needed. Decide after a week of the `unavailable` rate.
