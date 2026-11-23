package handler

import (
	"context"
	"net/url"
)

// newHiringParam narrows /jobs/search to companies that just started hiring: the ones
// rollup-company marked with insights_company_growth.hiring_started_at. Like the watchlist
// it is a set of company slugs, not a facet of the posting, so the handler resolves it and
// declares it as its own only when it did (see jobSearchOutcome.own). Unlike the watchlist
// it is the same for every caller, so the answer stays cacheable.
const newHiringParam = "new_hiring"

// newHiringLookup reads the companies that just started hiring. *db.Queries satisfies it.
type newHiringLookup interface {
	ListNewHiringCompanySlugs(ctx context.Context) ([]string, error)
}

// newHiringSlugsFor resolves `new_hiring=true`. applied is false when the param is absent
// or false, or when nothing can read it; applied with no slugs (before the first rollup)
// narrows to nothing.
func (h *searchHandlers) newHiringSlugsFor(ctx context.Context, vals url.Values) (slugs []string, applied bool, err error) {
	if h.newHiring == nil || !wantsParam(vals, newHiringParam) {
		return nil, false, nil
	}
	slugs, err = h.newHiring.ListNewHiringCompanySlugs(ctx)
	if err != nil {
		return nil, false, err
	}
	return slugs, true, nil
}
