## Why

172,879 job postings are permanently excluded from the search index because the
LLM gateway was down when their turn came. Of those, **172,875 died to
`enrich: llm: …`** — HTTP 502s and 500s from LiteLLM during outages on 17 and 24
July, both long since fixed. Exactly **4** died to a fault of their own (an
unparseable model response).

`ENRICH_MAX_ATTEMPTS` is 3, and a claimed entry becomes re-claimable once its
300-second lease expires, so an entry at the head of the queue burns its whole
budget in about fifteen minutes of gateway downtime. Both July outages lasted far
longer than that.

The consequence is not a stalled queue but silent, permanent data loss: a
dead-lettered entry is excluded from the claim query forever, its job never gets
a category, and a job without a category is never indexed. Those 172,875 postings
are in the catalogue, listed by `GET /api/v1/jobs`, and unreachable by search.

The mechanism cannot currently tell "this posting is unenrichable" from "our
gateway was down". Both spend the same three attempts, and the second is far more
common.

## What Changes

- Classify a failure by **whether the posting caused it**. The list is of *our
  own* faults — a corrupted row, a model response that could not be parsed for
  this input, a payload that fails validation — because those are errors we
  raise and control. Everything else, including any transport, gateway, auth or
  timeout error, is ours. An unrecognised error is therefore treated as ours,
  which is the fail-safe direction: being wrong that way costs some retries,
  being wrong the other way loses postings.
- **Bound the two classes differently.** A posting's own fault keeps the attempt
  ceiling (`ENRICH_MAX_ATTEMPTS`, 3). Our faults are bounded by *elapsed time*
  instead: an entry is dead-lettered only once it has been queued longer than
  `ENRICH_UPSTREAM_GRACE` (14 days).
- **Requeue the existing dead-lettered entries** with a one-off SQL statement
  against production, run once after the policy ships. No new command: the
  selection is a `WHERE` clause on one column, and the live policy is what stops
  it recurring.

**Not in scope, but worth recording:** the claim query orders by freshness
descending, so during an outage the entries burning their budget are the newest
postings — the most valuable ones. That ordering is right for normal operation
and wrong under failure, but changing it is a separate concern.

## Capabilities

### New Capabilities

_None._

### Modified Capabilities

- `ai-enrichment`: the requirement "Repeated failures are retried then
  dead-lettered" changes. Dead-lettering on the attempt counter applies only to
  failures the posting caused; every other failure is bounded by how long the
  entry has been queued, so an outage of any duration shorter than the grace
  window costs nothing permanent.

## Impact

- **Code:** `internal/enrich` (the classifier and the two ceilings),
  `internal/config/enrich.go` (the new grace window), `internal/db/queries/enrichment.sql`
  (the failure statement needs the queue age).
- **Schema:** none. `created_at` already exists on `enrichment_outbox`.
- **Operations:** one manual `UPDATE` on production, after the policy deploys —
  ordering matters, since requeueing under the old policy would let a fresh blip
  re-bury the same rows.
- **Expected effect:** roughly 172,875 postings return to the queue and, once
  drained, to the search index.
