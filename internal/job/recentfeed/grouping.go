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
	// CompanySlug is the canonical company identity Group's company-aggregation
	// pass keys on — never CompanyName, which is free text and varies in
	// spelling/punctuation for one employer (see AGENTS.md, "Company key:
	// normalize.CompanySlug", and docs/agents/company-identity.md).
	CompanySlug string
	JobSlug     string
}

// Group runs two aggregation passes over a claimed batch, so a single posting is
// never counted toward both:
//
//  1. Bucket by jobhash.NormalizedRoleTitle. A bucket at or above
//     AggregationThreshold — the same role posted by several different
//     companies — collapses into one KindAggregate entry naming a
//     representative posting. Everything else is left over for pass 2.
//  2. Bucket what's left by CompanySlug (never CompanyName — see Posting).
//     A bucket at or above AggregationThreshold — one company posting several
//     different roles at once, a mass hiring push — collapses into one
//     KindCompanyAggregate entry. Everything still left over becomes a
//     KindSingle entry.
//
// Buckets in each pass are emitted in first-seen order. Because pass 2 only
// ever sees postings pass 1 did not already aggregate, the result partitions
// the input exactly — role-aggregates first, then company-aggregates and
// remaining singles.
func Group(postings []Posting) []Entry {
	if len(postings) == 0 {
		return nil
	}

	roleAggregates, leftover := aggregateBy(postings, KindAggregate, func(p Posting) string {
		return jobhash.NormalizedRoleTitle(p.Title)
	})
	companyAggregates, leftover := aggregateBy(leftover, KindCompanyAggregate, func(p Posting) string {
		return p.CompanySlug
	})

	entries := make([]Entry, 0, len(postings))
	entries = append(entries, roleAggregates...)
	entries = append(entries, companyAggregates...)
	for _, p := range leftover {
		entries = append(entries, Entry{
			Kind:        KindSingle,
			Title:       p.Title,
			CompanyName: p.CompanyName,
			JobSlug:     p.JobSlug,
		})
	}
	return entries
}

// aggregateBy buckets postings by key(p), preserving first-seen bucket order. A
// bucket at or above AggregationThreshold becomes one Entry of the given kind,
// naming a representative posting from it; every posting in a smaller bucket is
// returned as leftover instead, for the caller to aggregate further or emit as
// KindSingle. The two Group passes differ only in kind and key — this is the
// bucket-or-leave-for-later shape both share.
func aggregateBy(postings []Posting, kind Kind, key func(Posting) string) (aggregates []Entry, leftover []Posting) {
	var order []string
	buckets := make(map[string][]Posting, len(postings))
	for _, p := range postings {
		k := key(p)
		if _, seen := buckets[k]; !seen {
			order = append(order, k)
		}
		buckets[k] = append(buckets[k], p)
	}

	for _, k := range order {
		bucket := buckets[k]
		if len(bucket) < AggregationThreshold {
			leftover = append(leftover, bucket...)
			continue
		}
		sample := bucket[0]
		aggregates = append(aggregates, Entry{
			Kind:        kind,
			Title:       sample.Title,
			CompanyName: sample.CompanyName,
			Count:       len(bucket),
		})
	}
	return aggregates, leftover
}
