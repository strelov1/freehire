## Why

`internal/ingest/sources/AGENTS.md` documents eight adapters that restate the paginated-listing
walk by hand instead of calling the shared `crawlPagedLinks`/`crawlAllPagedLinks` helpers, because
each differs in transport, dedup key or extra stop condition. All eight predate the
`fullBoardListing` marker (`source.go`) and, until now, none of them earned it: a later listing
page failing silently ended the walk with whatever was gathered so far, reported as an ordinary
success, and none of them treated exhausting their own page-cap safety ceiling as a failure
either. That is exactly the shape of #2337 — an unverified, undocumented pagination truncation on
`solidjobs` silently closed 110 live postings before the revert — and it is why `fullBoardListing`
gates `CloseUnseenJobsForBoard`: only an adapter that structurally proves it reached a board's real
end is safe for the board-scoped sweep to trust an "unseen" verdict from.

This change closes that gap for six of the eight — `hh`, `neogov`, `edjoin`, `workstream`,
`peopleforce`, `gusto` — the ones whose fix is the same mechanical shape: turn an unproven
later-page failure, and reaching the adapter's page-cap constant without ever finding the board's
proven end, into a hard `Fetch` failure. `bayt` and `teamtailor` are deliberately excluded: `bayt`
carries no source-declared total and is already fragile to upstream throttling, so hardening its
failure mode needs more care than a mechanical pass; `teamtailor` has its own change in flight
(`openspec/changes/teamtailor-listing-cap-fix`) that both fixes a confirmed live truncation bug and
raises its page cap based on measured data, which does not belong bundled into this batch.

## What Changes

- `hh`, `neogov`, `edjoin`, `workstream`, `peopleforce` and `gusto` each stop returning a partial
  posting list when a later listing page fails to fetch or decode — every page failure is now a
  hard `Fetch` error, matching the treatment their FIRST page already had.
- Each of the six now also fails `Fetch` when its walk exhausts the adapter's own page-cap safety
  constant (`hhMaxPages`, `neogovMaxPages`, `edjoinMaxPages`, `workstreamMaxPages`,
  `peopleforceMaxPages`, `gustoMaxPages`) without ever reaching a structurally-proven end — a
  genuinely empty page, or, where the source states one, its own declared total/page count.
  Previously reaching the cap silently returned whatever had been gathered.
- All six now implement the `fullBoardListing` marker and are included in
  `FullBoardListingProviders`, making their boards eligible for the post-run sweep's board-scoped
  close (`CloseUnseenJobsForBoard`).
- No page-cap constant is raised. Unlike `teamtailor-listing-cap-fix`, this batch has no live
  measurement showing any of these six boards is actually being truncated by its existing cap —
  only that the FAILURE MODE around the cap was unproven. Raising a cap without that evidence
  would repeat solidjobs' mistake in the opposite direction.

## Capabilities

### New Capabilities

- `full-board-listing-marker`: the `fullBoardListing` completeness contract itself — what an
  adapter must structurally prove before its boards are safe for the sweep's board-scoped close —
  was, until now, a code-level convention (`source.go`'s interface doc, `AGENTS.md`) with no
  capability spec of its own. This change specs it and, in the same delta, adds the six adapters
  above as providers that meet it. (`openspec/changes/teamtailor-listing-cap-fix`, a separate
  in-flight change, introduces the same capability name for `teamtailor`'s own fix; whichever of
  the two merges first becomes the base spec and the other becomes a `MODIFIED Requirements` delta
  against it.)

### Modified Capabilities

(none)

## Impact

- `internal/ingest/sources/hhru.go`, `neogov.go`, `edjoin.go`, `workstream.go`, `peopleforce.go`,
  `gusto.go` — listing-walk error handling and the new `fullBoardListing` marker method.
- `internal/ingest/sources/hhru_test.go`, `neogov_test.go`, `edjoin_test.go`, `workstream_test.go`,
  `peopleforce_test.go`, `gusto_test.go` — updated/added tests for the hard-fail behavior, a
  page-cap-exhaustion regression test per adapter, and a marker-registration test per adapter.
- `internal/ingest/sources/AGENTS.md` — documentation of the six adapters' new hard-fail behavior
  and their addition to the `fullBoardListing` wave.
- No migration, no config change, no page-cap constant changed.
