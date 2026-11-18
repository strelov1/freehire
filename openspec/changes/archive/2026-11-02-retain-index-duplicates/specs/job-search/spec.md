## MODIFIED Requirements

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


## ADDED Requirements

### Requirement: Duplicate jobs are hidden by indexed status

A posting newly marked as a duplicate SHALL have its existing Meilisearch document marked `is_duplicate = true` through the durable state-checked removal queue rather than physically deleted. Updates SHALL preserve text and vectors, change existing documents only, and be idempotent. All job searches, vector/federated queries, facets, distinct counts and job sitemaps SHALL exclude marked duplicates before pagination. Legacy documents lacking the marker SHALL remain eligible. The internal marker SHALL not enter the public job response or facet vocabulary.

The index document built from a database row SHALL carry its current duplicate status. Replacing an open canonical document SHALL clear stale closure and duplicate markers. A job released from duplication while an update is in flight SHALL be requeued for indexing on acknowledgment. Missing/private rows SHALL still be physically removed. Full rebuilds SHALL continue to omit duplicate and closed documents and discard obsolete indexes.

A link import's duplicate transition SHALL use its existing atomic removal outbox and SHALL NOT submit a separate direct index deletion.

#### Scenario: A duplicate retains its indexed content
- **WHEN** a searchable posting becomes a duplicate and the status wave completes
- **THEN** its text and vectors remain stored but it contributes no hits, facets, distinct counts or sitemap entries

#### Scenario: An absent duplicate document creates no skeleton
- **WHEN** a duplicate status wave includes a job absent from the index
- **THEN** no document is created

#### Scenario: A released duplicate returns to search
- **WHEN** a duplicate becomes canonical before or during its status wave
- **THEN** it is queued for current-content replacement and becomes searchable with stale markers cleared

#### Scenario: A private or missing duplicate is physically removed
- **WHEN** a queued duplicate is private or its database row is missing
- **THEN** its document is physically removed regardless of duplicate status

#### Scenario: Link import has one durable duplicate update
- **WHEN** reimporting a previously searchable generic URL identifies an existing canonical posting
- **THEN** the marking transaction queues the duplicate and the importer submits no direct engine deletion
