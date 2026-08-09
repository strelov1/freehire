## Why

The semantic-embedding passage (`internal/search/embed.go`'s `jobPassage`) has two
quality problems, both discovered while investigating today's `ClaimSemanticBatch`
performance fix and the resulting backfill:

1. **The text still carries HTML markup.** `jobs.description` is sanitized at
   ingest (`internal/sources/sanitize.go`'s `descriptionPolicy`) but deliberately
   KEEPS structural tags (`<p>`, `<li>`, `<strong>`, `<table>`, …) — that sanitizer
   exists to make the description safe to render with `{@html}` on the site, not to
   produce embedding-ready plain text. The embedding model sees literal tag soup
   mixed into the prose it's supposed to understand.
2. **The text is truncated twice, and neither cap is embedding-aware.** `jobPassage`
   reads `JobDocument.Description`, which `search.FromJob` already truncates to
   `maxIndexedDescriptionRunes` (1000 runes) — a cap that exists ONLY to bound the
   facet/keyword Meilisearch index's on-disk rebuild footprint (comment:
   "Descriptions average ~4900 runes; 1000 captures the role summary and the first
   requirements"). On top of that, TEI itself (`intfloat/multilingual-e5-base`, run
   with `--auto-truncate`) silently drops anything past its own ~512-token sequence
   limit. Combined, only the opening ~15-20% of an average description ever reaches
   the vector — requirements, tech stack, or comp details later in a posting are
   invisible to semantic search.

Meilisearch's `userProvided` embedder natively supports multiple vectors per
document (`_vectors.<name>.embeddings` accepts an array of arrays — "one embedding
per paragraph/chunk" is an explicitly documented use case), so the fix is not a
lossy compromise (mean-pooling into one vector) — it's chunk the full cleaned text,
embed each chunk, and store all of them against the one job document.

## What Changes

- A new plain-text extraction path for embedding purposes ONLY (the rendered
  description / facet-index copy are untouched): strip ALL HTML down to text, not
  bluemonday's render-safe allowlist.
- Embed the FULL description (decoupled from `maxIndexedDescriptionRunes`, which
  stays exactly as-is for the facet index — that cap's rationale, disk footprint
  during a full rebuild, is real and unrelated to embedding).
- Chunk the cleaned full text into pieces sized to the model's real limit (need to
  confirm intfloat/multilingual-e5-base's exact max sequence length — commonly 512
  tokens for this model family, to be verified, not assumed, during design), embed
  each chunk, and store the resulting vectors as an array (Meilisearch
  `_vectors.default.embeddings: [[...], [...], ...]`; Postgres
  `jobs.semantic_embedding` needs a shape change from a single `real[]` to multiple
  vectors — exact representation TBD in design, e.g. `real[][]`).
- Update `reindex --semantic --from-pg` (the Postgres-vector rehydration path) to
  read and republish multi-vector rows.

## Capabilities

### New Capabilities
(none)

### Modified Capabilities
(none — internal embedding-quality change, no request/response contract changes.
Semantic search itself is still not exposed to end users, `semantic_ratio` stays 0.)

## Impact

- `internal/search/embed.go` (`jobPassage`, `embedBatch`/`embedChunk`), `internal/embed`
  (the runner's Store/Indexer ports carry `[]float32` today — becomes `[][]float32`).
- `internal/search/document.go`/`client.go` (`JobDocument`, `IndexSemanticJobs`,
  `_vectors` construction).
- `migrations/` — a new migration changing `jobs.semantic_embedding`'s shape.
- `internal/db/queries/semantic.sql` (`SetSemanticEmbedding`) and `jobs.sql`
  (`GetJobsByIDs` et al. touch the column's Go type).
- `cmd/reindex` (`--semantic --from-pg` path).
- **Timing decision needed from the user**: today's one-time semantic-embedding
  backfill (started after the `ClaimSemanticBatch` perf fix, PRs #1665/#1667) is
  currently in progress on prod (~15% through ~893k queued entries as of this
  writing). `EnqueuePendingSemanticJobs`'s staleness key is
  `(semantic_embedded_model, semantic_embedded_hash)` — a chunking-strategy change
  does NOT change either of those, so it will NOT automatically re-embed anything
  already marked current. Shipping this AFTER today's backfill finishes means
  re-doing that same multi-hour backfill again (bumping `embedderModel` or
  otherwise forcing a re-embed) to get the improved vectors. Shipping it BEFORE
  today's backfill finishes avoids that duplicate work but means pausing/restarting
  the in-flight backfill. Not decided here — flag for the kickoff conversation.
