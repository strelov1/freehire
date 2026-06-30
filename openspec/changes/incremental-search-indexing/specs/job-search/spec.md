## ADDED Requirements

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
requirement) remains responsible for reconciliation, including removing the
documents of closed jobs. A failure to push to the index SHALL NOT fail ingest.

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
