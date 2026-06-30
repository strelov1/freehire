## ADDED Requirements

### Requirement: The write path reports new and changed writes for indexing

The job write path SHALL maintain a deterministic content hash over a job's
indexed fields and SHALL report, for each write, whether it **inserted** a new
row or **changed** the indexed content of an existing one. An upsert that updates
only bookkeeping (such as the last-seen timestamp) without changing any indexed
field SHALL report neither inserted nor changed. The hash SHALL be derived from
the same persisted values that form the search document, so the signal tracks
exactly what the index would need re-pushed.

#### Scenario: A first-time write reports inserted

- **WHEN** a posting not previously in the catalogue is persisted
- **THEN** the write reports it as inserted

#### Scenario: An edited write reports changed

- **WHEN** an existing posting is persisted with an edited indexed field (e.g. its
  title)
- **THEN** the write reports it as changed

#### Scenario: A no-op refresh reports neither

- **WHEN** an existing posting is persisted with identical indexed fields and only
  its last-seen timestamp advances
- **THEN** the write reports neither inserted nor changed

### Requirement: Ingest feeds new and changed jobs to the search index

When the search engine is configured for the ingest worker, the worker SHALL push
the open jobs reported as inserted or changed to the live facet search index,
batched rather than one document per posting, after they are persisted. The push
SHALL be best-effort: a search-index error SHALL be logged and SHALL NOT fail the
ingest run or change its exit code. When the search engine is not configured for
the worker, ingest SHALL run unchanged and skip indexing.

#### Scenario: Configured worker indexes new and changed jobs

- **WHEN** a crawl persists new and edited jobs and the search engine is configured
- **THEN** those jobs' documents are pushed to the live facet index in batches

#### Scenario: Unconfigured worker skips indexing

- **WHEN** ingest runs without a configured search engine
- **THEN** ingest persists and sweeps as before and pushes nothing to the index

#### Scenario: Index error does not fail the run

- **WHEN** pushing documents to the search engine fails
- **THEN** the error is logged and the ingest run's success is unaffected
