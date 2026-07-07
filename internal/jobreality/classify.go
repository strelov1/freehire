// Package jobreality derives a deterministic "reality" classification of a job —
// whether it looks like a real, active opening or a perpetual ghost/evergreen
// listing — from ingest history and text facts. It is the same "never guesses"
// doctrine as internal/location / classify / skilltag, but computed from history
// (first-seen age, repost/mass-posting counts) rather than content alone, so it is
// TIME-DEPENDENT and computed at index/read time, never stored.
package jobreality

import "time"

// Reality classes.
const (
	ClassFresh           = "fresh"
	ClassStale           = "stale"
	ClassLikelyEvergreen = "likely-evergreen"
)

// Thresholds. One signal is weak; the verdict requires convergence.
const (
	freshWindowDays = 14 // age at or under which a job with no evergreen signal is fresh
	oldAgeDays      = 90 // age at or over which the "old" signal fires
	repostThreshold = 3  // distinct postings of one role (any status) that count as reposting
	massThreshold   = 5  // concurrent open postings of one role that count as mass-posting
	convergence     = 2  // number of independent signals required for likely-evergreen
)

// Input is the history + text facts a classification reads. RepostCount is the number
// of distinct external_ids of any status sharing the role fingerprint (repost
// history); MassPostingCount is the same restricted to open jobs (concurrent
// mass-posting). Both are at least 1 (the job itself). EvergreenText is the dictionary
// result (see HasEvergreenMarker). PostedAt is honored only when HasPostedAt is set.
type Input struct {
	Now              time.Time
	CreatedAt        time.Time
	PostedAt         time.Time
	HasPostedAt      bool
	RepostCount      int
	MassPostingCount int
	EvergreenText    bool
}

// Evidence is the observable facts behind a classification — surfaced so the UI states
// facts, not a bare accusation.
type Evidence struct {
	AgeDays          int
	RepostCount      int
	MassPostingCount int
	FakeFreshness    bool
}

// Result is a classification and the evidence that produced it.
type Result struct {
	Class    string
	Evidence Evidence
}

// Classify assigns a reality class from the input. likely-evergreen requires at least
// `convergence` independent signals to fire — never age alone; otherwise a new job
// with no evergreen signal is fresh and everything else is stale.
func Classify(in Input) Result {
	ageDays := int(in.Now.Sub(in.CreatedAt).Hours() / 24)

	old := ageDays >= oldAgeDays
	// Reposting and mass-posting must be INDEPENDENT signals: MassPostingCount (open)
	// is a subset of RepostCount (any status), so counting both raw would let one
	// concurrent spray fire two signals and reach the verdict alone. The repost signal
	// therefore counts only the HISTORICAL churn — reposts beyond the concurrent open
	// ones (roles closed and re-posted under new ids over time).
	massPosted := in.MassPostingCount >= massThreshold
	reposted := in.RepostCount-in.MassPostingCount >= repostThreshold

	signals := 0
	for _, on := range []bool{old, reposted, massPosted, in.EvergreenText} {
		if on {
			signals++
		}
	}

	fakeFreshness := old && in.HasPostedAt && int(in.Now.Sub(in.PostedAt).Hours()/24) <= freshWindowDays

	class := ClassStale
	switch {
	case signals >= convergence:
		class = ClassLikelyEvergreen
	case ageDays <= freshWindowDays && !in.EvergreenText:
		class = ClassFresh
	}

	return Result{
		Class: class,
		Evidence: Evidence{
			AgeDays:          ageDays,
			RepostCount:      in.RepostCount,
			MassPostingCount: in.MassPostingCount,
			FakeFreshness:    fakeFreshness,
		},
	}
}
