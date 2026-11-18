package handler

import (
	"context"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/platform/db"
	"golang.org/x/sync/singleflight"
)

// subindustryFacet is one option in the company subindustry vocabulary: a clean YC
// subindustry leaf and the number of companies carrying it.
type subindustryFacet struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
}

// CompanySubindustries serves the distinct subindustry vocabulary with company counts, most common first, for the
// "Industry" facet's options. Counts ignore other active list filters.
func (h *companiesHandlers) CompanySubindustries(ctx context.Context) (*apigen.CompanySubindustriesOK, error) {
	facets, err := h.subindustries.get(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]apigen.CompanySubindustry, len(facets))
	for i, f := range facets {
		out[i] = apigen.CompanySubindustry{Value: f.Value, Count: f.Count}
	}
	return &apigen.CompanySubindustriesOK{Data: out}, nil
}

// subindustryCacheTTL is how long one count of the vocabulary is served. The query is a full scan of companies, and uncached
// calls at a few req/s exhausted the connection pool. The vocabulary moves with ingest, so ten-minute-old counts are fine.
const subindustryCacheTTL = 10 * time.Minute

// subindustryCache keeps the vocabulary in process for subindustryCacheTTL. Concurrent misses share one query through
// flight. A failed query is not stored.
type subindustryCache struct {
	load func(context.Context) ([]db.CompanySubindustriesRow, error)
	// now is injectable so expiry is testable without sleeping.
	now func() time.Time

	flight  singleflight.Group
	mu      sync.Mutex
	facets  []subindustryFacet
	expires time.Time
}

// get returns the cached vocabulary, loading it when absent or expired. The slice is shared and must not be modified.
func (s *subindustryCache) get(ctx context.Context) ([]subindustryFacet, error) {
	if facets, ok := s.fresh(); ok {
		return facets, nil
	}
	v, err, _ := s.flight.Do("", func() (any, error) {
		// A flight that finished between our miss and this call has already stored it.
		if facets, ok := s.fresh(); ok {
			return facets, nil
		}
		rows, err := s.load(ctx)
		if err != nil {
			return nil, err
		}
		facets := make([]subindustryFacet, 0, len(rows))
		for _, r := range rows {
			facets = append(facets, subindustryFacet{Value: r.Value.String, Count: r.Count})
		}
		s.mu.Lock()
		s.facets, s.expires = facets, s.now().Add(subindustryCacheTTL)
		s.mu.Unlock()
		return facets, nil
	})
	if err != nil {
		return nil, err
	}
	return v.([]subindustryFacet), nil
}

func (s *subindustryCache) fresh() ([]subindustryFacet, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.facets, s.facets != nil && s.now().Before(s.expires)
}

// facetValues reads the repeatable values of one facet query param, dropping empty entries and always returning a
// non-nil slice (pgx encodes '{}', keeping the SQL cardinality() short-circuit true).
func facetValues(vals url.Values, key string) []string {
	out := make([]string, 0, len(vals[key]))
	for _, v := range vals[key] {
		// Comma-joined and repeated must mean the same thing, as in the Meilisearch path (search.CompanyFilterFromValues)
		// and the job search. Treating `countries=DE,BR` as ONE literal value answered zero companies instead of an error.
		for _, part := range strings.Split(v, ",") {
			if part != "" {
				out = append(out, part)
			}
		}
	}
	return out
}
