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

This change closes that gap for five of the eight — `neogov`, `edjoin`, `workstream`,
`peopleforce`, `gusto` — the ones whose fix is the same mechanical shape: turn an unproven
later-page failure, and reaching the adapter's page-cap constant without ever finding the board's
proven end, into a hard `Fetch` failure; the empty-page proof for all five reads the RAW per-page
item count rather than the count of items newly kept after cross-page dedup, so a page whose items
are all already-seen duplicates cannot masquerade as the board's genuine end. `peopleforce`
additionally needed its per-posting detail fetch to stop silently dropping a posting on a transient
failure, since it has no hydrating fallback and re-fetches every listed posting's detail on every
crawl — see What Changes.

`hh`, `bayt` and `teamtailor` were audited alongside these five and are deliberately excluded.
`hh` cannot structurally earn the marker at all: hh.ru's own search UI caps a query's reachable
depth at ~2000 results independent of a role's true count, confirmed live 2026-09-08 against a
currently-configured board (professional_role 96 — hh.ru's own paging state reports
`totalResults=6657` for the 7-day window while capping its own `lastPage` at index 19, exactly the
ceiling `hhMaxPages` already encodes) — treating that as a hard failure would fail this board's
crawl on every run, not surface a genuine truncation. `bayt` carries no source-declared total and
is already fragile to upstream throttling, so hardening its failure mode needs more care than a
mechanical pass. `teamtailor` has its own change in flight
(`openspec/changes/teamtailor-listing-cap-fix`) that both fixes a confirmed live truncation bug and
raises its page cap based on measured data, which does not belong bundled into this batch.

## What Changes

- `neogov`, `edjoin`, `workstream`, `peopleforce` and `gusto` each stop returning a partial posting
  list when a later listing page fails to fetch or decode — every page failure is now a hard
  `Fetch` error, matching the treatment their FIRST page already had.
- Each of the five now also fails `Fetch` when its walk exhausts the adapter's own page-cap safety
  constant (`neogovMaxPages`, `edjoinMaxPages`, `workstreamMaxPages`, `peopleforceMaxPages`,
  `gustoMaxPages`) without ever reaching a structurally-proven end — a genuinely empty RAW page
  (before cross-page dedup), or, where the source states one, its own declared total/page count.
  Previously reaching the cap silently returned whatever had been gathered.
- The empty-page proof for all five now reads the raw per-page item count, not the count of items
  newly kept after cross-page dedup: a page whose items are all already-seen duplicates (a sort tie
  spanning a page boundary, a re-served page) is not itself proof the board has no more pages
  beyond it. Each adapter gets a regression test for exactly this shape.
- `peopleforce`'s `detail()` now returns the established `unreadableDetail` marker (already used by
  the non-hydrating link-only ATS adapters — jazzhr, icims, careerplug, jobvite, successfactors,
  breezy, bamboohr, smartrecruiters) instead of dropping the posting on a failed-but-not-gone fetch.
  peopleforce has no `HydratingSource` fallback, so every crawl re-fetches every listed posting's
  detail; a plain drop on a transient failure would silently remove an already-known, still-live
  posting from a run now trusted as a full-board listing — exactly what the marker exists to
  prevent. A 404/410 (the platform's own "gone" signal) still drops the posting, unchanged.
- `hh` is audited and explicitly excluded, with the live measurement recorded in `hhru.go`'s own
  comment: it keeps its original soft "reached the cap → return what was gathered" behavior and
  does NOT implement `fullBoardListing`.
- `neogov`, `edjoin`, `workstream`, `peopleforce` and `gusto` now implement the `fullBoardListing`
  marker and are included in `FullBoardListingProviders`, making their boards eligible for the
  post-run sweep's board-scoped close (`CloseUnseenJobsForBoard`).
- No page-cap constant is raised. Unlike `teamtailor-listing-cap-fix`, this batch has no live
  measurement showing any of these five boards is actually being truncated by its existing cap —
  only that the FAILURE MODE around the cap was unproven. Raising a cap without that evidence
  would repeat solidjobs' mistake in the opposite direction.

## Capabilities

### New Capabilities

- `full-board-listing-marker`: the `fullBoardListing` completeness contract itself — what an
  adapter must structurally prove before its boards are safe for the sweep's board-scoped close —
  was, until now, a code-level convention (`source.go`'s interface doc, `AGENTS.md`) with no
  capability spec of its own. This change specs it and, in the same delta, adds the five adapters
  above as providers that meet it. (`openspec/changes/teamtailor-listing-cap-fix`, a separate
  in-flight change, introduces the same capability name for `teamtailor`'s own fix; whichever of
  the two merges first becomes the base spec and the other becomes a `MODIFIED Requirements` delta
  against it.)

### Modified Capabilities

(none)

## Impact

- `internal/ingest/sources/hhru.go` — audited and reverted to its original page-cap behavior, with
  the live measurement that disqualifies it from `fullBoardListing` recorded in comments; no longer
  implements the marker.
- `internal/ingest/sources/neogov.go`, `edjoin.go`, `workstream.go`, `peopleforce.go`, `gusto.go` —
  listing-walk error handling switched to a raw-page-count empty proof, and the new
  `fullBoardListing` marker method. `peopleforce.go` additionally gains an `unreadableDetail`
  marker on its per-posting detail fetch.
- `internal/ingest/sources/hhru_test.go`, `neogov_test.go`, `edjoin_test.go`, `workstream_test.go`,
  `peopleforce_test.go`, `gusto_test.go` — updated/added tests for the hard-fail behavior, a
  page-cap-exhaustion regression test per marked adapter, a marker-(non-)registration test per
  adapter, a duplicate-only-page regression test per marked adapter, and (peopleforce) an
  unreadable-vs-gone detail regression test.
- `internal/ingest/sources/AGENTS.md` — documentation of the five adapters' new hard-fail behavior,
  their addition to the `fullBoardListing` wave, and hh's documented exclusion.
- No migration, no config change, no page-cap constant changed.
