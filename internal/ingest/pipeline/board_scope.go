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
