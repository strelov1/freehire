## Context

Recent closure handling uses `is_closed` and edit-documents-by-function to retain text/vectors. Duplicate transitions still take the physical-removal branch, and link import submits a second unawaited deletion even though MarkJobDuplicateOfRole already queues the transition atomically.

## Decisions

Use a true-or-absent `is_duplicate` marker beside `is_closed`. Keep duplicate identity authoritative in PostgreSQL; the index needs only visibility. Apply both exclusions at the existing common query boundary, including search, vector/federated queries, facets, distinct counts and sitemap documents. Missing legacy markers remain eligible.

PrepareSearchDeletions returns closed/duplicate flags plus hard_delete, which now means only a missing or private row. The drain updates existing closed and duplicate documents, preserving text/vectors and creating no skeletons, and physically deletes missing/private documents. Both marks apply when both conditions hold. The existing acknowledgment repair requeues a now-canonical open job after a concurrent release. Full replacement omits false markers and clears stale ones.

Remove linkimport.unindex and SubmitJobDeletion: the import transaction already queues canonical-to-duplicate transitions, and the shared worker rereads state rather than acting on a stale verdict. Existing pending-removal filtering covers the delay before the status wave.

Retain the compact rebuild policy: only open canonical public documents enter the new index. Retention buys cheap incremental transitions between rebuilds; it is not a permanent archive in Meilisearch. Rejected alternative: copy all duplicates into every rebuild, increasing scan, indexing and vector storage costs without evidence of benefit. Physical cleanup of private/missing rows and old rebuild indexes remains necessary.

## Verification

Real Meilisearch tests prove visibility, counts, sitemap exclusion, retained vectors/text, no skeletons, idempotency and restoration. PostgreSQL/drain tests prove current-state classification and requeue on concurrent release. Import tests prove durable queueing without a direct engine delete. Run unit/vet suites plus affected integration tests.

## Rollout

Before deploying, append `is_duplicate` to the live jobs index filterable attributes, preserve all other settings, and await the settings task. The existing editDocumentsByFunction feature remains required. Deploy readers and writers together. No full rebuild is required. Measure production queue/task behavior before claiming a performance gain. Rollback must first physically remove documents matching `is_duplicate = true`, await completion, then revert readers/writers; legacy readers would otherwise expose retained duplicates.
