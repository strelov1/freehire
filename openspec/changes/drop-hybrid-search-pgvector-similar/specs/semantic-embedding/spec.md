## MODIFIED Requirements

### Requirement: Job descriptions are embedded as HTML-free, full-length chunks

The system SHALL derive the text embedded for a job from its description with
HTML markup stripped to plain prose (not the render-safety-sanitized copy served
to the site), covering the FULL description rather than any facet-index-only
truncation. The plain text SHALL be split into one or more chunks bounded by a
conservative size budget so each chunk stays within the embedding backend's
input limits; each chunk SHALL be embedded independently, so a job's semantic
representation is the full description in every case, not just its opening
portion.

#### Scenario: A short description yields one chunk

- **WHEN** a job's plain-text description fits within one chunk's size budget
- **THEN** exactly one embedding chunk is produced for it

#### Scenario: A long description yields multiple chunks covering the whole text

- **WHEN** a job's plain-text description exceeds one chunk's size budget
- **THEN** it is split into multiple chunks at paragraph (or, failing that, word)
  boundaries, and every chunk is embedded — content past the first chunk is
  still represented, not silently dropped

#### Scenario: HTML markup does not reach the embedding model

- **WHEN** a job's raw description contains HTML markup
- **THEN** the text embedded is plain prose with the markup removed, and
  paragraph/list/table-row boundaries are preserved as chunk-splitting hints
  rather than being glued into run-on text

### Requirement: Open jobs' embedding chunks are persisted

The system SHALL embed each claimed open job's chunks (per the chunking
requirement above) and persist the resulting vectors as that job's rows in
`job_semantic_chunks` (one row per chunk, replacing any previous rows for that
job in the same transaction). On success it MUST stamp the job's
`semantic_embedded_model` and `semantic_embedded_hash`, clear
`jobs.similar_computed_at` (so the similar-jobs backfill worker picks the job
back up), and delete the outbox entry — all in a single transaction, so a crash
between the chunk writes and the stamp is safely retried (idempotent re-embed).

#### Scenario: Open job's chunks are embedded and stamped

- **WHEN** the worker processes a claimed entry for an open job
- **THEN** the job's chunk embeddings replace its prior rows in
  `job_semantic_chunks`, its `semantic_embedded_model`/`semantic_embedded_hash`
  are stamped, `similar_computed_at` is cleared, and the outbox entry is deleted

#### Scenario: Newly embedded job becomes retrievable

- **WHEN** a previously un-embedded open job is processed
- **THEN** its chunk embeddings are available for the similar-jobs backfill
  worker without any Meilisearch rebuild

### Requirement: Closed jobs have their embedding chunks cleared

The system SHALL delete a job's `job_semantic_chunks` rows and clear its
provenance stamps when the job has closed after being embedded, so semantic
retrieval never surfaces a dead posting. The claim path MUST NOT filter closed
jobs out; the worker branches on the job's state.

#### Scenario: Closed embedded job's chunks are cleared

- **WHEN** the worker processes a claimed entry whose job is now closed and was
  previously embedded
- **THEN** the job's `job_semantic_chunks` rows are deleted, its provenance is
  cleared, and the outbox entry is deleted

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
swap-rebuild reconciler are removed entirely — `job_semantic_chunks` in
Postgres is now the only representation of job embeddings, with no secondary
index to reconcile.

**Migration**: No user-facing migration. Operationally, any cron/timer that ran
`reindex --semantic` is removed; the similar-jobs backfill worker
(`cmd/similar-backfill`) and its own cadence take over as the relevant
"catch up derived state" job for what `reindex --semantic --from-pg` used to
partially serve.
