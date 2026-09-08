## Why

`internal/ingest/sources/AGENTS.md` documents eight adapters that restate the paginated-listing
walk by hand instead of calling the shared `crawlPagedLinks`/`crawlAllPagedLinks` helpers. Six of
them (`hh`, `neogov`, `edjoin`, `workstream`, `peopleforce`, `gusto`) were already brought up to the
`fullBoardListing` bar in `fullboardlisting-hand-rolled-batch`, and `teamtailor` in its own change
— both merged. `bayt` was deliberately excluded from that batch at the time: it carries no
source-declared total to verify a fetched count against (only a genuinely empty page can prove
completeness), and its own code comments document a real, live-observed throttling risk (Bayt's
Akamai edge 403s a fast concurrent burst), which made "does hard-failing on a later-page error turn
routine throttling into constant crawl failures" a real open question rather than a mechanical
copy-paste of the batch fix.

Investigating that question found `bayt`'s listing walk is single-threaded (one page at a time,
sequential) — structurally different from the concurrent 3-way detail fan-out
(`baytDetailWorkers`) the throttling comment was written about, which needed narrowing specifically
because concurrency triggers Akamai's burst detection. There is no evidence the sequential listing
walk carries the same risk, and the same trade-off already accepted for the six batch-fixed
adapters — a transient later-page failure now fails the crawl for that run, absorbed by
`board_health`'s existing cooldown/backoff — applies here too. Separately, auditing `bayt`'s
`detail()` found it has no `HydratingSource` fallback (like `peopleforce` before its own fix): every
crawl re-fetches every listed posting's detail from scratch and dropped it outright on ANY fetch
failure, no distinction between "gone" (404/410) and "merely unreadable" — exactly the risk a
throttled 403 on the concurrent detail fan-out creates, and exactly the gap the established
`unreadableDetail` pattern exists to close.

## What Changes

- `bayt.Fetch`'s listing walk now proves completeness the same way the six batch-fixed adapters
  do: the empty-page proof reads the raw per-page job-link count (before cross-page dedup, and
  filtered to links `baytJobID` recognizes as job-detail links — the page's own navigation chrome
  is never empty and cannot serve as the proof). A later page failing to fetch, and reaching
  `baytMaxPages` without ever finding a genuinely empty page, are now hard `Fetch` failures rather
  than a partial success.
- `bayt.detail` now returns the established `unreadableDetail` marker (matching `careerplug.go`'s
  pattern) on a failed-but-not-404/410 fetch, a 200 with no ld+json `JobPosting` at all, or a
  `JobPosting` with no resolvable employer — instead of dropping the posting in any of those
  cases. Closes the gap a throttled detail request, or a site-wide markup change that broke
  parsing, would otherwise open now that `bayt` is trusted for board-scoped close.
- `bayt` now implements the `fullBoardListing` marker and is included in
  `FullBoardListingProviders`, making its boards eligible for the post-run sweep's board-scoped
  close (`CloseUnseenJobsForBoard`).
- No page-cap constant is raised. No live measurement shows `baytMaxPages` (50) being reached by a
  real country's listing — the fix is to the failure mode around the cap, not the cap's size.

## Capabilities

### New Capabilities

(none — `full-board-listing-marker` was already proposed as a new capability by
`fullboardlisting-hand-rolled-batch` and `teamtailor-listing-cap-fix`, both merged but not yet
archived into `openspec/specs/`. This change extends the same capability with a `MODIFIED
Requirements` delta rather than proposing it a third time; whichever of the two prior changes
archives first becomes the base spec this delta applies against.)

### Modified Capabilities

- `full-board-listing-marker`: adds `bayt` to the set of adapters that meet the completeness bar,
  and extends `bayt.detail`'s failure handling to distinguish an unreadable fetch from a gone
  posting.

## Impact

- `internal/ingest/sources/bayt.go` — listing-walk error handling and raw-count empty proof,
  `detail`'s `unreadableDetail` marker, the new `fullBoardListing` marker method.
- `internal/ingest/sources/bayt_test.go` — later-page-failure, page-cap-exhaustion,
  duplicate-only-page, unreadable-vs-gone-detail, and marker-registration regression tests.
- `internal/ingest/sources/AGENTS.md` — `bayt` moves from "audited and excluded" to marked, with
  the reasoning that unblocked it.
- No migration, no config change, no page-cap constant changed.
