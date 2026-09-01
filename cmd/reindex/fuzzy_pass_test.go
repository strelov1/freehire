package main

import (
	"context"
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/platform/db"
)

// fakeFuzzyQuerier is the store slice collapseFuzzyDuplicatesForCompany needs, which is why
// fuzzyDedupQuerier is an interface rather than *db.Queries. It records the marker write so a
// test can assert what the pass DECIDED, not merely what it merged — the release path has no
// other observable output.
type fakeFuzzyQuerier struct {
	titles []db.FuzzyDedupCandidateTitlesForCompanyRow
	bodies map[int64]string
	got    db.MarkFuzzyDuplicatesForCompanyParams
	calls  int
}

func (f *fakeFuzzyQuerier) CompaniesWithFuzzyDedupCandidates(context.Context) ([]string, error) {
	return []string{"acme"}, nil
}

func (f *fakeFuzzyQuerier) FuzzyDedupCandidateTitlesForCompany(context.Context, string) ([]db.FuzzyDedupCandidateTitlesForCompanyRow, error) {
	return f.titles, nil
}

func (f *fakeFuzzyQuerier) GetJobDescriptionsByIDs(_ context.Context, ids []int64) ([]db.GetJobDescriptionsByIDsRow, error) {
	rows := make([]db.GetJobDescriptionsByIDsRow, 0, len(ids))
	for _, id := range ids {
		rows = append(rows, db.GetJobDescriptionsByIDsRow{ID: id, Description: f.bodies[id]})
	}
	return rows, nil
}

func (f *fakeFuzzyQuerier) MarkFuzzyDuplicatesForCompany(_ context.Context, arg db.MarkFuzzyDuplicatesForCompanyParams) (int64, error) {
	f.calls++
	f.got = arg
	return int64(len(arg.Ids)), nil
}

func titleRows(ids []int64, title string) []db.FuzzyDedupCandidateTitlesForCompanyRow {
	rows := make([]db.FuzzyDedupCandidateTitlesForCompanyRow, 0, len(ids))
	for _, id := range ids {
		rows = append(rows, db.FuzzyDedupCandidateTitlesForCompanyRow{ID: id, Title: title})
	}
	return rows
}

// Two postings of one role whose bodies are near-identical collapse, and BOTH are reported as
// considered — the canon included, since it is a candidate whose verdict happens to be "stay".
func TestCollapseFuzzyForCompany_ReportsEveryConsideredRowAlongsideTheMerge(t *testing.T) {
	f := &fakeFuzzyQuerier{
		titles: titleRows([]int64{1, 2}, "Senior Fullstack Engineer"),
		bodies: map[int64]string{
			1: bodyA,
			2: bodyA + " Salary in Poland is quoted gross per month under Polish labour law.",
		},
	}

	if _, err := collapseFuzzyDuplicatesForCompany(context.Background(), f, "acme"); err != nil {
		t.Fatalf("collapse: %v", err)
	}
	if got := sortedIDs(f.got.Candidates); !slices.Equal(got, []int64{1, 2}) {
		t.Errorf("candidates = %v, want both rows [1 2]", got)
	}
	if !slices.Equal(f.got.Ids, []int64{2}) || !slices.Equal(f.got.Canons, []int64{1}) {
		t.Errorf("assignment = ids %v canons %v, want the higher id onto the lower", f.got.Ids, f.got.Canons)
	}
}

// The end-to-end release path the spec names: two postings whose descriptions have DIVERGED
// below the threshold are still both considered, and neither is assigned — so whatever marker
// they carry is cleared by the write. Without this the release is only ever proven at the query
// level, and nothing checks that the pass actually offers the rows up to be released.
func TestCollapseFuzzyForCompany_DivergedPostingsAreConsideredAndUnassigned(t *testing.T) {
	f := &fakeFuzzyQuerier{
		titles: titleRows([]int64{1, 2}, "Senior Fullstack Engineer"),
		bodies: map[int64]string{1: bodyA, 2: bodyB},
	}

	if _, err := collapseFuzzyDuplicatesForCompany(context.Background(), f, "acme"); err != nil {
		t.Fatalf("collapse: %v", err)
	}
	if f.calls != 1 {
		t.Fatalf("marker write called %d times, want 1 — a run that merges nothing must still "+
			"report its verdict, or a stale marker is never released", f.calls)
	}
	if got := sortedIDs(f.got.Candidates); !slices.Equal(got, []int64{1, 2}) {
		t.Errorf("candidates = %v, want both rows [1 2]", got)
	}
	if len(f.got.Ids) != 0 {
		t.Errorf("assignment = %v, want none (the bodies diverged below the threshold)", f.got.Ids)
	}
}

// A bucket past the size cap is skipped on COST, so its rows must not be reported as considered:
// reporting them would release every marker in the largest groups in the catalogue over a
// compute decision. With nothing else to judge, the pass writes nothing at all.
func TestCollapseFuzzyForCompany_AnOversizedBucketIsNotReportedAsConsidered(t *testing.T) {
	ids := make([]int64, 0, fuzzyMaxBucket+1)
	bodies := map[int64]string{}
	for i := 0; i <= fuzzyMaxBucket; i++ {
		id := int64(100 + i)
		ids = append(ids, id)
		bodies[id] = bodyA
	}
	f := &fakeFuzzyQuerier{titles: titleRows(ids, "Customer Service Associate"), bodies: bodies}

	if _, err := collapseFuzzyDuplicatesForCompany(context.Background(), f, "acme"); err != nil {
		t.Fatalf("collapse: %v", err)
	}
	if f.calls != 0 {
		t.Errorf("marker write called with candidates %v — an oversized bucket is not judged and "+
			"must not be offered for release", f.got.Candidates)
	}
}
