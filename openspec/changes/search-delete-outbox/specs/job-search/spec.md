## ADDED Requirements

### Requirement: Closed jobs leave the facet index incrementally

The system SHALL remove a closed job's document from the live facet index without waiting
for the next batch reindex, so a vacancy that is no longer open stops matching search within
one drain cycle.

Removal SHALL be driven by a queue written in the same database transaction that closes the
job, so that a close and its pending removal cannot diverge. Because jobs are closed in bulk
— one statement closing every posting a crawl no longer saw — the enqueue SHALL be part of
the closing statement itself rather than a separate call per row.

A job removed from the catalogue outright SHALL also leave the index, on the same path. The
queue therefore SHALL NOT depend on the job's row still existing: a queued removal identifies
the document by key alone, and MUST survive the deletion of the job it refers to. A queue that
cascaded with the job would lose exactly the removals it exists to guarantee.

Removal SHALL be idempotent: removing a document that is not in the index is a no-op, so a
retried, overlapping, or duplicated removal is harmless and needs no coordination with the
indexing path.

A job that is queued both for indexing and for removal SHALL end up removed. The indexing
claim already excludes jobs that are not open, so a job closed after being queued for
indexing is never indexed by that entry.

Failure to remove SHALL NOT fail the run that closed the job, and SHALL leave the queue entry
for a later attempt rather than dropping it.

#### Scenario: A closed job stops matching search before the next batch reindex

- **WHEN** a crawl closes a job that was previously searchable
- **THEN** the job's document is removed from the live facet index and it no longer matches
  search, without waiting for a batch reindex

#### Scenario: A bulk close queues every job it closed

- **WHEN** one statement closes many jobs a crawl no longer saw
- **THEN** every closed job is queued for removal by that same statement

#### Scenario: A rolled-back close queues nothing

- **WHEN** the transaction that closes a job does not commit
- **THEN** no removal is queued for that job

#### Scenario: A hard-deleted job leaves the index

- **WHEN** a job is removed from the catalogue outright rather than closed
- **THEN** its removal is queued by the same statement that deleted it, and the document
  leaves the index

#### Scenario: A queued removal survives its job being deleted

- **WHEN** a job is queued for removal and its row is then hard-deleted before the queue is
  drained
- **THEN** the queued removal still exists and is still processed

#### Scenario: Removing an unindexed job is harmless

- **WHEN** a removal is processed for a job whose document is not in the index
- **THEN** the operation succeeds and the queue entry is completed

#### Scenario: A job queued for both indexing and removal ends up removed

- **WHEN** a job is queued for indexing and is then closed before that entry is drained
- **THEN** the job is not indexed by that entry, and its document is removed

#### Scenario: An unavailable search engine does not lose the removal

- **WHEN** the search engine is unavailable while a removal is being processed
- **THEN** the failure is logged, the queue entry remains for a later attempt, and the run
  that closed the job is unaffected

## MODIFIED Requirements

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
