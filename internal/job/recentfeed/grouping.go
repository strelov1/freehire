package recentfeed

import "github.com/strelov1/freehire/internal/job/jobhash"

// AggregationThreshold is the minimum number of same-role postings claimed in one
// poll batch before Group collapses them into a single aggregated Entry instead of
// one Entry per posting. A code constant rather than an env var: this is product
// behavior (what a visitor sees), not an operational safety valve for a one-off
// run — see design.md, "Aggregation threshold is a code constant".
const AggregationThreshold = 5

// Posting is what Group needs from one claimed outbox row.
type Posting struct {
	Title       string
	CompanyName string
	JobSlug     string
}

// Group buckets postings by jobhash.NormalizedRoleTitle and turns each bucket into
// one or more Entry values: a bucket below AggregationThreshold yields one
// KindSingle Entry per posting; a bucket at or above it yields one KindAggregate
// Entry naming a representative posting from the bucket. Buckets are emitted in
// first-seen order, and postings within a KindSingle bucket keep their given
// order, so the result reads in the same order postings were claimed.
func Group(postings []Posting) []Entry {
	if len(postings) == 0 {
		return nil
	}

	var order []string
	buckets := make(map[string][]Posting, len(postings))
	for _, p := range postings {
		key := jobhash.NormalizedRoleTitle(p.Title)
		if _, seen := buckets[key]; !seen {
			order = append(order, key)
		}
		buckets[key] = append(buckets[key], p)
	}

	entries := make([]Entry, 0, len(postings))
	for _, key := range order {
		bucket := buckets[key]
		if len(bucket) >= AggregationThreshold {
			sample := bucket[0]
			entries = append(entries, Entry{
				Kind:        KindAggregate,
				Title:       sample.Title,
				CompanyName: sample.CompanyName,
				Count:       len(bucket),
			})
			continue
		}
		for _, p := range bucket {
			entries = append(entries, Entry{
				Kind:        KindSingle,
				Title:       p.Title,
				CompanyName: p.CompanyName,
				JobSlug:     p.JobSlug,
			})
		}
	}
	return entries
}
