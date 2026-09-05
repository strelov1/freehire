package viewlog

import (
	"bufio"
	"io"
)

// dayLayout is the UTC calendar-day key ("2006-01-02") used for bucketing and as
// the job_daily_views day.
const dayLayout = "2006-01-02"

// Counts is one (day, slug) result: two visitor counts, deduplicated independently
// over the same visitor key.
//
// They are two counts and not a count plus a breakdown, because a visitor who both
// opens the page and reads the API on that day is ONE visitor in each. Total and
// Page therefore do not sum with an API count, and nothing should try to derive one
// from the other beyond the invariant that Page never exceeds Total.
type Counts struct {
	// Total counts distinct visitors who produced either counted signal. This is what
	// job_daily_views.uniques has always held and what GET /api/v1/stats/catalog
	// publishes, so its value must not move.
	Total int
	// Page counts distinct visitors who opened the page. That signal is bot-filtered
	// and the API signal deliberately is not, which makes Page the only one of the two
	// safe to rank a public list on — see internal/engage/socialdigest.
	Page int
}

// Visitor-key states. One map with flags rather than two sets: both counters dedup
// over the identical key, so a second map would be a second lookup that can only ever
// hold a subset of the first's keys — and the "Page implies seen" invariant is safer
// expressed in the bits than in the agreement of two containers.
const (
	sawVisitor = 1 << iota
	sawPageOpen
)

// Aggregate reads access-log lines from r and returns unique view counts bucketed
// by UTC day: result[day][slug] holds the distinct visitors who viewed that job on
// that day. A visitor's identity is client IP + User-Agent, and the dedup key is
// (visitor, slug, day) — the day taken from each line's timestamp — so a visitor
// counts at most once per job per day in each of the two counters. Page opens from
// known bots are dropped; API reads are never bot-filtered. Unparseable and non-view
// lines are skipped.
//
// Memory is bounded by the distinct (visitor, slug, day) pairs in r — the natural
// size of the result — so one rotated file fits comfortably in memory.
func Aggregate(r io.Reader) (map[string]map[string]Counts, error) {
	counts := make(map[string]map[string]Counts)
	seen := make(map[string]uint8)

	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		rec, ok := ParseLine(sc.Text())
		if !ok {
			continue
		}
		sig, ok := Classify(rec)
		if !ok {
			continue
		}
		if sig.Kind == KindPage && isBot(rec.UserAgent) {
			continue
		}
		day := rec.Time.UTC().Format(dayLayout)
		key := rec.IP + "\x00" + rec.UserAgent + "\x00" + sig.Slug + "\x00" + day

		// What this line adds that the visitor has not already contributed. A visitor
		// whose first line was an API read still becomes a page visitor on a later page
		// open, so the order of a day's lines cannot decide the attribution.
		var add Counts
		state := seen[key]
		if state&sawVisitor == 0 {
			state |= sawVisitor
			add.Total = 1
		}
		if sig.Kind == KindPage && state&sawPageOpen == 0 {
			state |= sawPageOpen
			add.Page = 1
		}
		if add == (Counts{}) {
			continue
		}
		seen[key] = state

		if counts[day] == nil {
			counts[day] = make(map[string]Counts)
		}
		c := counts[day][sig.Slug]
		c.Total += add.Total
		c.Page += add.Page
		counts[day][sig.Slug] = c
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return counts, nil
}
