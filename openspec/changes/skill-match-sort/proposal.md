## Why

The jobs feed can only be ordered by freshness, salary, or keyword relevance —
none of which knows anything about the person reading it. The embedding-backed
"recommended" sort that once did was dropped in `e52f28a6` along with
`/recommendations`, and nothing replaced it. A signed-in candidate with a filled
profile browses the same undifferentiated catalogue as an anonymous visitor.

## What Changes

- A candidate's skills and a vacancy's skills each become a fixed-width vector,
  so the search engine can order the whole catalogue by how well they overlap.
- Skills are already canonical slugs from a finite dictionary (749 of them), so
  the vector is built arithmetically — **no embedding model is involved**, no
  inference cost, no AI credits.
- An overlap on a rare skill counts for more than an overlap on a ubiquitous one.
  Weights come from the facet-distribution rollup that already exists.
- `GET /api/v1/jobs/search` accepts `sort=match`. It gains optional auth: the
  endpoint stays public, and the new sort simply has nothing to rank against for
  a caller it cannot identify.
- The Meilisearch jobs index declares a `userProvided` embedder and each document
  carries its skill vector.
- The SPA offers the sort to signed-in visitors whose profile has skills.

Not breaking: every existing sort, filter, and response shape is untouched, and
`sort=match` degrades to the default feed rather than erroring for any caller who
cannot be served it.

## Capabilities

### New Capabilities
- `skill-match-sort`: the skill vector — its permanent position registry, its
  rarity weighting, how a vacancy's and a candidate's are built, and the ordering
  their cosine produces.

### Modified Capabilities
- `job-search`: the index gains a `userProvided` embedder and a per-document
  vector; the search endpoint gains the `sort=match` directive, optional
  authentication, and the rule that a vector ranking suppresses attribute sorting.

## Impact

**New code:** `internal/dict/skillvec` (registry, weights, vector construction,
plus a generator for the registry), `internal/search/search/skillweights.go`.

**Modified code:** `internal/dict/skilltag` (expose the canonical slug list),
`internal/search/search/document.go` (**`FromJob` changes signature** — it takes
the weights, so the compiler catches an indexer that forgets),
`internal/search/search/client.go` (embedder declaration, query vector),
`cmd/reindex`, `cmd/search-drain`, `internal/ingest/linkimport`,
`internal/api/handler/search.go`, `internal/platform/arch/layering/blocks.go`,
`web/src/lib/facetModel.ts` and the sort selector.

**Reused, not built:** `insights_facet_stats` already carries per-skill open-job
counts, populated by `cmd/rollup-facets`. `jobmatch.Compute` is untouched and
keeps serving the per-job match bar.

**No migrations.** No new tables, columns, or queries.

**Deployment is gated on host2 disk, and this is not incidental.** Introducing an
embedder requires a full index rebuild — it cannot be added to a live index
incrementally — and `cmd/reindex` currently refuses to run (62 GiB free against a
70 GiB floor). The rebuild also grows the index by ~10 GB and runs materially
longer. `document.go` already caps indexed descriptions at 1000 runes to keep
rebuilds inside that same budget. Ship the code; schedule the rebuild separately.
