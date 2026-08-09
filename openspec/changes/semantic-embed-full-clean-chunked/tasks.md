## 0. Kickoff decision (blocks everything else)

- [x] 0.1 Decide with the user: ship before today's in-flight semantic-embedding
      backfill (PRs #1665/#1667's follow-on) finishes (pause it, redo it once
      under the new scheme) or after (accept a second full backfill later)? See
      proposal.md's timing note and design.md's Migration Plan step 1.
      **Resolved 2026-08-09: ship AFTER the in-flight backfill finishes.**
      Implementation (tasks 1-5) proceeds now in an isolated worktree; the
      `embedderModel` version bump (task 5.1) and prod deploy/merge are held
      until the current backfill (~893k entries) completes, to avoid pausing
      it. Check backfill completion before merging.

## 1. Verify Meilisearch's multi-vector behavior (spike before committing)

- [x] 1.1 Stand up a local Meilisearch test index with a `userProvided` embedder,
      push a document with 2-3 vectors under `_vectors.default.embeddings` (array
      of arrays), and confirm empirically how a semantic query scores/ranks it —
      does it match on the nearest of the N vectors, or something else? (design.md
      Risk — not yet verified, only documentation-researched.)
      **VALIDATED 2026-08-09** (disposable `getmeili/meilisearch:v1.49.0`
      container, discarded after): a multi-vector document scores at the
      **nearest (max cosine similarity) of its N vectors** — a query matching
      any one chunk ranks the document as if that chunk were its only content.
      Confirmed with a 3-vector doc scoring 1.0 against queries near each of
      its 3 vectors individually, vs. 0.5 for orthogonal single-vector
      comparison docs.
- [x] 1.2 Confirm `retrieveVectors: true` round-trips the multi-vector shape as
      expected, since `reindex --semantic --from-pg` will need to read it back.
      **VALIDATED 2026-08-09**: `_vectors.default.embeddings` returned in the
      search response matches what was indexed, values and order unchanged.

## 2. Plain-text extraction for embedding

- [x] 2.1 Add a `stripToPlainText` (or similarly named) function in
      `internal/search` using `bluemonday.StrictPolicy()` — RED test first: feed
      it description HTML with headings/lists/tables/strong and assert the output
      is tag-free prose with reasonable whitespace (design.md Decision 1).
      **Done** (`internal/search/plaintext.go` + `plaintext_test.go`). Reviewed;
      fixed an entity re-escaping bug (`&amp;`/`&lt;`/`&gt;` leaking into
      embedding text) found in review. Documented, not fixed: implicitly-closed
      `<li>`/`<p>` (no explicit end tag) still glues — matches
      `sources.descriptionPolicy`'s own pre-existing limitation on malformed
      board HTML; deferred until shown to matter on real data.
- [x] 2.2 Wire it into the embedding path so `jobPassage` (or its replacement)
      uses the FULL, untruncated description — not `JobDocument.Description`
      (which stays capped at `maxIndexedDescriptionRunes` for the facet index,
      untouched — design.md Non-Goals). Likely needs the embed worker to read
      `jobs.description` directly rather than going through `JobDocument`.
      **Done**: kept the single `FromJob` build call (avoids duplicating it in
      `cmd/embed/indexer.go`) but added a new unexported `JobDocument.semanticText`
      field, computed from the full `view.Description` via `stripToPlainText`
      BEFORE the existing `maxIndexedDescriptionRunes` truncation runs.
      `jobPassage` now reads only `semanticText` — the old
      Description/Enrichment.Summary fallback is gone (design.md Decision 5).
      Reviewed: ready to merge, no Critical/Important issues.

## 3. Chunking

