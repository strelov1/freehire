package sources

import "time"

// The capability markers. Adapters declare them by method (FullBoardListing, Aggregator, ...);
// the registry's classifiers in this package read them.

// boardless marks an adapter whose API has no per-tenant board id, so config
// validation lets its entries omit board. A boardless adapter may serve one company
// (greenhouse/lever and the other multi-tenant ATS adapters are NOT boardless and
// still require a board) or aggregate many (see aggregator).
type boardless interface{ Boardless() }

// boardValidator is an adapter that can tell, from the board id alone, that a board is not one
// it may be handed: a spelling that only ever names some other board. Config.Validate asks it,
// so the catalogue stores such a candidate as rejected with the reason instead of crawling it.
type boardValidator interface{ ValidateBoard(board string) error }

// aggregator marks a boardless adapter that aggregates postings from many companies
// (e.g. jobstash) rather than serving a single company. It keeps such an adapter in
// the source facet: a single-company boardless platform is redundant with the company
// filter and excluded, but filtering by an aggregator is not.
type aggregator interface{ Aggregator() }

// social marks an adapter that reads postings out of a community channel (a forum thread,
// a chat) rather than a job board. It wins over aggregator in ProviderKind: such a source
// may still carry the aggregator marker for the dedup and coverage passes, which treat its
// postings as copies, but what it IS to a reader choosing sources is a community channel.
type social interface{ Social() }

// unknownOrigin marks a generic adapter whose board can be any site — an employer's, an ATS
// tenant's or a job board's — so the adapter alone cannot say what kind of source a posting
// came from. ProviderKind answers KindOther for it rather than guessing.
type unknownOrigin interface{ UnknownOrigin() }

// selfClosing marks an adapter that closes its own removed postings (via a Job with
// Removed set, emitted from its stream) and therefore must be excluded from the post-run
// unseen sweep. Such a source re-reports only changed postings each run, so the sweep's
// last_seen_at cutoff would wrongly close every still-open posting it did not touch; the
// stream's removal events are the authoritative close signal instead. See UnseenSweepExemptProviders.
type selfClosing interface{ SelfClosing() }

// probeClosing marks a bounded search whose missing postings can only be closed by
// cmd/liveness, never by a company-, board-, or source-scoped unseen sweep.
type probeClosing interface{ ProbeClosing() }

// fullCatalog marks an aggregator whose every crawl lists the WHOLE catalogue in one run —
// not a per-company board and not a subset. For such a source an open job the run did not
// see is genuinely gone, so the post-run sweep may close it by source alone, dropping the
// crawled-company scope that would otherwise leak the postings of a company that vanished
// from the feed entirely (see cmd/ingest's sweep). The marker is only sound when the adapter
// FAILS a truncated crawl (returns an error, not a partial success): a silently-truncated
// run looks like a shrunken catalogue and a source-scoped close would retire everything it
// never reached. cmd/ingest gates the source-scoped close on a zero-Failed run for exactly
// this reason. See FullCatalogProviders.
type fullCatalog interface{ FullCatalog() }

// sliceBoardListing marks a sweepGrace adapter whose boards are bounded SLICES rather than
// complete listings — whatjobs, where a board is a search keyword read up to a page budget or
// until relevance collapses — and admits it to the post-run sweep's board-scoped close ON ITS
// GRACE WINDOW. Its stored ids are board-namespaced like every other provider's, so a posting a
// keyword board stopped listing is visible per board; what it lacks is fullBoardListing's proof
// that one crawl read the whole board, and the grace window stands in for that proof: a posting
// that merely sat past the budget comes back within it, one the keyword stopped carrying does
// not. Without the marker such a provider closed only through the company scope, which leaked
// every posting whose company had drifted out of every slice (only cmd/liveness's age rule ever
// reached them). Granting it to an adapter without a sweepGrace would close on the 48h default
// every posting past the budget, so a test holds every member to one. See
// SliceBoardListingProviders.
type sliceBoardListing interface{ SliceBoardListing() }

