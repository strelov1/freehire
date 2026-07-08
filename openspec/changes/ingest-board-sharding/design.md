## Context

`cmd/ingest` crawls one board file per run under a systemd `TimeoutStartSec`. `workday.yml` (~6165 boards) can't finish in the 40-min window because Workday 429-throttles and each throttled board retries (honoring Retry-After), stalling the bounded pool. The run always dies at the same ~1700-board prefix, so most boards never crawl.

## Goals / Non-Goals

**Goals:**
- Let one oversized board file be crawled across several staggered, timeout-safe runs that together cover every board.
- Keep it a generic ingest capability (any file), not a workday special case.
- No change to the pipeline, the write path, or the stale-job sweep.

**Non-Goals:**
- Changing 429 retry behavior (a separate lever; sharding alone restores coverage).
- Per-file schedule config in the repo — cadence lives in the ops timers.

## Decisions

- **Round-robin by company, not by board index.** `Config.Shard(i, n)` assigns each distinct company (keyed by its normalized company slug, in first-appearance order) round-robin to a shard and keeps all of that company's boards together. This is a correctness requirement, not just balancing: the stale-job sweep scopes closes by `company_slug`, and `workday.yml` has ~800 companies owning multiple boards. Splitting a company across shards would let shard A (having crawled one of the company's boards) sweep-close the company's still-live boards that shard B owns but hasn't refreshed within the grace window — exactly the over-close the sweep exists to avoid. Grouping by company also spreads same-tenant boards across shards, so no shard inherits a dense 429 cluster.
- **Selector via `--shard=i/n` flag or `SHARD` env.** The flag suits explicit per-timer `ExecStart` lines; the env suits a drop-in. The positional board-file arg is unchanged, so existing timers keep working with no selector.
- **Validate full, crawl slice.** The whole file is validated every run (a bad entry fails every shard, not just the one that happens to include it); the slice is applied only to what gets crawled.
- **Sweep stays shard-safe.** With company-grouped shards, each shard crawls a company in full and its post-run sweep (`crawled.slugs`) closes only companies it wholly owns — the existing "partial run closes only what it saw" invariant, now at company (not board) granularity. Shards every 6h against a 48h grace keep every company re-seen well before its cutoff even if a shard misses a cycle.

## Risks / Trade-offs

- **Slower per-board refresh.** A board now refreshes once per shard cycle (6h) instead of hourly — an accepted trade for actually crawling all boards vs. hourly-crawling a fixed 28% and never the rest.
- **Ops must keep shard count and timers in sync.** If `n` in the timers doesn't match the number of staggered timers, some boards go uncrawled. Mitigated by generating the shard timers together.
