package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/job/jobdedup"
	"github.com/strelov1/freehire/internal/job/jobhash"
	"github.com/strelov1/freehire/internal/platform/db"
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
	GetMarkerPassCursor(ctx context.Context, pass string) (string, error)
	SetMarkerPassCursor(ctx context.Context, arg db.SetMarkerPassCursorParams) error
}

// collapseFuzzyDuplicates marks near-identical reposts the exact passes left split, company by
// company with `workers` in flight, and returns the rows re-marked. It runs LAST, over what the exact
// role-cluster and aggregator passes did not claim, so it only ever adds markers to leftovers and
// never contradicts a deterministic collapse.
//
// It RESUMES. Measured 2026-10-28 the walk covers ~383k companies and ran out its 4h unit timeout
// 155k in; it walked the same order every run, so the rest of the list was never reached at all.
// The companies are walked in sorted order, the last one finished is saved as the walk goes, the
// next run takes only the companies after it, and a run that reaches the end clears it so the
// following run starts from the top. Every company is therefore reached within a bounded number of
// runs even when every run times out. A company that joins the list behind the saved point waits
// for the next cycle.
//
// Descriptions are loaded only for buckets that survive the size filter — titles are cheap to ship,
// descriptions are not, and the largest bucket on prod holds 16 745 postings.
func collapseFuzzyDuplicates(ctx context.Context, q fuzzyPassStore, workers int) (int64, error) {
	after, err := q.GetMarkerPassCursor(ctx, fuzzyPassName)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("read resume point: %w", err)
	}
	companies, err := q.CompaniesWithFuzzyDedupCandidates(ctx)
	if err != nil {
		return 0, err
	}
	// Sorted here rather than by the query: the comparison with the saved point is Go's, so the
	// order has to be Go's too — Postgres's collation would order the same slugs differently.
	slices.Sort(companies)
	if after != "" {
		all := len(companies)
		start, found := slices.BinarySearch(companies, after)
		if found {
			start++
		}
		companies = companies[start:]
		log.Printf("reindex: fuzzy pass resuming after %q: %d of %d companies left", after, len(companies), all)
	}

	// A failed save does not stop the walk — the next checkpoint may well land — but if the LAST
	// one failed it is reported with the pass's outcome: the stored point is then stale, the next
	// run goes back over ground already covered, and marker-only mode must exit non-zero on it.
	var saveErr error
	save := func(point string) {
		// The run's own context is dead by the time the final save matters most — it is what
		// the unit's SIGTERM cancelled — so the write gets a short one of its own.
		sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		saveErr = q.SetMarkerPassCursor(sctx, db.SetMarkerPassCursorParams{Pass: fuzzyPassName, AfterCompany: point})
		if saveErr != nil {
			saveErr = fmt.Errorf("save resume point %q: %w", point, saveErr)
			log.Printf("reindex: fuzzy pass: %v", saveErr) // now, not hours later: a killed run never returns it
		}
	}
	total, err := forEachCompany(ctx, companies, workers, func(ctx context.Context, company string) (int64, error) {
		return collapseFuzzyDuplicatesForCompany(ctx, q, company)
	}, save)
	if ctx.Err() == nil {
		save("") // the walk reached the end: the next run starts from the top
	}
	return total, errors.Join(err, saveErr)
}

func collapseFuzzyDuplicatesForCompany(ctx context.Context, q fuzzyDedupQuerier, company string) (int64, error) {
	titles, err := q.FuzzyDedupCandidateTitlesForCompany(ctx, company)
	if err != nil {
		return 0, err
	}
	judged, comparable := jobdedup.BucketByRole(titles)
	if len(judged) == 0 {
		return 0, nil
	}

	// Descriptions are loaded only for the buckets worth COMPARING. The rest of `judged` is
	// rows that cannot cluster (a bucket of one, a title that normalizes to nothing), and the
	// verdict on those is "no cluster" without reading a byte of body.
	var ids []int64
	for _, bucketIDs := range comparable {
		ids = append(ids, bucketIDs...)
	}
	signatures := make(map[int64]map[string]struct{}, len(ids))
	if len(ids) > 0 {
		descriptions, err := q.GetJobDescriptionsByIDs(ctx, ids)
		if err != nil {
			return 0, err
		}
		for _, row := range descriptions {
			signatures[row.ID] = jobhash.DescriptionSignature(row.Description)
		}
	}

	var moveIDs, canons []int64
	for _, bucketIDs := range comparable {
		postings := make([]jobdedup.FuzzyPosting, 0, len(bucketIDs))
		for _, id := range bucketIDs {
			postings = append(postings, jobdedup.FuzzyPosting{ID: id, Signature: signatures[id]})
		}
		for id, canon := range jobdedup.ClusterBucket(postings, jobdedup.FuzzyThreshold) {
			moveIDs = append(moveIDs, id)
			canons = append(canons, canon)
		}
	}
	// Sent even when nothing clustered: `judged` carries the RELEASES. A run that collapses
	// nothing still has to clear the markers whose clusters have since dissolved, which is the
	// only thing that ever frees a row this pass hid.
	return q.MarkFuzzyDuplicatesForCompany(ctx, db.MarkFuzzyDuplicatesForCompanyParams{
		Candidates: judged,
		Ids:        moveIDs,
		Canons:     canons,
		Company:    company,
	})
}
