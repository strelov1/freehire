## Why

An agent calling `search_jobs` describes what it wants in prose ("backend engineer on payments
infrastructure, Go, remote in Europe"), and keyword relevance only finds postings that repeat
those words. The catalogue already holds description embeddings for 862k open postings
(`job_semantic_chunks`, 2.12M chunk vectors, measured 2026-10-18) that nothing on a search
path reads, so meaning-based matching costs an export and a query-side embed rather than a
new embedding pipeline.

## What Changes

- `search_jobs` (both OJCP transports) accepts an opt-in extension field `semantic: true`.
  Only when it is set, and the query is non-empty, the query is embedded and the search runs
  as a Meilisearch **hybrid** query (keyword + vector blend). Without it the answer is
  byte-for-byte what it is today.
- The response reports what actually happened in an extension field (`semantic`:
  `applied` / `unavailable`), so an agent that asked for meaning-based matching and got
  keyword-only is told so — the same "a degraded answer says so" rule `ignored_params` follows.
- The query-side embed has a short timeout. A slow or failing embedding backend degrades the
  search to keyword-only; it never fails or stalls the request.
- The main jobs index gains a second `userProvided` embedder, `desc`. Each document carries
  its posting's chunk vectors (multi-vector, nearest chunk wins), copied from
  `job_semantic_chunks` by all three document writers (`cmd/reindex`, `cmd/search-drain`,
  `internal/ingest/linkimport`). No vector is recomputed for the export.
- A job whose chunk set is written or replaced is re-queued for the search index, so a vector
  computed after the posting was indexed reaches Meilisearch without waiting for a rebuild.
  Done by a database trigger, because two writers exist and one of them lives in another
  repository (`freehire-volunteer-embed`'s `promote`).
- The query embed shares a TEI with bulk catalogue embedding: measured 2026-10-18, it takes
  0.09s on an idle TEI and 8–10s or times out while a bulk drain runs. The planned move of
  bulk embedding to the volunteer broker was dropped (2026-10-19, see design Decision 7); the
  keyword fallback absorbs the contention.

## Capabilities

### New Capabilities

- `agent-semantic-search`: the opt-in hybrid mode of `search_jobs` — the request flag, the
  query embed and its timeout, the keyword-only fallback, and how the response reports which
  of the two the agent received.

### Modified Capabilities

- `ojcp-provider`: `search_jobs` gains the `semantic` input extension and the `semantic`
  response extension; the manifest/tool input schema declares the field so an agent can
  discover it.
- `semantic-embedding`: a written or replaced chunk set re-queues its job for the search index,
  whichever process wrote it.

## Impact

- **Code:** `internal/api/ojcp` (input + response fields), `internal/api/handler/ojcp.go`
  (embed-then-search with fallback), `internal/api/ojcpmcp` (tool schema),
  `internal/search/search` (second embedder in settings, multi-vector `_vectors`, hybrid
  request builder, query embed call), the three document writers, a new sqlc query reading
  chunks for a batch of job ids, one migration (trigger).
- **Meilisearch:** settings change + full rebuild required before the flag can return
  `applied`; index grows by an estimated 15–20 GB (849 GB free on the Meili volume). Rebuild
  becomes slower (vector graph over ~2.1M more vectors).
- **Ops:** none beyond the rebuild. The API server reads `EMBED_URL`/`EMBED_API_KEY` from the
  shared `.env`, where `cmd/embed` already found them.
- **Not affected:** the public `/jobs/search`, the ChatGPT app (`internal/api/mcpapp`), the
  match sort's `skills` embedder, `/similar`.
- **Prior art:** `openspec/changes/drop-hybrid-search-pgvector-similar` removed an earlier
  hybrid search. That one queried a SEPARATE, smaller `jobs_semantic` index and was proposed
  as the default for every search; this change puts vectors in the main index (so every
  filter applies unchanged) and is opt-in for agents only.
