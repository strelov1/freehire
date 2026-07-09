## ADDED Requirements

### Requirement: Incremental enqueue of jobs needing embedding
The system SHALL enqueue into a `semantic_outbox` queue every open job whose current embedding is missing, content-stale, or model-stale, so the embedding worker processes only outstanding work rather than the whole catalogue. A job needs embedding when its `semantic_embedded_model` differs from the target model OR its `semantic_embedded_hash` differs from the job's current `content_hash`. Enqueue MUST be idempotent (at most one live entry per job for a given target model) and MUST exclude jobs whose derived category is in the configured non-tech exclusion set.

#### Scenario: Never-embedded open job is enqueued
- **WHEN** enqueue runs and an open job has `semantic_embedded_model IS NULL`
- **THEN** exactly one `semantic_outbox` entry is created for that job

#### Scenario: Content-changed job is re-enqueued
- **WHEN** an already-embedded job's `content_hash` no longer equals its `semantic_embedded_hash`
- **THEN** the job is enqueued for re-embedding

#### Scenario: Up-to-date job is not enqueued
- **WHEN** a job's `semantic_embedded_model` equals the target model AND its `semantic_embedded_hash` equals its `content_hash`
- **THEN** no new outbox entry is created for it

#### Scenario: Non-tech job is excluded
- **WHEN** enqueue runs with a non-tech exclusion set and a job's category is in that set
- **THEN** the job is not enqueued

#### Scenario: Repeated enqueue does not duplicate work
- **WHEN** enqueue runs twice with an entry still pending for a job at the target model
- **THEN** only one live entry exists for that job at that model

### Requirement: Leased, freshest-first claiming
The system SHALL claim a bounded batch of outbox entries per wave, freshest posting first, stamping a lease so concurrent workers take disjoint entries and a crashed worker's entries become reclaimable without a separate reaper. The claim MUST lock only outbox rows and skip already-locked ones.

#### Scenario: Concurrent workers take disjoint entries
- **WHEN** two workers claim batches at the same time
- **THEN** no outbox entry is handed to both workers in the same wave

#### Scenario: Expired lease is reclaimed
- **WHEN** an entry was claimed longer ago than the lease window and was never completed
- **THEN** a later claim wave may re-claim that entry

#### Scenario: Freshest jobs first
- **WHEN** a claim wave selects entries
- **THEN** entries are ordered by their job's effective posting date (newest first) so recent postings are embedded before older ones

### Requirement: Open jobs are embedded and upserted in place
The system SHALL embed each claimed open job's document (corpus/`passage:` form) and upsert the resulting vector into the live `jobs_semantic` index in place, without a swap rebuild. On success it MUST stamp the job's `semantic_embedded_model` and `semantic_embedded_hash` and delete the outbox entry in a single transaction, so a crash between the index write and the stamp is safely retried (idempotent re-embed).

#### Scenario: Open job embedded and stamped
- **WHEN** the worker processes a claimed entry for an open job
- **THEN** the job's vector is upserted into `jobs_semantic`, its `semantic_embedded_model`/`semantic_embedded_hash` are stamped, and the outbox entry is deleted

#### Scenario: Newly embedded job becomes retrievable
- **WHEN** a previously un-embedded open job is processed
- **THEN** it can be returned by semantic retrieval (`/similar`, recommendations) without a full `reindex --semantic`

### Requirement: Closed jobs are removed from the semantic index
The system SHALL remove a job's document from `jobs_semantic` when the job has closed after being embedded, and clear its embedding provenance, so semantic retrieval does not surface dead postings between full rebuilds. The claim path MUST NOT filter closed jobs out; the worker branches on the job's state.

#### Scenario: Closed embedded job is removed
- **WHEN** the worker processes a claimed entry whose job is now closed and was previously embedded
- **THEN** the job's document is deleted from `jobs_semantic`, its embedding provenance is cleared, and the outbox entry is deleted

### Requirement: Bounded retry with dead-lettering
The system SHALL retry a failed embed/index attempt on a later wave via lease expiry, and dead-letter the entry once a maximum attempt count is reached so a persistently failing job cannot loop forever. A transient failure MUST NOT abort the whole run.

#### Scenario: Transient failure retried later
- **WHEN** an attempt fails below the maximum attempt count
- **THEN** the entry's attempt count is incremented and it becomes eligible for a later wave rather than aborting the run

#### Scenario: Persistent failure dead-lettered
- **WHEN** an entry's attempts reach the configured maximum
- **THEN** the entry is marked failed and excluded from future claims

### Requirement: Pipeline is decoupled from ingest and the full rebuild
The system SHALL keep incremental embedding independent of the ingest write path and of `reindex --semantic`. Ingest (`UpsertJob`) MUST NOT be coupled to embedding provenance; `reindex --semantic` SHALL remain the reconciler that fully rebuilds and swaps the index (settings, at-scale model migration, compaction).

#### Scenario: Ingest write does not embed
- **WHEN** a job is ingested or updated via `UpsertJob`
- **THEN** no embedding is performed on the ingest path; the job is picked up by the next enqueue based on its `content_hash`

#### Scenario: Full rebuild remains available as reconciler
- **WHEN** `reindex --semantic` runs
- **THEN** it performs a full swap-rebuild of `jobs_semantic` independent of the outbox pipeline