- [x] 3.1 Add a chunker: splits cleaned text into ~2000-rune (design.md Decision 2)
      chunks, preferring paragraph/sentence boundaries. RED tests: a short
      description yields 1 chunk; a long one yields N chunks with no chunk
      grossly exceeding the budget; a chunk boundary doesn't split a word.
      **Done** (`internal/search/chunk.go`: `chunkText` + `wrapWords` +
      `chunkBudgetRunes`). Splits on `stripToPlainText`'s paragraph newlines,
      falls back to word-boundary wrapping for a single over-budget paragraph
      — matches design.md Decision 2's actual mechanism (no separate
      sentence-splitting step). Reviewed: tightened two test assertions that
      were too loose to catch an off-by-one in the packing guard (exact
      chunk/paragraph-count assertions instead of a padded tolerance and a
      substring `Contains` check); verified the tightened test actually goes
      red under a small budget-math mutation before reverting it. Pure
      function, not yet wired into the embed pipeline (task 4).

## 4. Multi-vector storage

- [x] 4.1 ~~Migration changing `jobs.semantic_embedding`'s shape~~ **No migration
      needed — resolved 2026-08-09.** Verified empirically (disposable Postgres 18
      container): a `real[]`-declared column already stores and returns a 2D array
      without complaint, since Postgres does not enforce declared array
      dimensionality. Real problem found instead: `db.Job.SemanticEmbedding` is one
      Go field scanned by ~a dozen query sites across `internal/db/jobs.sql.go`,
      including ordinary job-listing/detail queries, not just the embed pipeline —
      flipping it straight to `[][]float32` would error-scan every row still
      holding old single-vector data for the whole re-embed drain window (a
      whole-site failure, not a search-only one). Resolved per the user's explicit
      ask ("fallback to 1 if there's no several, like now"): `sqlc.yaml` gets an
      `overrides:` entry (`jobs.semantic_embedding` → `pgtype.Array[float32]`,
      same mechanism as the existing `jobs.enrichment` override) instead of a
      concrete `[][]float32`, so scanning never fails regardless of which shape a
      row holds. See design.md Decision 3 for the full writeup.
- [x] 4.2 Add `sqlc.yaml`'s `jobs.semantic_embedding` override; regenerate
      (`make sqlc`). Add `internal/search.SemanticVectorsFromArray`
      (`pgtype.Array[float32]` → `[][]float32`, RED test first: a 1-dimension
      array reshapes to a single-chunk list — byte-for-byte the old behavior —
      a 2-dimension array reshapes to its real rows, empty/invalid reshapes to
      nil) and `SemanticVectorsToArray` (the inverse, for the write side).
      **Done** as `internal/search/semantic_chunks.go`'s `SemanticChunksFromArray`/
      `SemanticChunksToArray` (named to avoid a clash with the pre-existing,
      unrelated `SemanticVector` type in `semantic_vectors.go`). Also had to
      drop `SetSemanticEmbedding`'s `::real[]` param cast
      (`internal/db/queries/semantic.sql`) — with the cast, sqlc typed the
      param from the literal cast instead of tracing it to the overridden
      column, so only the SELECT side picked up `pgtype.Array[float32]`; the
      untyped `sqlc.arg(embedding)` form makes sqlc trace both sides
      consistently. `make sqlc` (via Docker) also regenerated unrelated
      pre-existing drift on `main` (missing structs/methods for an unrelated
      Adzuna-hydration feature and `rate_limits`, apparently never committed
      after their own migrations) — manually reverted everything outside the
      `Job.SemanticEmbedding` field to keep this diff surgical; that drift is
      real but out of this change's scope.
- [x] 4.3 Update `internal/embed`'s Store/Indexer ports and `cmd/embed`'s wiring
      for the multi-vector shape.
      **Done**: `Store.CompleteOpen`/`Indexer.IndexOpen` now trade in
      `map[int64][][]float32`; `cmd/embed/store.go`'s `SetSemanticEmbedding`
      call converts via `search.SemanticChunksToArray`. Verified end-to-end
      against real Postgres + Meilisearch
      (`go test -tags=integration ./cmd/embed/...`).
