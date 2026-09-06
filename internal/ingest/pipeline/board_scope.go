package pipeline

import "github.com/strelov1/freehire/internal/ingest/sources"

// boardReachedPostings reports whether a board's crawl reached at least one posting,
// whether or not it was ultimately saved: Ingested, Rejected, and ATSCovered all mean the
// crawl listed the posting and something downstream then decided what to do with it.
// Skipped is deliberately excluded — it means the posting was listed and then FAILED TO
// PERSIST, so counting it would let a board whose every save is failing prove itself on the
// strength of its own persistence failures.
func boardReachedPostings(st Stats) bool {
	return st.Ingested+st.Rejected+st.ATSCovered > 0
}

// maxUnreadablePercent is the share of a board's listed postings that may be unreadable before
// the crawl stops counting as evidence of what is on that board. A share rather than a count,
// and neither zero nor generous, for reasons that pull in opposite directions:
//
//   - Zero tolerance would switch the mechanism off for exactly the boards it matters most
//     for. Per-posting failures are independent, so on a 5,000-posting board even a 0.1%
//     per-request failure rate leaves under a 1% chance that any given run reads every one of
//     them — the close would essentially never fire on a large board, while firing normally on
//     a small one. A share scales with the board and keeps the rule the same rule at both ends.
//   - The tolerance is nonetheless small because the failures that actually matter are
//     WHOLESALE, not scattered: a refusing origin, a retired tenant, a rate-limit burst, a
//     platform changing its markup — every measured incident in this area (seek's first prod
//     crawl at 87%, solidjobs' truncation, freehire#725) failed most or all of a board. 5% sits
//     an order of magnitude above the texture of the web and an order of magnitude below any of
//     those.
//
// On a small board it is effectively zero tolerance (one unreadable posting in fifteen already
// exceeds it), which is the right reading there: a zero-failure run is the norm on a small
// board, so refusing this one costs a crawl cycle of latency on a genuine removal. That is the
// direction to err in — a wrongly closed posting reopens on the next successful crawl, but only
// after it has left the search index and written a phantom removal into job_daily_stats.
//
// WHAT A BOARD-LEVEL SHARE DOES NOT COVER: the independence argument above holds per posting on
// one board, and a multi-tenant board is several employers under one crawl. A successfactors hub
// tenant with 20 postings going entirely dark on a 1,000-posting hub is 2% — under the threshold
// — so the hub still qualifies and that tenant's postings are still phantom-closed. The
// company-scoped half catches it only when the unreadable entries carry the right company, which
// they do whenever the adapter reads the employer from the ENTRY rather than the page. Narrowing
// this to a per-tenant share would need the tenant to be a dimension of Stats, which it is not
// today; the note is here so the next reader does not mistake the threshold for a guarantee it
// does not make.
const maxUnreadablePercent = 5

// boardReadWhatItListed reports whether the board's crawl actually READ the postings its
// listing named, within maxUnreadablePercent. It is the third qualification condition and the
// one the other two cannot see: a board whose listing succeeded and whose readable postings all
// saved has Failed == 0 and a healthy yield, while the postings whose detail requests died are
// simply absent from everything the run recorded — and absence is exactly what the sweep reads
// as removal.
//
// The denominator is every posting the board yielded, which the counters partition exactly:
// Ingested, Rejected, ATSCovered and Skipped between them account for every posting the crawl
// read, and Unreadable for every one it did not. Postings the platform answered 404/410 for are
// deliberately NOT in it — the adapter never yields those, because a posting the platform says
// is gone is evidence rather than a hole, and a board whose listing has gone entirely stale
// should still be allowed to retire it.
func boardReadWhatItListed(st Stats) bool {
	listed := st.Ingested + st.Rejected + st.ATSCovered + st.Skipped + st.Unreadable
	return st.Unreadable*100 <= listed*maxUnreadablePercent
}

