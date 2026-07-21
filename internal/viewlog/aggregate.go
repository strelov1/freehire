package viewlog

import (
	"bufio"
	"io"
)

// dayLayout is the UTC calendar-day key ("2006-01-02") used for bucketing and as
// the job_daily_views day.
const dayLayout = "2006-01-02"

// Aggregate reads access-log lines from r and returns unique view counts bucketed
// by UTC day: result[day][slug] is the number of distinct visitors who viewed that
// job on that day. A visitor's identity is client IP + User-Agent, and the dedup
// key is (visitor, slug, day) — the day taken from each line's timestamp — so a
// visitor counts at most once per job per day. Page opens from known bots are
// dropped; API reads are never bot-filtered. Unparseable and non-view lines are
// skipped.
//
// Memory is bounded by the distinct (visitor, slug, day) pairs in r — the natural
// size of the result — so one rotated file fits comfortably in memory.
func Aggregate(r io.Reader) (map[string]map[string]int, error) {
	counts := make(map[string]map[string]int)
	seen := make(map[string]struct{})

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
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		if counts[day] == nil {
			counts[day] = make(map[string]int)
		}
		counts[day][sig.Slug]++
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return counts, nil
}
