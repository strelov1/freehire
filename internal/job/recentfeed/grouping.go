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

// Group runs two aggregation passes over a claimed batch, so a single posting is
// never counted toward both:
//
//  1. Bucket by jobhash.NormalizedRoleTitle. A bucket at or above
//     AggregationThreshold — the same role posted by several different
//     companies — collapses into one KindAggregate entry naming a
//     representative posting. Everything else becomes a candidate for pass 2.
//  2. Bucket the remaining candidates by CompanyName. A bucket at or above
//     AggregationThreshold — one company posting several different roles at
//     once, a mass hiring push — collapses into one KindCompanyAggregate
//     entry. Everything still left over becomes a KindSingle entry.
//
// Buckets in each pass are emitted in first-seen order. Because pass 2 only
// ever sees postings pass 1 did not already aggregate, the result partitions
// the input exactly — role-aggregates first, then company-aggregates and
// remaining singles.
func Group(postings []Posting) []Entry {
	if len(postings) == 0 {
		return nil
	}

	entries := make([]Entry, 0, len(postings))

	var roleOrder []string
	roleBuckets := make(map[string][]Posting, len(postings))
	for _, p := range postings {
		key := jobhash.NormalizedRoleTitle(p.Title)
		if _, seen := roleBuckets[key]; !seen {
			roleOrder = append(roleOrder, key)
		}
		roleBuckets[key] = append(roleBuckets[key], p)
	}

	var candidates []Posting
	for _, key := range roleOrder {
		bucket := roleBuckets[key]
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
		candidates = append(candidates, bucket...)
	}

	var companyOrder []string
	companyBuckets := make(map[string][]Posting, len(candidates))
	for _, p := range candidates {
		if _, seen := companyBuckets[p.CompanyName]; !seen {
			companyOrder = append(companyOrder, p.CompanyName)
		}
		companyBuckets[p.CompanyName] = append(companyBuckets[p.CompanyName], p)
	}

	for _, company := range companyOrder {
		bucket := companyBuckets[company]
		if len(bucket) >= AggregationThreshold {
			sample := bucket[0]
			entries = append(entries, Entry{
				Kind:        KindCompanyAggregate,
				Title:       sample.Title,
				CompanyName: company,
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
