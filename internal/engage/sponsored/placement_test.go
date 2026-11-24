package sponsored

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/search"
)

type fakeSearcher struct {
	got search.SearchParams
	res search.SearchResult
	err error
}

func (f *fakeSearcher) Search(_ context.Context, p search.SearchParams) (search.SearchResult, error) {
	f.got = p
	return f.res, f.err
}

type fakeServing struct {
	rows []db.ListServingSponsoredCampaignsRow
}

func (f fakeServing) ListServingSponsoredCampaigns(context.Context, db.ListServingSponsoredCampaignsParams) ([]db.ListServingSponsoredCampaignsRow, error) {
	return f.rows, nil
}

func TestFindAsksForSponsoredAndResolvesCampaigns(t *testing.T) {
	s := &fakeSearcher{res: search.SearchResult{Hits: []search.JobDocument{{ID: 1}, {ID: 2}, {ID: 3}}}}
	// Job 2 stopped serving since the index was written (stale document): dropped.
	r := fakeServing{rows: []db.ListServingSponsoredCampaignsRow{{JobID: 1, CampaignID: 10}, {JobID: 3, CampaignID: 30}}}
	in := search.SearchParams{Query: "go", Filter: [][]string{{"skills = go"}}, Sort: []string{"posted_at:desc"}, Vector: []float32{1}, Limit: 20, Offset: 40}

	picks, err := Find(context.Background(), s, r, in, 2, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(picks) != 2 || picks[0].CampaignID != 10 || picks[0].Doc.ID != 1 || picks[1].CampaignID != 30 {
		t.Fatalf("picks = %+v", picks)
	}
	g := s.got
	groups, _ := g.Filter.([][]string)
	if g.Query != "go" || len(groups) != 2 || groups[1][0] != "sponsored = true" ||
		len(g.Sort) != 1 || g.Sort[0] != "sponsored_cpc:desc" || g.Limit != 2 || g.Offset != 0 || g.Vector != nil {
		t.Errorf("params = %+v", g)
	}
	if in.Offset != 40 || len(in.Filter.([][]string)) != 1 {
		t.Errorf("the caller's params were mutated: %+v", in)
	}
}

func TestFindPropagatesSearchError(t *testing.T) {
	boom := errors.New("meili down")
	if _, err := Find(context.Background(), &fakeSearcher{err: boom}, fakeServing{}, search.SearchParams{}, 1, time.Now()); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
}
