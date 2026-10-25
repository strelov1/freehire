package atsdetect

import "github.com/strelov1/freehire/internal/ingest/atsboard"

// FromURL classifies a single outbound job-application URL into the (provider, board)
// pair one of our source adapters can crawl, returning ok=false when the URL is not a
// supported ATS or its shape does not expose the exact board id the adapter expects
// (e.g. an ADP vanity path, a Join job link, or a Workable per-job shortlink). Unlike
// Detect, which scans arbitrary HTML, this parses one known URL — the shape an
// aggregator's apply link gives us directly.
//
// The shared table answers first. internal/ingest/atsboard is the ONE definition of "which board
// does this URL address", read by the contribution flow, link resolution and boardresolve;
// this package used to carry its own copy of eleven of those hosts, and the two had already
// drifted in both directions — atsdetect read a SmartRecruiters portal segment as the board,
// while atsboard read a lowercase Workday locale and a per-job "/job" path as career sites.
// Delegating also means an ATS added to atsboard is immediately visible to the harvest tools,
// which was the standing cost of the split.
//
// Nothing is read here any more. The split used to guard a money-affecting accept-set —
// internal/ingest/contribution paid for onboarded boards — and once that reward was retired the
// shapes it kept out moved into the shared table one by one: iCIMS, then NEOGOV, Paycom and
// Comeet, and last Oracle and Taleo Enterprise.
func FromURL(rawurl string) (provider, board string, ok bool) {
	src, brd, _, found := atsboard.Recognize(rawurl)
	return src, brd, found
}
