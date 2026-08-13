## 1. Migration and column backfill

- [ ] 1.1 New migration (next number off `origin/main`'s `migrations/`, re-check at
      implementation time — numbers have collided across branches before):
      `CREATE EXTENSION IF NOT EXISTS vector;` + `ALTER TABLE jobs ADD COLUMN
      semantic_embedding_vec vector(768)`, `ADD COLUMN similar_job_ids bigint[]`,
      `ADD COLUMN similar_computed_at timestamptz` — all nullable, no default (instant).
- [ ] 1.2 One-off batched backfill script/command: convert existing
      `jobs.semantic_embedding` (`real[]`) rows into `semantic_embedding_vec`
      (`vector(768)`), batched, resumable, mirroring `cmd/backfill-semantic-vectors`'s
      shape.
- [ ] 1.3 `internal/db/queries/*.sql`: add/update sqlc queries for the new columns;
      `make sqlc` (or `~/go/bin/sqlc generate` if Docker unavailable).

## 2. `cmd/embed` writes the new column and clears staleness

- [ ] 2.1 `cmd/embed`'s open-job stamp transaction also persists
      `semantic_embedding_vec` and clears `similar_computed_at` (so a re-embedded job
      is picked back up by the similar-jobs backfill worker).
- [ ] 2.2 `cmd/embed`'s closed-job path also clears `semantic_embedding_vec`
      alongside the existing `semantic_embedding` clear.
- [ ] 2.3 Unit + integration tests for both paths (mirror the existing
      `semantic_embedding` stamp/clear tests).

## 3. HNSW index

- [ ] 3.1 Build `CREATE INDEX CONCURRENTLY ... USING hnsw (semantic_embedding_vec
      vector_cosine_ops)` on prod, in a scheduled low-load window, with a raised
      session-scoped `maintenance_work_mem` — measure actual build time/disk on prod
      rather than trusting the session's own (contended, bug-affected) local spike
      numbers; treat this as a monitored operation like a Meili full rebuild.

## 4. `cmd/similar-backfill` worker

- [ ] 4.1 New package (e.g. `internal/similarjobs`) with Store/Indexer-style ports,
      unit-tested with fakes, mirroring `internal/embed`'s shape.
- [ ] 4.2 Query: jobs with `semantic_embedding_vec IS NOT NULL AND
      similar_computed_at IS NULL` (or stale), batched, ordered sensibly (e.g.
      freshest-first like the embed claim).
- [ ] 4.3 Per job: one pgvector nearest-neighbour query
      (`ORDER BY semantic_embedding_vec <=> $1 LIMIT N`, excluding self and closed
      jobs), write `similar_job_ids` + stamp `similar_computed_at`.
- [ ] 4.4 `cmd/similar-backfill/main.go`: run-once-and-exit worker (this repo's
      standard `internal/worker` Bootstrap convention), flags mirroring telagon's
      (`-batch`, `-workers`, `-limit`) where they make sense here.
- [ ] 4.5 Integration test: end-to-end, a job with an embedding gets a correct
      `similar_job_ids` list written.

## 5. Switch `/similar` to the precomputed lookup

- [ ] 5.1 `internal/handler/similar.go`: read `jobs.similar_job_ids` for the
      resolved job id, fetch + filter to open jobs, project to `jobview.Job`, keep
      the same response envelope and `limit` clamping behavior.
- [ ] 5.2 Update/replace `internal/search/client.go`'s `SimilarJobs` Meili
      implementation — remove once the handler no longer calls it (see Section 7).
- [ ] 5.3 Update `similar_integration_test.go` and unit tests for the new data path.

## 6. Switch `/me/recommendations` to a live pgvector query

- [ ] 6.1 New query: CV vector (from the caller's persisted embedding) +
      the existing facet-filter-to-SQL translation, `ORDER BY
      semantic_embedding_vec <=> $1 LIMIT/OFFSET`, filtered to open jobs.
- [ ] 6.2 Wire the handler to the new query instead of `search.Client.RecommendByVector`.
- [ ] 6.3 Update recommendation tests (unit + integration) for the new path;
      verify facet-filter scenarios from the `cv-recommendations` spec still hold.

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
- [ ] 8.3 Run the `semantic_embedding_vec` backfill on prod.
- [ ] 8.4 Build the HNSW index on prod (Section 3.1) in a scheduled window.
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
