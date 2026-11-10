package pipeline

// Stats reports what a run did: Ingested counts saved jobs, Failed counts boards that
// errored (unknown provider or a fetch failure), Skipped counts jobs that fetched fine
// but failed to persist, Cooled counts boards skipped because they are in cooldown (a
// deliberate back-off, distinct from a failure), and Rejected counts postings the
// catalogue filter turned away before the write path. Skipped is surfaced so a run whose
// every save fails (e.g. a DB schema drift) is not mistaken for a clean success.
//
// Rejected is kept apart from Skipped because the two mean opposite things: a rejection
// is the filter working as intended, a skip is something broken. Folded together, a board
// whose every save fails would read as a board full of non-technical postings.
type Stats struct {
	Ingested int
	Failed   int
	Skipped  int
	Cooled   int
	// Quiet counts boards skipped because their listing kept coming back unchanged (QuietFor) —
	// kept apart from Cooled, which means a FAILING board.
	Quiet      int
	Rejected   int
	ATSCovered int // aggregator postings skipped: company already covered by a non-aggregator source
	// Refreshed counts the stored open rows among covered or unreadable postings whose liveness the
	// run refreshed (CoveredRefresher). Not Ingested — no content was written — but evidence the
	// crawl worked all the same, so cmd/ingest sweeps a provider whose every posting was covered.
	Refreshed int
	// RefreshFailed counts covered or unreadable postings whose refresh failed. The listing named them,
	// but their stored rows keep the last_seen_at they had, so the board's close scopes are
	// withheld (boardQualifies, cmd/ingest's sweepBySource): reading that age as absence would
	// close a posting still listed, and a covered row is never written again to reopen it.
	RefreshFailed int
	// Unreadable counts postings the board's listing NAMED but whose detail this crawl could
	// not read (sources.Job.Unreadable). It is deliberately not Failed: the board answered, so
	// failing it would cool a working board and freeze its whole catalogue over a per-posting
	// hole. What it does instead is withhold this run's close scopes for the board — an
	// unreadable posting is missing from the crawl's evidence, and the sweep must not read a
	// posting it could not see as a posting that is gone. See boardReadWhatItListed.
	Unreadable int
	// Interrupted counts boards whose crawl was still under way when the run's context ended —
	// the crawl deadline cmd/ingest sets ahead of its unit's timeout, or a signal. Such a board
	// stopped at an arbitrary point: its fetch, or the writes that refresh last_seen_at, reached
	// only part of it, so it must never prove coverage (boardQualifies). It is not Failed: the
	// board did nothing wrong, the run simply ran out of time around it.
	Interrupted int
	// SeenUnknown counts boards a hydrating adapter crawled without the catalogue's seen-set,
	// because the lookup failed. Every stored posting then reads as new, so a budgeted adapter
	// yields only its budget of them and refreshes none of the rest: the crawl is no evidence
	// of what is gone, and the board's close scopes are withheld (boardQualifies, the company
	// scope through UnprovenCompanies, cmd/ingest's source scope).
	SeenUnknown int
	// QualifyingBoards lists, per provider once merged into RunStats, the boards this run
	// structurally PROVED it covered — see boardQualifies. It is the post-run sweep's board
	// scope input (freehire#2328): cmd/ingest still gates a board-scoped close on the
	// provider's adapter carrying the fullBoardListing marker before using any of these.
	QualifyingBoards []BoardRef
	// EmptyBoards lists the boards this run crawled to completion whose listing named no
	// posting at all — see boardListedEmpty. cmd/ingest adds them to the board scope only for a
	// provider whose adapter is registered as proving an empty answer (emptyBoardListing on top
	// of fullBoardListing); for every other provider an empty answer may be a broken crawl.
	EmptyBoards []BoardRef
	// UnprovenCompanies lists the company slugs whose postings this run may NOT be used as
	// evidence to close, because a board they are filed under did not read what it listed
	// (boardReadWhatItListed). cmd/ingest subtracts them from the crawled-company scope of
	// CloseUnseenJobs; the run still records everything it did manage to read, so the fix
	// costs latency on a genuine removal and never a live posting.
	//
	// Carried per company rather than per board because the company scope is what the sweep
	// speaks: a board's whole company set is withheld, not just the employers of the postings
	// that failed, since a board that could not read part of itself has proved nothing about
	// any of the employers on it.
	UnprovenCompanies []string
	// SkippedBoards is the externalid.BoardPattern of every board this run skipped — in quiet
	// pause or in failure cooldown. The company- and source-scoped unseen closes exclude them: a
	// skipped board was not looked at, so its rows' age since last seen says nothing about them,
	// and a quiet pause lasts longer than the sweep's 48h window.
	SkippedBoards []string
}

// add accumulates another Stats into s, so the per-board and per-provider merges cannot
// drift apart when a counter is added.
func (s *Stats) add(o Stats) {
	s.Ingested += o.Ingested
	s.Failed += o.Failed
	s.Skipped += o.Skipped
	s.Cooled += o.Cooled
	s.Quiet += o.Quiet
	s.Rejected += o.Rejected
	s.ATSCovered += o.ATSCovered
	s.Refreshed += o.Refreshed
	s.RefreshFailed += o.RefreshFailed
	s.Unreadable += o.Unreadable
	s.Interrupted += o.Interrupted
	s.SeenUnknown += o.SeenUnknown
	s.QualifyingBoards = append(s.QualifyingBoards, o.QualifyingBoards...)
	s.EmptyBoards = append(s.EmptyBoards, o.EmptyBoards...)
	s.UnprovenCompanies = append(s.UnprovenCompanies, o.UnprovenCompanies...)
	s.SkippedBoards = append(s.SkippedBoards, o.SkippedBoards...)
}

// RunStats is a run's outcome broken down by provider. A run may cover several providers
// (a mixed board file), and the post-run unseen-job sweep is per provider, so the breakdown
// is kept rather than a single aggregate; Total folds it back when only the sum is needed.
type RunStats map[string]Stats

// Total sums the per-provider stats into one aggregate (for the run's done-log line).
func (rs RunStats) Total() Stats {
	var t Stats
	for _, s := range rs {
		t.add(s)
	}
	return t
}