// boardQualifies reports whether a run structurally PROVED it covered a board — the fact the
// post-run sweep's board-scoped close needs before it may retire anything within that board
// (freehire#2328, docs/agents/job-lifecycle.md). Three conditions, all read directly off this
// board's own Stats rather than through BoardHealth's tolerant health verdict:
//
//  1. the board's crawl reported zero failures (st.Failed == 0). A streaming board that dies
//     mid-crawl after partial progress is deliberately recorded as HEALTHY by BoardHealth (a
//     rate-limited stream must not cool a working board down), but the Stats ingestStream
//     returns for it still carries Failed > 0 — and a board caught mid-crawl is exactly the
//     case this scope must refuse, or the sweep would close every posting past the point the
//     crawl died (the freehire#725 class of bug this whole mechanism exists to avoid). Reading
//     BoardHealth's verdict here instead of Stats.Failed would reintroduce that exposure.
//  2. the crawl reached at least one posting (boardReachedPostings). A board that returned
//     nothing is indistinguishable from a board whose crawl silently broke.
//  3. the crawl READ the postings it listed (boardReadWhatItListed). On the ATS platforms
//     whose detail request is a posting's only source, a dropped detail used to leave the
//     posting missing from a run that reported no failure at all — so the sweep closed a live
//     posting as unseen. Failed cannot answer this and must not: a dropped detail is a
//     per-posting hole, and failing the board over one would cool it to its 24-hour ceiling
//     and freeze the whole catalogue behind it.
//
// A boardless entry (e.Board == "") never qualifies: its postings are namespaced with an
// empty board (externalid.Namespace("", id)), so a board-scoped LIKE pattern built from it
// would match the provider's WHOLE catalogue rather than nothing.
func boardQualifies(e sources.CompanyEntry, st Stats) bool {
	return e.Board != "" && st.Failed == 0 && boardReachedPostings(st) && boardReadWhatItListed(st)
}

// providerBoard identifies a board by name alone, ignoring region — the key the board-scoped
// close's SQL predicate is forced to use, since externalid.Namespace does not encode region.
// Deliberately NOT the existing boardKey{provider, board, region}: that type identifies one
// board WITHIN a run (a specific region-variant, used to key the recovery probe's cache), while
// providerBoard exists specifically to GROUP a run's entries by (provider, board) so their
// distinct regions can be counted — reusing boardKey here would make every region its own key
// and defeat the grouping this type exists for.
type providerBoard struct{ provider, board string }

// ambiguousRegionBoards returns the (provider, board) pairs that appear under more than one
// distinct region across entries. The `boards` catalog allows one board name to exist twice
// under a provider, distinguished only by region
// (`UNIQUE(provider, lower(board), region)` — see internal/ingest/boardcatalog), but
// CloseUnseenJobsForBoard's `external_id LIKE '<board>:%'` predicate has no region dimension
// at all, so it cannot tell which region's crawl proved coverage. Qualifying such a board from
// one region's outcome alone risks closing postings only a DIFFERENT region's crawl keeps
// alive, so an ambiguous board name never qualifies through this scope, regardless of how any
// single region's crawl went — it falls back to the company scope instead.
func ambiguousRegionBoards(entries []sources.CompanyEntry) map[providerBoard]bool {
	regions := make(map[providerBoard]map[string]bool)
	for _, e := range entries {
		if e.Board == "" {
			continue
		}
		pb := providerBoard{e.Provider, e.Board}
		if regions[pb] == nil {
			regions[pb] = make(map[string]bool)
		}
		regions[pb][e.Region] = true
	}
	ambiguous := make(map[providerBoard]bool)
	for pb, rs := range regions {
		if len(rs) > 1 {
			ambiguous[pb] = true
		}
	}
	return ambiguous
}

// AmbiguousBoardNames is exported so cmd/ingest can compute region-ambiguity against the
// FULL, unsharded board list before sources.Config.Shard splits it across processes. Shard
// groups by company slug, not by board, so two CompanyEntry rows sharing (provider, board)
// under different regions but
// different companies — exactly the shape ambiguousRegionBoards guards against — can land in
// separate shard processes, each running its own Runner.Run with only its own slice. Each
// shard's own Run() then sees just one region for that board name and concludes it is
// unambiguous, even though the two regions together are not — a per-Run() check alone cannot
// catch a split its caller introduced after the check already ran. cmd/ingest calls this on
// entries loaded straight from the catalog, before Shard, and filters its board-scope close
// against the result regardless of which shard actually produced a qualifying board.
//
// Reports by board name alone, not by (provider, board): a caller crawls one provider per
// run (cmd/ingest takes a single provider argument), so the provider component carries no
// information here.
func AmbiguousBoardNames(entries []sources.CompanyEntry) map[string]bool {
	ambiguous := ambiguousRegionBoards(entries)
	names := make(map[string]bool, len(ambiguous))
	for pb := range ambiguous {
		names[pb.board] = true
	}
	return names
}
