## 1. Migration

- [x] 1.1 **Edit migration 0092 in place** (never applied to a real/shared/persistent
      database yet — only replayed in ephemeral local/CI containers, so amending is
      safe here and avoids an add-then-drop column in shipped history): drop the
      `semantic_embedding_vec vector(768)` column from `jobs` (superseded — see
      design.md Decision 1), keep `CREATE EXTENSION IF NOT EXISTS vector;`, keep
      `jobs.similar_job_ids bigint[]`/`similar_computed_at timestamptz`. Add a new
      `CREATE TABLE job_semantic_chunks (job_id bigint NOT NULL REFERENCES
      jobs(id) ON DELETE CASCADE, chunk_index smallint NOT NULL, embedding
      vector(768) NOT NULL, PRIMARY KEY (job_id, chunk_index));`.
- [x] 1.2 **Delete** `cmd/backfill-semantic-embedding-vec/` and its integration test
      (`internal/db/semantic_embedding_vec_backfill_integration_test.go`) — the
      one-off reshape backfill it implemented no longer applies (design.md Decision
      1b: getting real chunked vectors needs a full re-embed via TEI, not a
      Postgres-side conversion of already-computed numbers). Remove the
      `SelectSemanticEmbeddingVecBackfillBatch`/`BackfillSemanticEmbeddingVecBatch`
      sqlc queries added for it. This is a deliberate reversal, not a bug — note it
      as such in the commit message.
- [x] 1.3 `internal/db/queries/*.sql`: sqlc queries for `job_semantic_chunks`
      (insert/replace a job's chunk rows, delete a job's chunk rows, the
      nearest-neighbour-over-chunks query from design.md Decision 5). `make sqlc`.

## 2. Chunked embedding pipeline

- [x] 2.1 Port `stripToPlainText` (`internal/search/plaintext.go`) and `chunkText`
      (`internal/search/chunk.go`) — plus their unit tests — verbatim from
      `origin/worktree-semantic-embed-full-clean-chunked` into wherever they now
      belong (likely `internal/embed`, since section 7 removes most of
      `internal/search`'s semantic code). No Meilisearch dependency in either
      function — should port with zero logic changes, only the package/import path.
- [x] 2.2 `cmd/embed`'s open-job path: build the plain-text chunks for the job's
      description, embed each chunk (existing TEI embed call, `passage:` prefix +
      title/company context per chunk — mirror `jobPassages` from the same branch),
      replace the job's `job_semantic_chunks` rows (delete-then-insert) in the same
      transaction as the stamp, clear `similar_computed_at`.
- [x] 2.3 `cmd/embed`'s closed-job path: delete the job's `job_semantic_chunks` rows
      alongside the existing stamp-clear.
- [x] 2.4 Unit + integration tests for 2.2/2.3 (mirror the existing
      `semantic_embedding` stamp/clear tests; add a multi-chunk case for a long
      description and a single-chunk case for a short one).
- [x] 2.5 Bump `search.CurrentEmbedderModel()`'s version string (forces the existing
      staleness check to re-enqueue the whole catalogue through the new pipeline —
      this is what makes the full re-embed happen, no separate backfill tooling).
      Actually flipping this on prod is a scheduled ops step (section 8), not bundled
      into a routine deploy — this task is just making the version bump possible in
      code (e.g. a version constant), not triggering it.

## 3. HNSW index

- [ ] 3.1 Build `CREATE INDEX CONCURRENTLY ... USING hnsw (embedding
      vector_cosine_ops)` on `job_semantic_chunks` on prod, in a scheduled low-load
      window, with a raised session-scoped `maintenance_work_mem`, only once there's
      real chunked data to index against (after the section 8 re-embed has made
      meaningful progress) — measure actual build time/disk on prod rather than
      trusting the session's own (contended, bug-affected) local spike numbers;
      treat this as a monitored operation like a Meili full rebuild.

## 4. `cmd/similar-backfill` worker

- [x] 4.1 New package (e.g. `internal/similarjobs`) with Store/Indexer-style ports,
      unit-tested with fakes, mirroring `internal/embed`'s shape.
- [x] 4.2 Query: jobs with at least one `job_semantic_chunks` row but
      `similar_computed_at IS NULL` (or older than the job's newest chunk), batched,
      ordered sensibly (e.g. freshest-first like the embed claim).
- [x] 4.3 Per job: the nearest-neighbour-over-chunks query from design.md
      Decision 5 — minimum cosine distance per candidate job across all
      (source chunk, candidate chunk) pairs, excluding the source job itself AND
      any job sharing its `company_slug`, excluding closed jobs, ordered by
      distance, limited to N — write `similar_job_ids` + stamp
      `similar_computed_at`.
- [x] 4.4 `cmd/similar-backfill/main.go`: run-once-and-exit worker (this repo's
      standard `internal/worker` Bootstrap convention), flags mirroring telagon's
      (`-batch`, `-workers`, `-limit`) where they make sense here.
- [x] 4.5 Integration test: end-to-end — a job with embedding chunks gets a correct
      `similar_job_ids` list written, a same-company candidate that would otherwise
      be the closest match is excluded and a different-company one appears instead,
      and a job whose only close matches are same-company yields a short/empty list
      rather than an error.

## 5. Switch `/similar` to the precomputed lookup

- [x] 5.1 `internal/handler/similar.go`: read `jobs.similar_job_ids` for the
      resolved job id, fetch + filter to open jobs, project to `jobview.Job`, keep
      the same response envelope and `limit` clamping behavior.
- [ ] 5.2 Update/replace `internal/search/client.go`'s `SimilarJobs` Meili
      implementation — remove once the handler no longer calls it (see Section 7).
- [x] 5.3 Update `similar_integration_test.go` and unit tests for the new data path.

## 6. Remove `/me/recommendations` entirely (mid-implementation reversal — see
      design.md Context; NOT migrated to pgvector)

