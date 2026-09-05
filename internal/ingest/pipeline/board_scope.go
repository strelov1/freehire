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

// boardQualifies reports whether a run structurally PROVED it covered a board — the fact the
// post-run sweep's board-scoped close needs before it may retire anything within that board
// (freehire#2328, docs/agents/job-lifecycle.md). Two conditions, both read directly off this
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
//
// A boardless entry (e.Board == "") never qualifies: its postings are namespaced with an
// empty board (externalid.Namespace("", id)), so a board-scoped LIKE pattern built from it
// would match the provider's WHOLE catalogue rather than nothing.
func boardQualifies(e sources.CompanyEntry, st Stats) bool {
	return e.Board != "" && st.Failed == 0 && boardReachedPostings(st)
}

// providerBoard identifies a board by name alone, ignoring region — the key the board-scoped
// close's SQL predicate is forced to use, since externalid.Namespace does not encode region.
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
