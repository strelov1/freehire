## MODIFIED Requirements

### Requirement: Open jobs are embedded and persisted

The system SHALL embed each claimed open job's document (corpus/`passage:` form)
and persist the resulting vector to `jobs.semantic_embedding` and
`jobs.semantic_embedding_vec` (the pgvector-typed column used for nearest-neighbour
queries). On success it MUST stamp the job's `semantic_embedded_model` and
`semantic_embedded_hash`, clear `jobs.similar_computed_at` (so the similar-jobs
backfill worker picks the job back up), and delete the outbox entry in a single
transaction, so a crash between the vector write and the stamp is safely retried
(idempotent re-embed).

#### Scenario: Open job embedded and stamped

- **WHEN** the worker processes a claimed entry for an open job
- **THEN** the job's vector is persisted to `jobs.semantic_embedding` and
  `jobs.semantic_embedding_vec`, its `semantic_embedded_model`/
  `semantic_embedded_hash` are stamped, `similar_computed_at` is cleared, and the
  outbox entry is deleted

#### Scenario: Newly embedded job becomes retrievable

- **WHEN** a previously un-embedded open job is processed
- **THEN** its vector is available for the similar-jobs backfill worker and for
  live recommendation queries without any Meilisearch rebuild

### Requirement: Closed jobs have their embedding cleared

The system SHALL clear a job's persisted embedding (`jobs.semantic_embedding`,
`jobs.semantic_embedding_vec`) and its provenance stamps when the job has closed
after being embedded, so semantic retrieval never surfaces a dead posting. The
claim path MUST NOT filter closed jobs out; the worker branches on the job's
state.

#### Scenario: Closed embedded job is cleared

- **WHEN** the worker processes a claimed entry whose job is now closed and was
  previously embedded
- **THEN** the job's persisted embedding and provenance are cleared and the
  outbox entry is deleted

### Requirement: Pipeline is decoupled from ingest

The system SHALL keep incremental embedding independent of the ingest write
path. Ingest (`UpsertJob`) MUST NOT be coupled to embedding provenance.

#### Scenario: Ingest write does not embed

- **WHEN** a job is ingested or updated via `UpsertJob`
- **THEN** no embedding is performed on the ingest path; the job is picked up by
  the next enqueue based on its `content_hash`

## REMOVED Requirements

### Requirement: Pipeline is decoupled from the full rebuild

**Reason**: The `jobs_semantic` Meilisearch index and its `reindex --semantic`
swap-rebuild reconciler are removed entirely — `jobs.semantic_embedding`/
`semantic_embedding_vec` in Postgres are now the only representation of job
embeddings, with no secondary index to reconcile.

**Migration**: No user-facing migration. Operationally, any cron/timer that ran
`reindex --semantic` is removed; the similar-jobs backfill worker
(`cmd/similar-backfill`) and its own cadence take over as the relevant
"catch up derived state" job for what `reindex --semantic --from-pg` used to
partially serve.
