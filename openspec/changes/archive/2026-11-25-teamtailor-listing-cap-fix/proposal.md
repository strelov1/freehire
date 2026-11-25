## Why

`internal/ingest/sources/AGENTS.md` names the `fullBoardListing` marker's registry as an ongoing audit: the first wave of ~19-21 large ATS platforms is marked; "the rest of the registry is unmarked, not because it fails the bar, but because nobody has audited it yet." A past incident (#2337) is exactly why the bar is strict: an unverified pagination cap on `solidjobs` silently truncated a board and closed 110 live postings before the revert.

Auditing the 8 adapters `AGENTS.md` already calls out as structurally distinct (`hh`, `teamtailor`, `neogov`, `edjoin`, `workstream`, `bayt`, `peopleforce`, `gusto` — each "restates the walk by hand" instead of calling the shared `crawlPagedLinks`/`crawlAllPagedLinks` helper) found all 8 currently fail the bar, most sharing one defect: a later-page fetch error `break`s and returns the partial result as a plain success instead of failing the whole `Fetch`.

Auditing `teamtailor` specifically surfaced something beyond a marker-eligibility question: a confirmed, live, active data-loss bug. `teamtailor.go`'s own `ttMaxPages=100` cap was silently truncating real boards. Live-probed against the two largest real `teamtailor.com` boards in prod (by open-job count): `migen.teamtailor.com` (2434 open postings) reaches its natural end well under page 100, unaffected — but `tantor.teamtailor.com` (2000 open postings, a suspiciously round number) still returns a full page of fresh links at pages 100 AND 101, with its real end only found between page 120 (still full) and page 130 (empty). Its real catalogue is roughly 2400-2580 postings; the adapter was silently capping it at ~2000 — a confirmed ~20-25% under-collection on at least this one live board, not a hypothetical.

## What Changes

- `internal/ingest/sources`' `teamtailor` adapter: `ttMaxPages` raised from 100 to 1000 (a wide multiple over the confirmed real maximum of ~125-129 pages), and its listing walk (`jobURLs`, shared by `Fetch` and `FetchNew`) now treats both a later-page fetch failure and exhausting the page ceiling without finding a natural end as a hard `Fetch` failure — never a partial success. `teamtailor` earns the `fullBoardListing` marker on the strength of this fix, joining the existing wave.
- New capability describing the `fullBoardListing` marker's own contract as an observable behavior (previously only documented in `AGENTS.md`/code comments, never captured in `openspec/specs`) — what a marked provider must structurally prove, and what depends on the marker (the per-run board-scoped unseen-job close, `CloseUnseenJobsForBoard`, freehire#2328).

**Explicitly out of scope:** the other 7 adapters audited alongside `teamtailor` (`hh`, `neogov`, `edjoin`, `workstream`, `bayt`, `peopleforce`, `gusto`) are NOT fixed by this change — they were read to know which share `teamtailor`'s exact defect shape (most do) and which don't (`bayt` has no source-declared total to verify against at all, a harder problem), but the actual fixes are left for a later pass. This change is a single, urgent fix picked out of a larger, still-open audit — not the audit's completion. Also out of scope: the ~163 remaining unmarked, unaudited adapters in the registry, among which `apploi` (~1.47M open postings — larger than `workday`'s own ~1.05M) is the single largest by real prod volume and has not been read at all yet.

## Capabilities

### New Capabilities
- `full-board-listing-marker`: the observable contract behind `sources.fullBoardListing` — what a marked provider's crawl must prove about a board's completeness, and that only a marked provider's boards are eligible for the per-run board-scoped unseen-job close.

### Modified Capabilities
(none)

## Impact

- `internal/ingest/sources/teamtailor.go`: `ttMaxPages` raised; `jobURLs` fails hard on a later-page error or an unproven end; `fullBoardListing()` marker added.
- `internal/ingest/sources/teamtailor_test.go`: three new tests (a later-page failure, exhausting the raised cap, the marker's own registry assertion).
- `internal/ingest/sources/AGENTS.md`: the `fullBoardListing` paragraph's provider list and audit-status note updated to reflect `teamtailor`'s addition and the state of the broader, still-open audit.
- No migration, no config change. The next `cmd/ingest teamtailor` run is the only thing that needs to happen for the fix to reach prod — no backfill, since a previously-truncated board's missing postings simply appear on its next successful crawl.
