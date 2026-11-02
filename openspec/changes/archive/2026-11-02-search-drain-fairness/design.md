## Context

The production drain can spend over fifteen minutes indexing tiny waves after deleting one wave. Most pending deletion entries can already refer to reopened jobs. TouchJob reopens without enqueueing search.

## Goals / Non-Goals

Goals: fair wave scheduling, bounded work per queue, and eventual repair for reopening before or during deletion. Non-goals: changing rebuild concurrency or filtering already paginated search results in Postgres.

## Decisions

Keep Runner and DeletionRunner and reuse their timeout/fallback handling. A package-level Drain alternates one wave of each, deletion first, counts claimed entries toward each queue's independent MaxPerRun, and stops a queue after a short wave. Short waves end that queue's participation so fresh trickles accumulate for the next scheduled run. Preserve existing standalone Run callers.

The deletion adapter reads current job state immediately before sending ids: closed or missing jobs remain eligible, currently open jobs do not. Acknowledging a removal atomically requeues any currently open public canonical jobs, including a job reopened during the Meilisearch call, then drops the old removal. TouchJob locks and materializes the prior row state before updating it, and enqueues only on a closed-to-open transition, covering reopening after acknowledgment and a concurrent close that commits while the touch waits. Existing upserts already requeue reopens via their changed-content signal.

Use native `UpdateDocumentsByFunctionWithContext` to set `doc.is_closed = true` on existing documents selected by id. This preserves text/vectors and cannot insert skeletal documents for jobs absent from the index. It requires the engine's experimental `editDocumentsByFunction` feature; enabling is an explicit rollout operation, not a side effect of constructing a client. Private, noncanonical, and missing jobs retain physical removal.

The internal marker is true-or-absent. `NOT is_closed = true` accepts legacy documents without the field. Every full replacement of an open document omits and therefore clears the marker. Apply the mandatory predicate at the search client boundary for keyword/vector/federated queries, facet/disjunctive counts, distinct counts, and job sitemaps. It stays out of the public facet vocabulary and response shape.

## Risks / Trade-offs

- Reopening during an in-flight deletion can temporarily remove the document; the acknowledgment repair makes it eligible for the next index wave. Crash before acknowledgment retains the deletion lease for retry and repair.
- A queue that receives new work after its short wave waits until the next invocation. This avoids paying a full index mutation for each trickle.
- Rebuild still pauses both queues. Its concurrency remains gated on a separate production-scale experiment.

## Migration Plan

No schema migration. Before deploying the reader/worker binary:

1. Enable `editDocumentsByFunction` via PATCH `/experimental-features` with that field set to true. The API is experimental in the engine and must be checked on engine upgrades.
2. Read the live jobs index's existing filterable attributes, append `is_closed`, PATCH `/indexes/jobs/settings/filterable-attributes`, and await the resulting task. Preserve every existing attribute and all embedder settings.
3. Deploy the reader and worker together; retain the current batch size and per-run budget. Do not remove the existing rebuild pause based on the small-copy benchmark.
4. Measure task duration, queue age, and backlog change before deciding rebuild-time concurrency. Production-scale acceleration is unproven.

Rollback: first physically delete documents matching `is_closed = true` and await completion, then revert the reader/worker together. An old reader does not know this predicate and would otherwise expose retained closed documents. The extra filterable setting and enabled engine feature can remain during rollback.