// sweepGrace marks an adapter that needs the post-run unseen sweep to wait a DIFFERENT length of
// time than the 48h default before closing its jobs, and reports how long. The default is sized
// for "many crawl cycles at the hourly per-provider cadence" (see DefaultSweepGrace); an adapter
// declares its own window when that assumption does not hold for it, in either direction:
//
//   - WIDER, when the crawl deliberately reaches only a SLICE of the source's catalogue (a
//     keyword's first N pages of a feed far deeper than any crawl can walk), so a posting that
//     merely drifted past that depth reads as unseen. On the default window it would be closed
//     and then reopened when it drifts back — churn that also lands in job_daily_stats as a
//     phantom removal. A window wider than the drift absorbs it, at the cost of a genuinely
//     withdrawn posting lingering that long. Sound only for a source whose postings CANNOT be
//     probed for liveness; anything verifiable should be closed on evidence instead.
//   - NARROWER, when the crawl lists a board's WHOLE posting set every run, so an unseen posting
//     is already positive evidence of removal and the 48h default only adds latency nothing
//     needs (gem: see gemSweepGrace). Sound only when the fleet's own crawl cadence for that
//     provider is comfortably tighter than the declared window, with room to spare for a missed
//     run or two — narrowing the SHARED default instead would falsely close the much slower,
//     jitterier providers that still rely on it (workday, smartrecruiters).
//
// See SweepGraceWindows.
type sweepGrace interface{ SweepGrace() time.Duration }

// livenessProbed marks an adapter whose crawl cannot re-list every live posting it holds — it
// reads only the newest slice of a feed deeper than any run can walk, so an older posting is
// never listed again however long it stays up — and whose removals are therefore judged by a
// probe of the source's own page instead (cmd/liveness's probeDespiteRegistered,
// cmd/adzuna-liveness). For such a source a stale last_seen_at says how old the posting is,
// not whether it is gone, so search's unseen-posting rule leaves it to the probe (see
// UnseenWindows). Grant it only together with a probe that actually closes the source's dead
// postings: without one the marker is a promise nothing keeps.
type livenessProbed interface{ LivenessProbed() }

// fullBoardListing marks an adapter whose Fetch structurally proves, for every board it
// crawls, that it retrieved the board's full posting list — either by verifying a fetched
// count against the source's own reported total, or by paginating to the source's natural
// end (an empty page, hasNext=false, or a declared page count reached) with no artificial
// page or offset cap in adapter code — and treats any failure to establish that proof as a
// failed Fetch rather than a partial success. Only such an adapter's boards are safe for the
// post-run sweep's board-scoped close: a silently truncated crawl reported as an unqualified
// success would let the sweep close the unreached remainder as if it were gone. The bar is
// deliberately structural rather than a documented assumption: a cap recorded only in a
// comment is not enough, so this marker must not be grantable on that evidence. See
// habrcareer.go's fullCatalog Fetch for the pattern (any page failure is a hard error, never a
// partial result) and FullBoardListingProviders.
type fullBoardListing interface{ FullBoardListing() }

// emptyBoardListing marks a fullBoardListing adapter whose Fetch returns an EMPTY list only when
// the source itself stated that the board carries no postings, so a finished crawl that listed
// nothing may close the board's stored postings through the ordinary board-scoped close.
// fullBoardListing alone does not prove that: its bar is about a listing being read to its END,
// and an answer that never had a listing in it — a challenge page, an HTML error page served
// with 200, a JSON envelope without its posting collection, a page whose markup moved so that
// nothing parses — ends a walk on its first page and looks exactly like an empty board. Without
// this marker such a board's last postings stay open until the empty-feed net
// (cmd/close-chronic-boards) closes them; with it, a wrongly granted adapter would close a live
// board's whole catalogue on the 48h window.
//
// The bar: a non-2xx answer is a failed Fetch, an answer that does not decode as the source's
// listing is a failed Fetch, and an answer that decodes but carries no posting collection at
// all (the key absent or null) is a failed Fetch too (errListingAbsent) — only the collection
// present and empty is an empty board. A source that answers an UNKNOWN board with an empty
// listing (SmartRecruiters' totalFound=0 for any company identifier) does not qualify: the
// empty answer is then also what a failed lookup looks like. An HTML-scraped listing does not
// qualify either, since zero links parsed is what moved markup looks like.
// Granted per adapter after that check, never by sweeping the fullBoardListing set — a test
// holds every member to fullBoardListing and to the default sweep window. See
// EmptyBoardListingProviders.
type emptyBoardListing interface{ EmptyBoardListing() }
