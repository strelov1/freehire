## 1. Schema & generated access

- [x] 1.1 Add migration `migrations/00NN_semantic_outbox.sql`: `semantic_outbox` table (mirror `enrichment_outbox`: id IDENTITY, job_id, `target_model text`, attempts, claimed_at, failed_at, last_error, created_at; `UNIQUE(job_id, target_model)`; index to support the claim join/order) and `ALTER TABLE jobs ADD COLUMN semantic_embedded_model text, ADD COLUMN semantic_embedded_hash text`.
- [x] 1.2 Write `internal/db/queries/semantic.sql` with the query stubs (`EnqueuePendingSemanticJobs`, `ClaimSemanticBatch`, `StampSemanticEmbedded`, `ClearSemanticEmbedded`, `DeleteSemanticEntry`, `RecordSemanticFailure`), mirroring `enrichment.sql`.
- [x] 1.3 Run `make sqlc`; confirm `go build ./...` and `go vet ./...` pass with the generated code.

## 2. Enqueue (add / update / model-stale / non-tech exclusion / closed-removal)

- [x] 2.1 Integration test (`//go:build integration`) for `EnqueuePendingSemanticJobs`: never-embedded open job enqueued; content-changed job (hash mismatch) enqueued; up-to-date job (model+hash match) not enqueued; non-tech category excluded; repeated enqueue does not duplicate (ON CONFLICT); closed-but-still-embedded job enqueued for removal.
- [x] 2.2 Implement the `EnqueuePendingSemanticJobs` SQL to pass 2.1 (predicate: `semantic_embedded_model IS DISTINCT FROM target OR semantic_embedded_hash IS DISTINCT FROM content_hash` for open jobs, plus closed-with-stamp rows; `category <> ALL(exclude_categories)`; `ON CONFLICT (job_id, target_model) DO NOTHING`).

## 3. Claim, stamp, clear, failure

- [x] 3.1 Integration test for `ClaimSemanticBatch`: freshest-first ordering; lease + `FOR UPDATE OF o SKIP LOCKED` disjoint claims; expired-lease reclaim; **closed jobs are returned** (not filtered out).
- [x] 3.2 Implement `ClaimSemanticBatch` to pass 3.1 (join jobs, order by `COALESCE(posted_at, created_at) DESC, id DESC`, lease predicate; do NOT filter `closed_at`).
- [x] 3.3 Integration test + implementation for `StampSemanticEmbedded(job_id, model, hash)`, `ClearSemanticEmbedded(job_id)`, `DeleteSemanticEntry(id)`, and `RecordSemanticFailure(id, last_error, max_attempts)` (attempts bump + dead-letter at max), mirroring `RecordEnrichmentFailure`.

## 4. `cmd/embed` worker

- [x] 4.1 Pin the target-model identity to one source of truth in `internal/search` (a constant/accessor for the e5 model string used for both embedding and the stamp); unit test that it is stable/non-empty.
- [x] 4.2 Scaffold `cmd/embed/main.go` on `worker.Main`/`worker.Bootstrap`: require `MEILI_MASTER_KEY` + embedder config; build `search.NewClient` and `db.Queries`; call `EnqueuePendingSemanticJobs(target_model, enrich.NonTechCategories)`.
- [x] 4.3 Implement the claim-wave drain loop (concurrency = `EMBED_CONCURRENCY`, per-call timeout, wave sized to concurrency), mirroring `cmd/enrich`'s runner shape. Add unit tests for the per-job branch decision (open vs. closed) against a fake store/indexer.
- [x] 4.4 Open-job path: `search.FromJob(persisted row)` → `IndexSemanticJobs` (embed `passage:` + in-place upsert with `_vectors`) → in one PG txn `StampSemanticEmbedded` + `DeleteSemanticEntry`.
- [x] 4.5 Closed-job path: `DeleteSemanticJobs([id])` → in one PG txn `ClearSemanticEmbedded` + `DeleteSemanticEntry`.
- [x] 4.6 Failure path: on embed/index/delete error, `RecordSemanticFailure` (retry-then-dead-letter); a single failure never aborts the wave/run.

## 5. End-to-end & docs

- [x] 5.1 Integration test (testcontainers PG + Meili) driving the full loop: seed open + closed jobs → enqueue → drain → assert open job vector present in `jobs_semantic` and retrievable, closed job removed, provenance stamped/cleared, outbox drained.
- [x] 5.2 Update `AGENT.md`: add `cmd/embed` to Layout + Commands, and a "Conventions and gotchas" entry describing the semantic-embedding outbox (mirrors the enrichment convention: reconciler = `reindex --semantic`; incremental = `cmd/embed`).
- [x] 5.3 Final `go build ./...`, `go vet ./...`, `go test ./...`, and `go test -tags=integration ./...` (where Docker available) all green.
