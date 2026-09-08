## Why

`apploi` is the largest unaudited source in the registry by real prod volume — ~1.47M open
postings, larger than `workday`'s own ~1.05M — and had never been checked against the
`fullBoardListing` bar the way the hand-rolled batch (`hh`, `neogov`, `edjoin`, `workstream`,
`peopleforce`, `gusto`), `teamtailor` and `bayt` already were.

Auditing it found the identical defect class those changes already fixed: `Fetch`'s
offset/limit pagination loop breaks to a partial success on a later page's fetch error, and
falls out of the loop silently (an ordinary success, not a failure) if it exhausts
`apploiMaxPages` without ever proving the listing's end. Unlike most of the batch, `apploi` is
otherwise the SIMPLEST of the audited adapters to bring to the bar: its "last page" proof (a
page shorter than the requested page size) is already a valid, unambiguous natural-end signal
for offset/limit pagination — no dedup-vs-raw-count issue applies, since `Fetch` never
deduplicates across pages in the first place (there is nothing to get the emptiness proof
wrong about). And it needs no `unreadableDetail` treatment either: descriptions are inline in
the listing response, so there is no separate per-posting detail fetch to drop postings from.

Before touching the page-cap constant, checked whether any real employer is anywhere close to
it: `apploiMaxPages = 100` at `apploiPageSize = 100` bounds a single employer to 10,000
postings. Prod holds 1,473,738 open `apploi` postings across 5,833 active boards (≈253 per
board on average). A `TABLESAMPLE SYSTEM (10)` sample (a full unindexed scan over source-level
volume this large timed out twice against production and was cancelled both times rather than
left running — see design.md) found the largest single `company_slug` at an estimated ~8,700
postings — and that is a companyWIDE aggregate across however many separate employer/board IDs
that company operates, an upper bound on any ONE board's real count, not a per-board
measurement. No real board is anywhere near the cap.

## What Changes

- `apploi.Fetch`'s pagination loop now proves completeness structurally: a later page failing
  to fetch, and reaching `apploiMaxPages` without ever seeing a page shorter than
  `apploiPageSize`, are both hard `Fetch` failures rather than a partial success.
- `apploi` now implements the `fullBoardListing` marker and is included in
  `FullBoardListingProviders`, making its boards eligible for the post-run sweep's board-scoped
  close (`CloseUnseenJobsForBoard`).
- No page-cap constant is raised. The company-level sample found nothing close to the cap;
  raising it without board-level evidence would repeat solidjobs' mistake in the opposite
  direction.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `full-board-listing-marker`: adds `apploi` to the set of adapters that meet the completeness
  bar.

## Impact

- `internal/ingest/sources/apploi.go` — listing-loop error handling and the new
  `fullBoardListing` marker method.
- `internal/ingest/sources/apploi_test.go` — later-page-failure, page-cap-exhaustion, and
  marker-registration regression tests.
- `internal/ingest/sources/AGENTS.md` — `apploi` added to the marked-provider list.
- No migration, no config change, no page-cap constant changed.