- [x] 4.4 Update `internal/search` (`IndexSemanticJobs`, `_vectors` construction,
      `JobDocument.semanticVector`) for multi-vector documents.
      **Done**: `JobDocument.semanticVector` → `semanticVectors [][]float32`;
      `jobPassage` → `jobPassages` (one embedding input per chunk of
      `semanticText`, via task 3's `chunkText`); `embedDocs` embeds every
      doc's chunks in one batched call and regroups by position (a job with
      zero chunks — no description text — is dropped from the result, not
      given an empty vector list); `semanticDocument.Vectors` is
      `map[string][][]float32`, serialized as a BARE array-of-arrays under
      `_vectors.default` — verified directly against a disposable
      Meilisearch v1.49.0 instance that this shorthand (no
      `{"embeddings": [...]}` wrapper) is accepted and scores correctly.
      Review follow-on: found and fixed a latent data-loss bug this change
      would have introduced in the separate `cmd/backfill-semantic-vectors`
      disaster-recovery tool — its Meili→Postgres copy only ever read
      `embeddings[0]`, so re-running it after a job was re-embedded under the
      chunked scheme would have silently truncated that job's vectors back
      to one. Fixed (`SemanticVector.Vectors [][]float32`,
      `parseSemanticVectorsPage` keeps all chunks, `pgSink.Save` writes all
      of them) and covered by both unit and integration tests.
- [x] 4.5 Update `reindex --semantic --from-pg` to read/republish multi-vector
      rows from Postgres.
      **Done**: `semanticDocsFromPG` needed no changes beyond 4.4's
      `semanticVectors` rename — it already just re-serializes whatever the
      document carries. Verified with a genuine 2-chunk case added to
      `TestIntegration_SemanticRebuildFromPG` (real Meilisearch): both chunks
      land in the index, in order, confirmed by reading the raw document back
      via `GetDocument(..., RetrieveVectors: true)` — not just presence, but
      chunk 1 vs. chunk 2 correctly distinguished.

## 5. Force re-embed

- [x] 5.1 Bump `embedderModel` in `internal/search/client.go` to a versioned
      value (design.md Decision 4). Confirm via integration test that
      `EnqueuePendingSemanticJobs` re-queues previously-current jobs after the
      bump (mirrors the existing "model-stale job is re-enqueued" test in
      `internal/db/semantic_integration_test.go`).
      **Done**: `embedderModel = "intfloat/multilingual-e5-base@chunked-v1"`.
      Added `TestIntegration_EmbedWorkerReEmbedsOnModelBump`
      (`cmd/embed/embed_integration_test.go`) — full-pipeline coverage beyond
      the existing SQL-predicate test: seeds a job stamped under the OLD
      model with a matching content_hash (isolating model-mismatch as the
      sole trigger), runs the real worker against real Postgres +
      Meilisearch, confirms re-embed + stamp update + index presence.
      Reviewed: ready to merge.
      **Reminder (task 0.1): do not deploy/merge this bump until the
      in-flight prod backfill finishes.**

## 6. Verify

- [x] 6.1 `go build/vet ./...`, `go test ./...`, `go vet -tags=integration ./...`.
      **Done, all clean** (2026-08-09).
- [x] 6.2 `go test -tags=integration ./internal/db/... ./cmd/embed/...
      ./internal/embed/... ./internal/search/...`.
      **Done, all pass against real Postgres + Meilisearch via testcontainers**
      (2026-08-09): `internal/db` 40s, `cmd/embed` 10s, `internal/embed` 0.4s
      (no integration tests, unit-only), `internal/search` 30s.
- [x] 6.3 End-to-end spot check on a real job with a long, HTML-heavy
      description: confirm multiple chunks, confirm the LAST chunk's content
      (something that would have been truncated before) is actually present in
      one of the stored vectors.
      **Done** as `TestEndToEnd_LongHTMLDescriptionReachesLastChunk`
      (`internal/search/embed_test.go`): a realistic HTML description
      (headings/paragraphs/lists, >1000 runes — past the old facet-index cap)
      with a distinctive marker placed at the very end, run through the real
      `FromJob` → `jobPassages` path. Confirms the marker is absent from the
      facet-index `Description` (correctly still capped) but present in the
      LAST embedding passage, proving both that nothing past the old cap is
      lost and that chunk ordering is preserved end to end.
