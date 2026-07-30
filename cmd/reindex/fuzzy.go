package main

import (
	"sort"

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

// fuzzyPosting is one open canonical posting inside a (company, title) bucket.
type fuzzyPosting struct {
	ID        int64
	Signature map[string]struct{}
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
