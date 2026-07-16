## Why

careers-page.com rate-limits by a per-IP request budget per time window (HTTP 429).
The shipped proxy egress (#791) and reduced detail concurrency (#792) stopped the
board-level hard-fails and un-froze the provider, but they cap the instantaneous
burst, not the total requests per window. In a full careerspage run all boards
crawl sequentially over one shared proxy IP, so the first/large board spends the
window budget and later boards starve — Alcor plateaus at ~17 of its ~34 valid
postings and never fully converges. We need the aggregate request rate held under
the window so one run collects every posting.

## What Changes

- Add a reusable rate-limited `HTMLGetter` decorator (token-bucket over
  `golang.org/x/time/rate`) that blocks before each `GetHTML` so a wrapped getter's
  aggregate request rate stays under a configured limit, independent of the caller's
  worker concurrency. (careerspage's listing + detail both go through `GetHTML`;
  widening to other request kinds is a trivial later addition.)
- Wire that decorator around the `careerspage` adapter only, so a single limiter is
  shared across all of careerspage's requests in a run (listing + detail, every
  board). The decorator is provider-agnostic; only careerspage opts in for now.
- Keep `careerspageDetailWorkers = 2` — the pacer, not the pool size, now governs
  pace; workers stay a secondary latency knob.

## Capabilities

### New Capabilities

<!-- none: the pacing behavior is a requirement of the existing careerspage-source capability; the decorator is an implementation detail (design.md) -->

### Modified Capabilities

- `careerspage-source`: the crawl SHALL hold its aggregate request rate under the
  host's per-IP rate-limit window so a single run collects every posting across all
  configured boards, instead of later boards starving once an earlier board spends
  the window budget.

## Impact

- `internal/sources/` — new rate-limited client decorator + one-line registry wiring
  for careerspage; no change to the adapter's parsing.
- New dependency: `golang.org/x/time/rate` (small, std-adjacent).
- Ops: a paced full run is longer (bounded request rate × posting count); confirm the
  `freehire-ingest@careerspage` unit's `TimeoutStartSec` has headroom.
