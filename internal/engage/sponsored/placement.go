package sponsored

import (
	"context"
	"time"

	"github.com/strelov1/freehire/internal/search/search"
)

// Searcher is the job search the placements ask. *search.Client satisfies it.
type Searcher interface {
	Search(ctx context.Context, p search.SearchParams) (search.SearchResult, error)
}

// Pick is a sponsored hit with the campaign that pays for it.
type Pick struct {
	Doc        search.JobDocument
	CampaignID int64
}

// Find runs the sponsored twin of a search: the same query and filter ANDed with
// sponsored = true, the highest cpc first, at most limit hits from the first page. Each hit
// is resolved to its serving campaign in Postgres; a hit the database no longer serves (the
// index lags) is dropped. Callers treat an error as "no sponsored slot", never as a failure.
func Find(ctx context.Context, s Searcher, r search.ServingReader, p search.SearchParams, limit int, now time.Time) ([]Pick, error) {
	p.Filter = search.AndFragment(p.Filter, "sponsored = true")
	// A vector search forbids an explicit sort, and the paid order is by cpc.
	p.Vector, p.Sort, p.Limit, p.Offset = nil, []string{"sponsored_cpc:desc"}, limit, 0
	res, err := s.Search(ctx, p)
	if err != nil || len(res.Hits) == 0 {
		return nil, err
	}
	ids := make([]int64, len(res.Hits))
	for i, h := range res.Hits {
		ids[i] = h.ID
	}
	serving, err := search.ServingCampaigns(ctx, r, ids, now)
	if err != nil {
		return nil, err
	}
	var picks []Pick
	for _, h := range res.Hits {
		if row, ok := serving[h.ID]; ok {
			picks = append(picks, Pick{Doc: h, CampaignID: row.CampaignID})
		}
	}
	return picks, nil
}
