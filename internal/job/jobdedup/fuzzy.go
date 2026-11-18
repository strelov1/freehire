package jobdedup

import (
	"context"
	"fmt"
	"sort"

	"github.com/strelov1/freehire/internal/job/jobhash"
	"github.com/strelov1/freehire/internal/platform/db"
)

const (
	// FuzzyThreshold is the word overlap two descriptions need before they count as the same
	// posting. The spike put per-city variants of one role at 0.95–1.00 and genuinely distinct
	// roles in the same bucket at or below 0.5, so anything inside that gap behaves identically;
	// 0.9 sits plainly inside it rather than being tuned to one sample.
	FuzzyThreshold = 0.9

	// FuzzyMaxBucket is the largest bucket this pass will read. Measured on prod, eleven buckets
	// hold more than a thousand postings each and account for 92% of all pairwise work — and they
	// are generic-title-by-location (one retailer's "customer service associate i" spans 16 745
	// stores), exactly the shape that must NOT collapse. Skipping them removes 97% of the cost and
	// 0.05% of the buckets, which is why this pass needs no MinHash or LSH.
	FuzzyMaxBucket = 200
)

// FuzzyPosting is one open canonical posting inside a (company, title) bucket.
type FuzzyPosting struct {
	ID        int64
	Signature map[string]struct{}
}

// BucketByRole groups one company's open unclaimed postings by normalized title and splits them
// into the two sets the marker write needs.
//
//   - judged: every row this pass reaches a verdict on. A row here with no assignment is
//     RELEASED, so the set has to be exactly what the pass is willing to decide.
//   - comparable: the buckets worth loading descriptions for — more than one member, and no
//     more than FuzzyMaxBucket of them.
//
// The difference between them is the difference between "no cluster" and "not judged", and
// getting it wrong is destructive in one direction. A bucket OVER the cap is not judged: the cap
// is a cost heuristic (eleven buckets on prod hold 92% of all pairwise work), so treating its
// members as unclustered would un-collapse the largest groups in the catalogue on a decision
// about compute. A bucket UNDER two, or a title that normalizes to nothing, IS judged — such a
// row cannot cluster with anyone, and saying so is what frees a marker whose canon has closed.
//
// The key comes from jobhash.RoleKey, the same normalization the rest of the codebase groups roles
// by, so a per-city variant lands on its base role here exactly as it does elsewhere. Deriving it
// in Go rather than in SQL keeps one definition — a second copy in the query would drift.
//
// A title that normalizes to nothing yields no key, per RoleKey's contract: every blank title
// would otherwise share one bucket and merge unrelated postings.
func BucketByRole(rows []db.FuzzyDedupCandidateTitlesForCompanyRow) (judged []int64, comparable map[string][]int64) {
	buckets := make(map[string][]int64)
	for _, row := range rows {
		// The company is fixed for the whole call, so it adds nothing to the key here.
		key := jobhash.RoleKey("", row.Title)
		if key == "" {
			judged = append(judged, row.ID)
			continue
		}
		buckets[key] = append(buckets[key], row.ID)
	}
	comparable = make(map[string][]int64, len(buckets))
	for key, ids := range buckets {
		if len(ids) > FuzzyMaxBucket {
			continue // not judged: skipped on cost, so its members keep whatever marker they hold
		}
		judged = append(judged, ids...)
		if len(ids) > 1 {
			comparable[key] = ids
		}
	}
	return judged, comparable
}

// ClusterBucket assigns near-identical postings in one bucket to a canonical id, returning only
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
func ClusterBucket(postings []FuzzyPosting, threshold float64) map[int64]int64 {
	if len(postings) < 2 {
		return map[int64]int64{}
	}
	ordered := make([]FuzzyPosting, len(postings))
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

// FuzzySyncMaxCandidates bounds the per-insert check: a company with more open unclaimed
// postings than this is left to the batch pass instead of paying a scan on every insert.
const FuzzySyncMaxCandidates = 5000

// fuzzyInsertQuerier is the slice of the store CanonicalForFuzzy needs.
type fuzzyInsertQuerier interface {
	FuzzyDedupCandidateTitlesForCompanyCapped(ctx context.Context, arg db.FuzzyDedupCandidateTitlesForCompanyCappedParams) ([]db.FuzzyDedupCandidateTitlesForCompanyCappedRow, error)
	GetJobDescriptionsByIDs(ctx context.Context, ids []int64) ([]db.GetJobDescriptionsByIDsRow, error)
}

// CanonicalForFuzzy reports the canon the batch fuzzy pass (cmd/reindex) would give the row just
// written (id), by running the batch's own rule over the same candidate set: BucketByRole for the
// bucket and its size cap, ClusterBucket for the greedy lowest-id clustering.
//
// ClusterBucket decides a row from the rows with a LOWER id only, so postings above id are
// dropped before clustering (they still count toward the bucket's size cap). A row therefore
// moves only onto an older canon, which is what keeps the next batch run from inverting it. The
// batch offers rows already carrying a fuzzy marker again, so this does too: both tiers cluster
// from scratch over every open unclaimed row.
//
// Run inside the write transaction, after the upsert (the row must be visible to the lookup).
// An empty company or title, an oversized bucket or an oversized company report no canon: the row
// is written unmarked and the batch decides. A failed lookup returns an error, which aborts a
// Postgres transaction: the caller must run this inside a savepoint and roll back to it.
func CanonicalForFuzzy(ctx context.Context, q fuzzyInsertQuerier, p db.UpsertJobParams, id int64) (int64, bool, error) {
	if p.CompanySlug == "" || jobhash.RoleKey("", p.Title) == "" {
		return 0, false, nil
	}
	rows, err := q.FuzzyDedupCandidateTitlesForCompanyCapped(ctx, db.FuzzyDedupCandidateTitlesForCompanyCappedParams{
		Company: p.CompanySlug,
		MaxRows: FuzzySyncMaxCandidates + 1,
	})
	if err != nil {
		return 0, false, fmt.Errorf("fuzzy candidates for %s: %w", p.CompanySlug, err)
	}
	if len(rows) > FuzzySyncMaxCandidates {
		return 0, false, nil
	}
	key := jobhash.RoleKey("", p.Title)
	size, older := 0, []int64{}
	for _, r := range rows {
		if jobhash.RoleKey("", r.Title) != key {
			continue
		}
		size++
		if r.ID <= id {
			older = append(older, r.ID)
		}
	}
	// size > FuzzyMaxBucket: not judged (BucketByRole's cap). len(older) < 2: nobody to join.
	if size > FuzzyMaxBucket || len(older) < 2 {
		return 0, false, nil
	}
	descriptions, err := q.GetJobDescriptionsByIDs(ctx, older)
	if err != nil {
		return 0, false, fmt.Errorf("fuzzy descriptions for %s: %w", p.CompanySlug, err)
	}
	postings := make([]FuzzyPosting, 0, len(descriptions))
	for _, d := range descriptions {
		postings = append(postings, FuzzyPosting{ID: d.ID, Signature: jobhash.DescriptionSignature(d.Description)})
	}
	canon, ok := ClusterBucket(postings, FuzzyThreshold)[id]
	return canon, ok, nil
}
