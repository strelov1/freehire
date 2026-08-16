## Why

The SEEK adapter's first production run exposed two faults the local verification could not: it
ingested 2,982 postings and **2,593 of them (87%) carry no description**.

SEEK's GraphQL detail endpoint is behind a **burst rate limiter**. The unpaced hydration fired 3,267
POSTs in 95 seconds from the prod egress IP and was answered **429** on nearly all of them; the same
request returned 200 after two minutes idle, so this is a rate window, not a block. The local
verification issued 31 requests and never approached it.

The second fault is what turned a transient refusal into permanent damage. A posting whose detail
fails is ingested body-less, and the next crawl reports it as *seen* — so it is never hydrated again.
`internal/sources/pacer.go` already names this failure mode in the emagine comment ("the loss is
permanent"). Its severity was misjudged when the adapter shipped: it was recorded as a rare-event
limitation on the strength of 31/31 local successes, and under a rate limiter it is instead the
dominant case.

## What Changes

- The SEEK GraphQL detail fetch is **paced** through a shared rate limiter, so the aggregate request
  rate of a whole run stays under SEEK's window regardless of the detail pool's concurrency — the
  same lever `careerspage`, `clinch` and `vagas` already use, extended to a JSON POST transport,
  which the pacer did not previously cover.
- **A posting whose description could not be fetched is no longer ingested.** It is dropped for that
  run, so the next crawl still sees it as new and retries it. This reverses the rule the adapter
  shipped with (and that `hh` and the other hydrating adapters keep), deliberately and only for
  SEEK: that rule assumes detail failures are rare, and against a rate-limited endpoint they are not.
- The prod rows already ingested body-less are pruned so the paced crawl re-ingests them complete.

## Capabilities

### New Capabilities
<!-- None: this changes how an existing capability behaves. -->

### Modified Capabilities
- `seek-source`: the detail-hydration requirement changes on two points — the fetch is rate-paced,
  and a posting whose detail fails is dropped rather than ingested body-less.

## Impact

- **Modified:** `internal/sources/seek.go`, `internal/sources/seek_test.go`,
  `internal/sources/pacer.go` (a paced JSON-POST wrapper), `internal/sources/registry.go`.
- **Ops:** a one-off `cmd/prune` of the `seek` rows, then a fresh crawl. The backfill (~7.8k
  postings) no longer fits one ingest window at the paced rate and accretes across a few runs —
  which the drop-and-retry rule now makes safe.
- **Risk:** the chosen rate is a conservative guess, as every other pacer constant in this file is.
  Under-shooting only lengthens the backfill; over-shooting re-enters the 429 window.
