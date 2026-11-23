package db

import (
	"context"
	"encoding/json"
)

// refreshCompanyFacetsParams is cmd/recount-companies' input: the unseen windows go to the
// read half, the domain→industry mapping to the write half.
type refreshCompanyFacetsParams struct {
	MappingDomains    []string
	MappingIndustries []string
	UnseenSources     []string
	UnseenSeconds     []int64
}

// refreshCompanyFacets runs cmd/recount-companies' two statements against one database, as
// it does when no analytics replica is configured.
func (q *Queries) refreshCompanyFacets(ctx context.Context, p refreshCompanyFacetsParams) (int64, error) {
	rows, err := q.ComputeCompanyJobFacets(ctx, ComputeCompanyJobFacetsParams{UnseenSources: p.UnseenSources, UnseenSeconds: p.UnseenSeconds})
	if err != nil {
		return 0, err
	}
	facets, err := json.Marshal(rows)
	if err != nil {
		return 0, err
	}
	return q.ApplyCompanyFacets(ctx, ApplyCompanyFacetsParams{Facets: facets, MappingDomains: p.MappingDomains, MappingIndustries: p.MappingIndustries})
}
