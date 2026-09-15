## Context

Confirmed live against `boldbusiness.scalis.ai`: the listing page
(`/jobs?page=N&limit=10&sortBy=SORT_BEST_MATCH`) inlines an `"initialData":{"results":[...],
"count":46,"paginationCount":46,...}` object in its RSC flight, where each result already
carries the SAME rich job shape the single-posting detail page carries (title, company,
locations, employment/workplace enums, skills, salary, and `description`/`descriptionHtml`
as `"$<id>"` text-row references) — confirmed across page 1 (10 results), page 5 (6 of 46,
the true last page) and page 6 (empty results, no redirect trap). This means the listing
alone is a `fullBoardListing` source with no per-posting detail fetch needed at all, unlike
`topco` (whose flight only carries a lazy `"$undefined"` body).

## Goals / Non-Goals

**Goals:**
- Crawl a Scalis tenant's open postings to exhaustion from the listing alone, using the
  existing `nextflight.go` primitives exactly as `deel`/`topco`/`micro1` already do.

**Non-Goals:**
- No tenant-discovery/harvest prober — only one live tenant (`boldbusiness`) is known
  today, the same reasoning `selfrecruit-source` and `hrmos-source` already gave.
- No `salary_period` mapping beyond the direct `payment` enum reading (`SALARY`→`year`,
  `HOURLY`→`hour`) — those are the only two values seen in the tenant's own filter facet
  counts, and neither salary bound was populated on any sampled posting, so the period
  mapping is untested against a real non-null example; ship the safe, obviously-correct
  reading rather than guessing further.

## Decisions

- **Page until an empty result list, not until `page*limit >= count`.** The confirmed
  past-the-end behavior (page 6: empty `results`, `count` unchanged) is a strictly simpler
  termination signal than tracking a running total against a count field that could itself
  be stale mid-walk; it also mirrors the empty-page-ends-the-walk convention already used
  by every HTML-listing paginator in this package (`crawlPagedLinks`).
- **A page-fetch/decode failure, OR exhausting `scalisMaxPages` without an empty page,
  fails the whole `Fetch`.** Manual loop (not `crawlAllPagedLinks`, which is HTML-link-
  oriented) that returns the error immediately on any page failure — same "whole listing or
  fail outright" contract as the HTML paginators — and, after the loop, an error naming the
  ceiling if it ran out without ever seeing an empty page. The first review pass on this
  change shipped without that second check (a silent partial success on ceiling exhaustion),
  which is the exact bug `teamtailor.ttMaxPages` already caused and fixed once on a real
  board (see that adapter's own comment) — caught here before merge instead of by a second
  live incident.
- **Resolve the `"$<id>"` description reference the same way `deel`/`micro1` do, and share
  one resolver between the per-posting mapper and the board-wide health check.**
  `scalisDescription` is the single place that reads `descriptionHtml` (preferred, for its
  structure) falling back to the plain-text `description` when the HTML reference doesn't
  resolve, and reports whether either field was a reference at all — both `scalisToJob` and
  `Fetch`'s "did any reference resolve" gate call it, so they cannot disagree about whether
  a given posting's description resolved (an earlier draft had two separate resolution
  paths that disagreed on exactly this point, caught by a fallback test written directly
  against the board-wide check).
- **The board-wide "no reference resolved" gate mirrors `deel.Fetch`'s.** If every
  description reference across the whole board failed, the row-parse itself likely broke
  (e.g. a marker-format change) — fail loudly rather than ship a board of empty-bodied
  postings. A single unresolved reference on an otherwise-healthy board still degrades to
  an empty description for just that posting.

## Risks / Trade-offs

- [Only one tenant confirmed] → the job object's field set could vary slightly on a
  differently-configured tenant. Mitigation: every field is read defensively (a missing or
  differently-shaped field decodes to its zero value via `encoding/json`, never an error),
  matching this codebase's general posture for a single-tenant-confirmed adapter.

## Migration Plan

No data migration. Ship the adapter, merge, deploy, then add `scalis/boldbusiness` by hand
via `cmd/add-board` to close `board_submissions` id 29.
