## Why

The `reed` aggregator re-fetches per-posting detail for **every** live posting on
every crawl (~1700 detail requests plus keyword-search pages), and its ingest timer
runs hourly. The Reed Jobseeker API enforces a **per-hour request quota** and signals
exhaustion with HTTP 403 (`"You have exceeded your per-hour request limit"`). When the
quota runs out on the first keyword search — before any posting is emitted — the
streaming crawl records a board-level `"streaming board failed with no progress"`
failure, intermittently backing the board off in `board_health`. The catalogue already
holds these postings; re-hydrating unchanged ones each hour is the wasteful multiplier
that blows the quota.

## What Changes

- Convert the `reed` adapter from a `StreamingSource` to a `HydratingSource`: it
  fetches per-posting detail **only for postings the catalogue does not already have**
  (mirroring the existing `justjoin` adapter), and marks already-ingested postings for a
  liveness refresh instead of re-fetching their detail.
- **BREAKING (internal contract only):** `reed` no longer implements `FetchStream`
  (drops incremental streaming save). Its `Fetch` remains as the list-only fallback used
  when the pipeline cannot supply a seen-set (tests / non-DB callers), hydrating every
  posting.
- Pairs with an already-applied ops-side cadence change (not part of this repo): the
  `reed` ingest timer moved from hourly to every 6h in `freehire-ops`
  (`gen-ingest-timers.sh`).

Net effect: steady-state detail requests drop from ~1700/run to just the run's new
postings (dozens), keeping `reed` under the per-hour quota and eliminating the
"no progress" failures.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `source-ingest`: The "Reed is a registered keyed, keyword-scoped aggregator provider"
  requirement changes from "fetch each unique job's detail" (all postings) to hydrating
  detail only for postings not already ingested, refreshing liveness for the rest — reed
  now conforms to the existing "Adapters may hydrate only postings the catalogue lacks"
  capability.

## Impact

- `internal/sources/reed.go` — remove `FetchStream`; add `FetchNew`; reshape `Fetch` to
  a list-only fallback.
- `internal/sources/reed_test.go` — add a seen-set test mirroring `justjoin_test.go`
  (seen id → liveness-refresh, no detail request; new id → hydrated).
- No pipeline changes: the `HydratingSource` seam, seen-set lookup, and liveness `touch`
  path already exist and are exercised by `justjoin`.
- No new dependencies, no DB migration, no config changes.
- Behavioral trade-off: loss of streaming incremental-save for reed; acceptable because
  the hydrating crawl only fetches detail for new postings and is small.
