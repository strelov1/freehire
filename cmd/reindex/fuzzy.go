package main

import (
	"context"
	"errors"
	"slices"

	"github.com/strelov1/freehire/internal/job/jobdedup"
	"github.com/strelov1/freehire/internal/job/jobhash"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/htmltext"
)

// fuzzyDedupQuerier is the slice of the store this pass needs, named so the pass can be read
// (and faked) without the full generated querier.
type fuzzyDedupQuerier interface {
	CompaniesWithFuzzyDedupCandidates(ctx context.Context) ([]string, error)
	FuzzyDedupCandidateTitlesForCompany(ctx context.Context, company string) ([]db.FuzzyDedupCandidateTitlesForCompanyRow, error)
	GetJobDescriptionsByIDs(ctx context.Context, ids []int64) ([]db.GetJobDescriptionsByIDsRow, error)
	MarkFuzzyDuplicatesForCompany(ctx context.Context, arg db.MarkFuzzyDuplicatesForCompanyParams) (int64, error)
}

// fuzzyPassName keys this pass's resume point in marker_pass_cursors.
const fuzzyPassName = "fuzzy"

// fuzzyPassStore is fuzzyDedupQuerier plus the pass's resume point.
type fuzzyPassStore interface {
	fuzzyDedupQuerier
	cursorStore
}

// collapseFuzzyDuplicates marks near-identical reposts the exact passes left split, company by
// company with `workers` in flight, and returns the rows re-marked. It runs LAST, over what the exact
// role-cluster and aggregator passes did not claim, so it only ever adds markers to leftovers and
// never contradicts a deterministic collapse.
//
// It RESUMES: companies are walked in sorted order and the last finished one is saved as the walk goes,
// so the next run takes only those after it and a run that reaches the end clears it. Every company is
// reached within a bounded number of runs even when each run times out. A company joining behind the saved point waits for the next cycle.
//
// Descriptions are loaded only for buckets that survive the size filter: titles are cheap to ship, descriptions are not.
func collapseFuzzyDuplicates(ctx context.Context, q fuzzyPassStore, workers int) (int64, error) {
	cursor := &passCursor{store: q, pass: fuzzyPassName}
	companies, err := q.CompaniesWithFuzzyDedupCandidates(ctx)
	if err != nil {
		return 0, err
	}
	if companies, err = cursor.resume(ctx, companies); err != nil {
		return 0, err
	}
	total, err := forEachCompany(ctx, companies, workers, func(ctx context.Context, company string) (int64, error) {
		return collapseFuzzyDuplicatesForCompany(ctx, q, company)
	}, func(point string) { cursor.save(ctx, point) })
	return total, errors.Join(err, cursor.finish(ctx))
}

func collapseFuzzyDuplicatesForCompany(ctx context.Context, q fuzzyDedupQuerier, company string) (int64, error) {
	titles, err := q.FuzzyDedupCandidateTitlesForCompany(ctx, company)
	if err != nil {
		return 0, err
	}
	judged, comparable := jobdedup.BucketByRole(titles)
	judgeSet := make(map[int64]bool, len(judged))
	for _, id := range judged {
		judgeSet[id] = true
	}
	summaryGroups, skipped := summaryBuckets(titles, judgeSet)
	judged = slices.DeleteFunc(judged, func(id int64) bool { return skipped[id] })
	slices.Sort(judged)
	if len(judged) == 0 {
		return 0, nil
	}

	// Descriptions are loaded only for buckets worth COMPARING; the rest of `judged` cannot cluster
	// (a bucket of one, a title normalizing to nothing) and its verdict is "no cluster".
	var ids []int64
	for _, bucketIDs := range comparable {
		for _, id := range bucketIDs {
			if !skipped[id] {
				ids = append(ids, id)
			}
		}
	}
	summaryIDs := make(map[int64]bool)
	for _, group := range summaryGroups {
		for _, row := range group {
			ids = append(ids, row.ID)
			summaryIDs[row.ID] = true
		}
	}
	slices.Sort(ids)
	ids = slices.Compact(ids)
	signatures := make(map[int64]map[string]struct{}, len(ids))
	evidence := make(map[int64]map[string]struct{})
	if len(ids) > 0 {
		descriptions, err := q.GetJobDescriptionsByIDs(ctx, ids)
		if err != nil {
			return 0, err
		}
		for _, row := range descriptions {
			signatures[row.ID] = jobhash.DescriptionSignature(row.Description)
			if summaryIDs[row.ID] {
				evidence[row.ID] = titleWords(htmltext.ToTextFull(row.Description))
			}
		}
	}

	moves := make(map[int64]int64)
	for _, bucketIDs := range comparable {
		postings := make([]jobdedup.FuzzyPosting, 0, len(bucketIDs))
		for _, id := range bucketIDs {
			if !skipped[id] {
				postings = append(postings, jobdedup.FuzzyPosting{ID: id, Signature: signatures[id]})
			}
		}
		for id, canon := range jobdedup.ClusterBucket(postings, jobdedup.FuzzyThreshold) {
			moves[id] = canon
		}
	}
	for _, group := range summaryGroups {
		collapseSummaries(
			group,
			signatures,
			evidence,
			moves,
		)
	}
	var moveIDs, canons []int64
	for id := range moves {
		moveIDs = append(moveIDs, id)
	}
	slices.Sort(moveIDs)
	for _, id := range moveIDs {
		canon := moves[id]
		for moves[canon] != 0 {
			canon = moves[canon]
		}
		canons = append(canons, canon)
	}
	// Sent even when nothing clustered: `judged` carries the RELEASES that clear markers of dissolved clusters.
	return q.MarkFuzzyDuplicatesForCompany(ctx, db.MarkFuzzyDuplicatesForCompanyParams{
		Candidates: judged,
		Ids:        moveIDs,
		Canons:     canons,
		Company:    company,
	})
}
