# job-index-incremental Specification

## Purpose
How the jobs index stays current between rebuilds: new and changed jobs indexed incrementally, and closed jobs leaving the facet index. Split out of job-search.

## Requirements

### Requirement: Incremental indexing keeps new and changed jobs fresh

The system SHALL index a job into the live Meilisearch facet index as soon as
ingest persists it with new or changed indexed content, so a newly ingested or
edited open job becomes searchable within one crawl cycle rather than only after
the next scheduled batch reindex. A job whose indexed content did not change on a
re-ingest (for example, an upsert that only refreshes its last-seen timestamp)
SHALL NOT be re-pushed. This incremental path SHALL target the facet/keyword
production index only; the semantic index keeps its separate schedule.

Incremental indexing SHALL be best-effort and SHALL NOT change the source of
truth: the batch reindex (the "Batch reindex keeps the index in sync"
requirement) remains responsible for reconciliation. Removing the documents of
closed jobs is no longer part of that reconciliation alone — it also has its own
incremental path (the "Closed jobs leave the facet index incrementally"
requirement), so the batch reindex bounds how stale the index can drift rather
than bounding how long a closed job stays searchable. A failure to push to the
index SHALL NOT fail ingest.

#### Scenario: A newly ingested job is searchable before the next batch reindex

- **WHEN** ingest persists a job that was not previously in the catalogue
- **THEN** the job's document is present in the live facet index and the job
  matches search without waiting for a batch reindex

#### Scenario: An edited job is re-indexed on re-ingest

- **WHEN** a job already in the catalogue is re-ingested with an edited title or
  description
- **THEN** the job's document in the live facet index reflects the edit without
  waiting for a batch reindex

#### Scenario: An unchanged re-ingest does not re-push the document

- **WHEN** a job already in the catalogue is re-ingested with no change to its
  indexed content
- **THEN** no document push is issued for that job

#### Scenario: An index failure does not fail ingest

- **WHEN** the search engine is unavailable while ingest is pushing new documents
- **THEN** the ingest run records the persisted jobs and completes, and the
  failure is logged rather than aborting the run

### Requirement: Closed jobs leave the facet index incrementally

The system SHALL mark an ordinary closed posting's existing document with an internal `is_closed = true` status instead of physically deleting its text and vectors. All job search, vector/hybrid search, federated OR search, facet and distinct counts, and job sitemap queries SHALL exclude marked documents inside Meilisearch, before pagination. Documents without the marker SHALL remain eligible. Full replacement of a reopened job SHALL clear the marker.

Status updates SHALL use the queue written atomically with the closing statement, including bulk closes. They SHALL modify only existing documents, create no skeletal records, and be idempotent. Missing or private jobs SHALL retain physical removal; the queue SHALL survive a missing job row.

An entry for a still-closed job SHALL not be indexed by the indexing queue. Failed status updates SHALL retain the removal entry for retry without failing the ingest run that closed the job. A full rebuild SHALL omit closed documents, eventually reclaiming their storage. Rebuild-time queue scheduling remains independently configured.

#### Scenario: A closed job stops matching search before the next batch reindex
- **WHEN** a crawl closes a previously searchable posting and the status wave completes
- **THEN** its document remains stored but marked closed and absent from every public search/count surface

#### Scenario: A bulk close queues every job it closed
- **WHEN** one statement closes many jobs
- **THEN** every closed job is queued by that same statement

#### Scenario: A rolled-back close queues nothing
- **WHEN** the closing transaction does not commit
- **THEN** no removal is queued

#### Scenario: A hard-deleted job leaves the index
- **WHEN** the job row is removed before its queued removal is processed
- **THEN** its document is physically removed

#### Scenario: A queued removal survives its job being deleted
- **WHEN** a queued job row is hard-deleted
- **THEN** its queue entry survives and is processed

#### Scenario: Removing an unindexed job is harmless
- **WHEN** a queued removal names an absent index document
- **THEN** processing succeeds without creating a new document

#### Scenario: A job queued for both indexing and removal ends up removed
- **WHEN** a queued indexing entry's job closes before indexing
- **THEN** it is not indexed and its document is excluded by the status update

#### Scenario: An unavailable search engine does not lose the removal
- **WHEN** a status wave cannot complete
- **THEN** its queue entry remains for retry and the closing ingest run is unaffected

#### Scenario: A reopened document clears its marker
- **WHEN** an open posting replaces a previously marked document
- **THEN** the marker is cleared and the posting is searchable again
