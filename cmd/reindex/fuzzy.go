package main

import (
	"context"
	"sort"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/jobhash"
)

const (
	// fuzzyThreshold is the word overlap two descriptions need before they count as the same
	// posting. The spike put per-city variants of one role at 0.95–1.00 and genuinely distinct
	// roles in the same bucket at or below 0.5, so anything inside that gap behaves identically;
	// 0.9 sits plainly inside it rather than being tuned to one sample.
	fuzzyThreshold = 0.9

	// fuzzyMaxBucket is the largest bucket this pass will read. Measured on prod, eleven buckets
	// hold more than a thousand postings each and account for 92% of all pairwise work — and they
	// are generic-title-by-location (one retailer's "customer service associate i" spans 16 745
	// stores), exactly the shape that must NOT collapse. Skipping them removes 97% of the cost and
	// 0.05% of the buckets, which is why this pass needs no MinHash or LSH.
	fuzzyMaxBucket = 200
)

// fuzzyDedupQuerier is the slice of the store this pass needs, named so the pass can be read
// (and faked) without the full generated querier.
type fuzzyDedupQuerier interface {
	CompaniesWithFuzzyDedupCandidates(ctx context.Context) ([]string, error)
	FuzzyDedupCandidateTitlesForCompany(ctx context.Context, company string) ([]db.FuzzyDedupCandidateTitlesForCompanyRow, error)
	GetJobDescriptionsByIDs(ctx context.Context, ids []int64) ([]db.GetJobDescriptionsByIDsRow, error)
	MarkFuzzyDuplicatesForCompany(ctx context.Context, arg db.MarkFuzzyDuplicatesForCompanyParams) (int64, error)
}

// collapseFuzzyDuplicates marks near-identical reposts the exact passes left split, one company at
// a time, and returns the rows re-marked. It runs LAST, over what the exact role-cluster and
// aggregator passes did not claim, so it only ever adds markers to leftovers and never contradicts
// a deterministic collapse.
//
// Descriptions are loaded only for buckets that survive the size filter — titles are cheap to ship,
// descriptions are not, and the largest bucket on prod holds 16 745 postings.
func collapseFuzzyDuplicates(ctx context.Context, q fuzzyDedupQuerier) (int64, error) {
	companies, err := q.CompaniesWithFuzzyDedupCandidates(ctx)
	if err != nil {
		return 0, err
	}
	return forEachCompany(ctx, companies, func(ctx context.Context, company string) (int64, error) {
		return collapseFuzzyDuplicatesForCompany(ctx, q, company)
	})
}

func collapseFuzzyDuplicatesForCompany(ctx context.Context, q fuzzyDedupQuerier, company string) (int64, error) {
	titles, err := q.FuzzyDedupCandidateTitlesForCompany(ctx, company)
	if err != nil {
		return 0, err
	}
	buckets := bucketByRole(titles)
	if len(buckets) == 0 {
		return 0, nil
	}

	var ids []int64
	for _, bucketIDs := range buckets {
		ids = append(ids, bucketIDs...)
	}
	descriptions, err := q.GetJobDescriptionsByIDs(ctx, ids)
	if err != nil {
		return 0, err
	}
	signatures := make(map[int64]map[string]struct{}, len(descriptions))
	for _, row := range descriptions {
		signatures[row.ID] = jobhash.DescriptionSignature(row.Description)
	}

	var moveIDs, canons []int64
	for _, bucketIDs := range buckets {
		postings := make([]fuzzyPosting, 0, len(bucketIDs))
		for _, id := range bucketIDs {
			postings = append(postings, fuzzyPosting{ID: id, Signature: signatures[id]})
		}
		for id, canon := range clusterBucket(postings, fuzzyThreshold) {
			moveIDs = append(moveIDs, id)
			canons = append(canons, canon)
		}
	}
	if len(moveIDs) == 0 {
		return 0, nil
	}
	return q.MarkFuzzyDuplicatesForCompany(ctx, db.MarkFuzzyDuplicatesForCompanyParams{
		Ids:     moveIDs,
		Canons:  canons,
		Company: company,
	})
}

// fuzzyPosting is one open canonical posting inside a (company, title) bucket.
type fuzzyPosting struct {
	ID        int64
	Signature map[string]struct{}
}

// bucketByRole groups one company's open canonical postings by normalized title, keeping only the
// buckets worth comparing: more than one member, and no more than fuzzyMaxBucket of them.
//
// The key comes from jobhash.RoleKey, the same normalization the rest of the codebase groups roles
// by, so a per-city variant lands on its base role here exactly as it does elsewhere. Deriving it
// in Go rather than in SQL keeps one definition — a second copy in the query would drift.
//
// A title that normalizes to nothing yields no key and is dropped, per RoleKey's contract: every
// blank title would otherwise share one bucket and merge unrelated postings.
func bucketByRole(rows []db.FuzzyDedupCandidateTitlesForCompanyRow) map[string][]int64 {
	buckets := make(map[string][]int64)
	for _, row := range rows {
		// The company is fixed for the whole call, so it adds nothing to the key here.
		if key := jobhash.RoleKey("", row.Title); key != "" {
			buckets[key] = append(buckets[key], row.ID)
		}
	}
	for key, ids := range buckets {
		if len(ids) < 2 || len(ids) > fuzzyMaxBucket {
			delete(buckets, key)
		}
	}
	return buckets
}

// clusterBucket assigns near-identical postings in one bucket to a canonical id, returning only
// the postings that MOVE — each mapped to its canon. The canon is the lowest id in its cluster, so
// the result is stable across runs and a re-run is a no-op.
//
// Membership is decided against the canon, never transitively. Similarity is not an equivalence
// relation: A can resemble B and B resemble C while A and C share almost nothing, and following
// that chain would walk a bucket into one blob. Comparing every candidate to the canon keeps a
// cluster as tight as its canon.
//
// Callers must only pass buckets keyed by a real company and title — the bucket is what stops this
// from merging unrelated roles, since two boilerplate-heavy postings of different jobs at one
// employer can share 0.98 of their vocabulary.
func clusterBucket(postings []fuzzyPosting, threshold float64) map[int64]int64 {
	if len(postings) < 2 {
		return map[int64]int64{}
	}
	ordered := make([]fuzzyPosting, len(postings))
	copy(ordered, postings)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })

	moved := make(map[int64]int64)
	claimed := make(map[int64]bool, len(ordered))
	for i, canon := range ordered {
		if claimed[canon.ID] {
			continue
		}
		for _, other := range ordered[i+1:] {
			if claimed[other.ID] {
				continue
			}
			if jobhash.DescriptionSimilarity(canon.Signature, other.Signature) >= threshold {
				claimed[other.ID] = true
				moved[other.ID] = canon.ID
			}
		}
	}
	return moved
}