- [ ] 6.1 Backend: delete `internal/handler/recommendations.go`, its route
      registration, and its tests. Delete the CV-embedding write path (résumé
      upload, `internal/resume`/`internal/handler/resume.go` — grep both for
      `Embedding`/`EmbedText` and remove what only served this feature; do not
      touch anything résumé upload needs for other reasons, e.g. skill
      extraction). Leave `search.Client.RecommendByVector` (Meili-backed) for
      section 7 to delete alongside the rest of the Meili semantic code — don't
      duplicate that removal here.
- [ ] 6.2 Frontend: remove the "Recommended" sort option and all `sort=cv`
      handling from the standalone jobs feed (`web/src/lib/components/JobsView.svelte`,
      `web/src/lib/facetModel.ts`, and `web/src/lib/api.ts`'s `recommendations()`
      call) — the sign-in/no-CV prompts, the URL round-trip, the whole CV-mode
      branch. A pre-existing `?sort=cv` link should fall back to the default
      "Newest" feed, not error.
- [ ] 6.3 Confirm nothing else references the removed endpoint/UI (grep
      `recommendations`/`sort=cv`/`sort.*cv` across `web/src` and `internal/`)
      and clean up any now-dead helper left behind.

## 7. Remove the old Meili semantic path

- [ ] 7.1 `internal/search/client.go`: remove `semanticSettings`,
      `EnsureSemanticIndex`, `NewSemanticRebuild`, `IndexSemanticJobs`,
      `IndexSemanticJobsFromPG`, `ResetSemanticIndex`, the old `SimilarJobs`/
      `RecommendByVector` Meili implementations, `semanticIndexUID`/
      `semanticRebuildUID` constants.
- [ ] 7.2 `cmd/reindex`: remove `--semantic`, `--from-pg`, `--posted-within` flags
      and their code paths.
- [ ] 7.3 `internal/embed`: remove the Meili-write half of the `Indexer` port
      (keep the Postgres-write half); update `internal/embed/AGENTS.md`.
- [ ] 7.4 `internal/handler/search.go`: remove `semantic_ratio` param handling,
      `defaultSemanticRatio`.
- [ ] 7.5 `web/src/lib/api.ts`: remove `semantic_ratio` from `searchJobs()`; update
      its doc comment.
- [ ] 7.6 `internal/search/AGENTS.md`: remove Meili semantic-index-specific
      guidance that no longer applies (reindex `--semantic` hazards, `jobs_semantic`
      composition/purge notes) — keep only what's still true of the facet/keyword
      `jobs` index.
- [ ] 7.7 Full repo search for remaining `semantic_ratio`/`jobs_semantic`/
      `SemanticRatio` references (docs, tests, ops scripts) and clean up or confirm
      intentionally kept (e.g. historical memory files are out of scope).

## 8. Prod ops

- [ ] 8.1 Install `postgresql-18-pgvector` on prod (host2).
- [ ] 8.2 Apply the migration (manual apply per this repo's deploy convention —
      new tables need a manual `psql` apply before/with the deploy, see
      `internal/db/AGENTS.md`).
- [ ] 8.3 Bump `embedderModel` (task 2.5) to trigger the full-catalogue re-embed
      through the new chunked pipeline — a scheduled, monitored operation (design.md
      flags this as a real, possibly multi-hour-to-multi-day TEI cost, not a toggle).
      Watch it drain via the existing `semantic_outbox` progress signals.
- [ ] 8.4 Once the re-embed has made meaningful progress, build the HNSW index on
      `job_semantic_chunks` on prod (Section 3.1) in a scheduled window.
- [ ] 8.5 Deploy `cmd/similar-backfill`; run its initial full pass; verify
      coverage before flipping `/similar` live (Section 5).
- [ ] 8.6 After `/similar` and `/recommendations` are verified on the new path:
      stop and remove `freehire-reindexw`-adjacent `--semantic` cron/timers, drop
      the live `jobs_semantic` Meili index, add a `cmd/similar-backfill` cron
      (cadence: default daily unless implementation reveals otherwise).

## 9. Documentation

- [ ] 9.1 Update the module table in `AGENTS.md` and `docs/architecture.md` if
      either references the removed semantic search flow.
- [ ] 9.2 Offer a changelog entry per this repo's "announce shipped work"
      convention once live.
