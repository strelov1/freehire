## Context

`intfloat/multilingual-e5-base` (host2's local TEI server) has a hard 512-token
sequence limit — exceeding it does not error, TEI's `--auto-truncate` flag silently
drops the overflow. Today's passage (`jobPassage` in `internal/search/embed.go`) is
built from `JobDocument.Description`, which is already:
1. Run through `internal/sources/sanitize.go`'s `descriptionPolicy` at ingest — a
   RENDER-safety allowlist (keeps `<p>`, `<li>`, `<strong>`, `<table>`, …), not a
   plain-text extractor.
2. Truncated to 1000 runes by `search.FromJob` for a completely unrelated reason
   (bounding the facet Meilisearch index's on-disk rebuild footprint).

So the text TEI actually sees is: HTML-tag-laden, pre-truncated to ~20% of the
average description, then silently re-truncated again at the token boundary.
Meilisearch's `userProvided` embedder natively stores multiple vectors per document
(`_vectors.<name>.embeddings` as an array of arrays) — chunking is a supported
first-class shape, not a workaround.

## Goals / Non-Goals

**Goals:**
- Every job's FULL, HTML-free description text is represented in its embedding(s) —
  nothing past word ~150 silently disappears.
- Multiple vectors per job (one per chunk), stored and searchable natively.
- Reuse the existing "model migration" mechanism (`CurrentEmbedderModel()` /
  `semantic_embedded_model`) to force a full re-embed — no new staleness-tracking
  concept.

**Non-Goals:**
- Not touching `maxIndexedDescriptionRunes` or the facet/keyword index — that cap's
  rationale (rebuild disk footprint) is real and unrelated to embeddings.
- Not touching the rendered description (`{@html}` on the site) — the new
  plain-text extraction is for the embedding path only, a parallel derivation, not
  a replacement of the stored/served HTML.
- Not enabling hybrid search for end users — still out of scope, `semantic_ratio`
  stays 0 (same non-goal as the steady-state cron change this builds on).
- Not implementing exact model tokenization in Go — chunk sizing is a
  conservative heuristic (see Decision 2), not a byte-exact token count. TEI's own
  `--auto-truncate` remains the safety net for a chunk that slightly overshoots.

## Decisions

**1. Strip HTML to plain text using `bluemonday.StrictPolicy()`, not a new
policy or regex**
`bluemonday` is already a project dependency (`internal/sources/sanitize.go`).
`StrictPolicy()` is the library's own built-in "remove all tags, keep only text"
policy — exactly what's needed, and distinct in *purpose* from
`descriptionPolicy` (render-safety allowlist) even though both are bluemonday
policies. Defined locally in `internal/search` (where `jobPassage` lives) rather
than exported from `internal/sources`, since it serves a different consumer with a
different goal (plain text for a model, not safe-to-render markup).

**2. Chunk by a conservative rune/word heuristic, not exact tokenization**
Replicating XLM-RoBERTa's actual SentencePiece tokenizer in Go to hit the 512-token
limit exactly is real engineering weight for a soft target — TEI's
`--auto-truncate` already fails safe (silently drops overflow within ONE chunk,
never errors) if a chunk estimate runs slightly over. Chunk on a rune-count budget
with a comfortable margin under 512 tokens (English/Latin-script text averages
~1.3-1.5 tokens per word for this tokenizer family; a conservative
~350-word / ~2000-rune budget per chunk leaves headroom for denser scripts/tokens
without needing per-language tuning). Split on paragraph/sentence boundaries where
possible so a chunk doesn't cut mid-sentence — job descriptions are already
paragraph-structured HTML before stripping, so paragraph boundaries survive into
the plain text and are a natural, cheap split point.

**3. Storage shape: NO migration — `real[]` already accepts a 2D value; scan
tolerantly instead**
Verified empirically against a disposable Postgres 18 container (2026-08-09,
container discarded after): Postgres does not enforce an array column's declared
dimension count — a column declared `real[]` transparently stores and returns BOTH
a flat vector (`ARRAY[1,2,3]`) and a 2D array (`ARRAY[[1,2],[3,4]]`); `real[][]` and
`real[]` are the same underlying catalog type. So no `ALTER TABLE` is needed at all
— the originally planned "exact DDL for `real[]` → `real[][]`" open question is
moot.

That does NOT mean the Go side is free of a shape problem: `db.Job.SemanticEmbedding`
is one field on the ONE struct sqlc generates for `jobs.*`, scanned by roughly a
dozen query sites across `internal/db/jobs.sql.go` — not just the embed pipeline's
narrow `GetJobsByIDs`, but the general job-listing/detail queries the whole site
runs. Flipping that field's Go type straight to `[][]float32` would make pgx error
on every row still holding OLD single-vector data (verified: scanning a 1D `real[]`
value into a `[][]float32` destination fails) — during the re-embed drain window
(days one lever, task 5.1's model-version bump, forces to run), that is not a
narrow semantic-search-only failure, it is a whole-site job-loading failure for any
row not yet re-embedded.

Resolved with the user 2026-08-09 (their explicit ask: "fallback if there's no
[multiple vectors], use 1 like now"): scan the column via sqlc's generic
`pgtype.Array[float32]` (an `overrides:` entry on `jobs.semantic_embedding` in
sqlc.yaml, the same mechanism already used for `jobs.enrichment` etc.) instead of a
concrete `[][]float32`. `pgtype.Array[float32]` carries `Elements []float32` plus
`Dims []pgtype.ArrayDimension`, so it scans EITHER shape without erroring.
A small hand-written pair of functions in `internal/search`
(`SemanticVectorsFromArray`/`SemanticVectorsToArray`) reshapes between that and
`[][]float32`: a 1-dimension array becomes a single-element chunk list (byte-for-byte
the old single-vector behavior — nothing about an unmigrated row's read path
changes), a 2-dimension array becomes its real chunk rows. No data backfill, no
deploy-timing coordination, no operational window to get wrong — every row is
readable at every point in the re-embed drain, old or new shape, permanently (not
just during a transition), which is a stronger property than a one-time nulling
backfill would have given. Cost: `internal/search` (not `internal/db`, per its own
"generated, edit via sqlc" convention) owns a bit of manual reshape logic instead of
the field being naturally typed — reviewed as worth it against a batched
production write burst on the 900k-row `jobs` table.

**4. Force a full re-embed via a versioned `CurrentEmbedderModel()` string, not a
new staleness mechanism**
`semantic_embedded_model`/`EnqueuePendingSemanticJobs`'s `IS DISTINCT FROM` check
already exists precisely for "the embedding strategy changed, re-embed everything"
— per its own doc comment, `target_model` covers "add + content-update +
model-migration" in one predicate. A chunking-strategy change IS a model
migration in this system's own vocabulary, even though the underlying HTTP
endpoint/weights are unchanged. Bump `embedderModel`
(`internal/search/client.go`) to a versioned value (e.g.
`"intfloat/multilingual-e5-base@chunked-v1"`) rather than inventing a parallel
"strategy version" column — reuses a mechanism the codebase already trusts and
tests, at the cost of the string no longer being literally the HF model id (already
somewhat true in spirit — it's a staleness KEY, not a display value; not shown to
users anywhere).

**5. Drop the enrichment-summary preference; embed the full description for every job**
The current `jobPassage` prefers `Enrichment.Summary` over `Description` specifically
because the summary is short enough to dodge `maxIndexedDescriptionRunes`'s
truncation — that rationale evaporates once the full description is chunked instead
of truncated. Resolved with the user 2026-08-09: embed chunks of the full plain-text
description only, for every job (enriched or not) — no special-cased summary vector.
Keeps one source of truth per job instead of two divergent embedding inputs, at the
cost of losing the summary's distilled, CV-query-adjacent phrasing as a signal — judged
not worth the added code/test surface absent evidence it measurably helps ranking.

## Risks / Trade-offs

- **[Risk] Doubles today's backfill cost if shipped after it finishes.** See
  proposal.md's timing note — this is the real, unresolved trade-off, and it's a
  scheduling decision, not an engineering one. Flagged, not decided here.
- **[Risk] Multi-vector storage/indexing is heavier per document** (N vectors
  instead of 1 — more TEI calls per job, more Postgres row bytes, a bigger Meili
  `_vectors` payload). Average description ~4900 runes / ~2000-rune chunks ≈ 2-3
  chunks per job typically — a 2-3x embed-time and storage cost per job, not an
  order of magnitude. Given the current backlog is already is_tech-gated (~440k
  eligible, not the full multi-million catalogue), this is judged acceptable
  without a dedicated capacity re-check — revisit if TEI throughput becomes the
  bottleneck again post-ship.
- **[Risk] Query-side behavior for multi-vector documents** (how does a search
  query match against a job with 3 candidate vectors — nearest of the 3? all 3
  independently ranked?) needs verification against Meilisearch's actual behavior,
  not just the existence of the storage format, before shipping. Not fully
  researched in this planning pass — first implementation task.

## Migration Plan

1. Resolve the proposal.md timing question with the user before starting
   implementation (pause/restart today's in-flight backfill vs. accept a second
   full backfill later). **Resolved 2026-08-09: ship after.**
2. ~~Migration: `jobs.semantic_embedding real[]` → `real[][]`~~ **No migration —
   see Decision 3, resolved 2026-08-09 after empirical verification: `real[]`
   already accepts a 2D value at the Postgres level.** Instead: an
   `sqlc.yaml overrides:` entry (`jobs.semantic_embedding` → `pgtype.Array[float32]`)
   plus `internal/search`'s `SemanticVectorsFromArray`/`SemanticVectorsToArray`.
3. Code: plain-text extraction, chunking, multi-vector embed/store/index plumbing.
4. Bump `embedderModel`, verify the enqueue predicate re-queues the whole eligible
   set as expected (a real re-embed, by design).
5. Verify Meilisearch's actual multi-vector query/ranking behavior (Risk above)
   before considering this shippable.

## Open Questions

- ~~Timing: ship before or after today's in-flight backfill completes?~~
  Resolved 2026-08-09: after.
- ~~Exact DDL for the `real[]` → `real[][]` column shape change.~~ Resolved
  2026-08-09: none needed (Decision 3).
- Meilisearch's precise multi-vector scoring behavior — needs hands-on
  verification, not just documentation reading. **Resolved 2026-08-09** (task 1
  spike, tasks.md): nearest-of-N cosine scoring, verified against a disposable
  Meilisearch v1.49.0 instance.
