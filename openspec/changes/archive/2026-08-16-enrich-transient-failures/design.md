## Context

`enrichment_outbox` dead-letters an entry by setting `failed_at` once `attempts`
reaches `ENRICH_MAX_ATTEMPTS` (3). `ClaimEnrichmentBatch` excludes
`failed_at IS NOT NULL`, so dead-lettering is permanent — there is no path back.

The runner already accepts that the error class should decide the ceiling:
`process` passes `maxAttempts=1` for a corrupted row, forcing an immediate
dead-letter because an unreadable row will never load. What is missing is the
symmetric case — a failure that will certainly succeed later.

Production, measured 2026-08-16:

| Dead-lettered entries | Count |
| --- | ---: |
| `enrich: llm: …` (gateway 502/500/401, timeouts) | 172,875 |
| `enrich: unparseable model response: …` | 4 |
| **Total** | **172,879** |

All of the first group date from LiteLLM outages on 17 and 24 July. Those
postings are in the catalogue and unreachable by search, because
`search.CategoryUnresolved` excludes a job with no category and enrichment is
what assigns one.

## Goals / Non-Goals

**Goals:**

- An outage of any plausible duration costs no posting permanently.
- A posting that genuinely cannot be enriched still stops consuming budget.
- The existing 172,875 entries return to the queue.
- The classification is defined once, in terms this codebase controls.

**Non-Goals:**

- Retry backoff, jitter, or a circuit breaker on the LLM gateway. The lease
  already spaces retries; adding a breaker is a different change with a different
  justification.
- Changing the claim ordering. It sorts by freshness descending, so an outage
  burns the budget of the newest postings first — right for normal operation,
  wrong under failure. Recorded, not fixed here.
- A command for the requeue. It runs once.

## Decisions

### Enumerate our own faults, not the upstream's

The classifier lists the failures **the posting caused**: a corrupted row
(`pgerr.IsDataCorrupted`), an unparseable model response (`errUnparseableResponse`),
and a payload that fails `Validate`. Everything else is ours.

The alternative — matching the upstream's error text for 5xx, timeouts and auth —
was rejected on two counts. The text belongs to langchaingo, so it is not ours to
depend on; and the default would be wrong. An unmatched error would be blamed on
the posting, which is precisely the failure being fixed here.

Enumerating our own faults inverts the default. An error class nobody anticipated
is treated as ours and retried. Being wrong in that direction costs some LLM
calls; being wrong in the other direction is what buried 172,875 postings.

### Bound our faults by time, not attempts

An attempt counter does not measure outage duration. A claimed entry becomes
re-claimable when its 300-second lease expires, so an entry at the head of the
queue accrues roughly twelve attempts an hour while the gateway is down. Three
attempts is fifteen minutes. Even a ceiling of 50 — the shape first considered —
is four hours, and both July outages ran far longer. Surviving two days by
attempts alone needs a ceiling near 600, which is not a bound anyone can reason
about.

Elapsed queue time measures the thing the bound is actually for.
`ENRICH_UPSTREAM_GRACE` (14 days) gives a two-day outage a sevenfold margin, and
still stops an entry the gateway will never accept.

`enrichment_outbox.created_at` already exists, so the age is available with no
schema change.

### The two ceilings stay separate

`ENRICH_MAX_ATTEMPTS` keeps its meaning and value: it bounds a posting that
cannot be enriched. `ENRICH_UPSTREAM_GRACE` bounds everything else. Neither
subsumes the other — one counts tries at a task that may be impossible, the other
waits out a dependency that is expected back.

### Requeue with SQL, not a command

The selection is a `WHERE` clause on one column against a two-class distribution,
and it runs once. A command would mean a binary, a unit file, a release-script
entry and tests, all for a single execution. `seed-adzuna-description-queue` is
the nearest precedent and does not apply: that one had to *compute* eligibility
per job.

The cost is that "the posting's fault" is briefly expressed twice — once in Go,
once as `last_error NOT LIKE 'enrich: unparseable model response%'`. That
duplication lasts one statement, because the live policy is what prevents a
recurrence.

Order matters: the policy deploys first. Requeueing under the current policy
would let the next brief blip re-bury the same rows.

## Risks / Trade-offs

- **A permanently unservable entry now occupies the queue for 14 days instead of
  15 minutes** → It is claimed, fails fast, and releases. The queue already holds
  ~880k drainable entries; a few thousand retried occasionally is not the
  constraint. The bound still exists.
- **An error class we should blame on the posting gets retried for 14 days** →
  The deliberate direction of the default. If a class shows up in volume, it goes
  in the classifier, and the classifier is a list with a test per entry.
- **The requeue adds ~173k entries to an already large queue** → It is additive to
  a backlog the worker is already draining at roughly 23k/day against ~7k/day
  inflow. It extends the drain by about a week, and every one of those entries is
  a posting currently invisible to search.
- **`last_error` is matched as text in the one-off statement** → Verified against
  the live distribution first: two classes, one exclusion. The statement is
  reviewed against that measurement, not written from memory.

## Migration Plan

1. Ship the policy. It changes only which failures dead-letter; nothing existing
   is rewritten.
2. Confirm the error-class distribution still holds on production.
3. Run the requeue statement:

   ```sql
   UPDATE enrichment_outbox
   SET failed_at = NULL, attempts = 0
   WHERE failed_at IS NOT NULL
     AND last_error NOT LIKE 'enrich: unparseable model response%';
   ```

4. Watch the enrich worker's progress log and the indexed document count.

Rollback: reverting the policy restores attempt-only dead-lettering. Entries
requeued in step 3 stay requeued, which is harmless — they simply follow the old
rules again.

## Open Questions

None.
