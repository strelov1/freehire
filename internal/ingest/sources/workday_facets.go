package sources

import (
	"context"
	"fmt"
	"sync"

	"golang.org/x/sync/errgroup"
)

// workdayFacetValue is one value of a listing facet, e.g. a single job family within the
// "jobFamilyGroup" dimension. Count is the platform's own true count for that value, which is
// what makes it safe to plan a subdivision from a response already being parsed — no extra
// probe request needed.
type workdayFacetValue struct {
	ID    string `json:"id"`
	Count int    `json:"count"`
	// Parameter and Values are set instead of ID and Count when the value is itself a facet:
	// Workday nests its location dimensions (locationCountry, locationRegionStateProvince,
	// locations, ...) inside a "locationMainGroup" wrapper whose values are facets, not values.
	Parameter string              `json:"facetParameter"`
	Values    []workdayFacetValue `json:"values"`
}

// workdayFacet is one filterable dimension a listing response offers, e.g. "jobFamilyGroup" or
// "workerSubType". Applying a value from it (via appliedFacets) scopes the *other* dimensions'
// counts in the response, not its own, so subdividing a still-capped slice further means
// picking a dimension not yet applied in that branch.
type workdayFacet struct {
	Parameter string              `json:"facetParameter"`
	Values    []workdayFacetValue `json:"values"`
}

// splitByFacet subdivides a capped slice by the not-yet-used facet dimension that best splits
// it, recursing into each value's own slice (which may itself still be capped, in which case it
// recurses again with a different dimension) up to maxFacetDepth combined dimensions. Results
// are merged and deduped by ExternalPath, since a multi-counting dimension (e.g. workerSubType)
// can place the same posting in more than one slice.
//
// Slices are listed concurrently; inflight is one semaphore shared by the whole split, so the
// board never has more than workdaySliceParallelism requests open however deep it recurses. It is
// held only around a slice's own requests, never across a recursion, so a level waiting on its
// children cannot starve them.
func (s workday) splitByFacet(ctx context.Context, url string, b workdayBoard, appliedFacets map[string]any, capped workdayListPage, used map[string]bool, depth int, inflight chan struct{}) ([]workdayPosting, error) {
	dim := pickSplitDimension(capped.Facets, used)
	if dim == nil {
		// Paging the capped response as-is would return fewer postings than the board has and
		// report success, indistinguishable from the board shrinking. Fail loudly instead.
		return nil, fmt.Errorf("workday: board %s/%s capped at %d with no further facet dimension to split on",
			b.tenant, b.site, workdayCapTotal)
	}
	if depth >= maxFacetDepth {
		// Same as the no-dimension case: a still-capped slice this deep cannot be proven
		// complete, so fail rather than return the depth limit's leaf slice as the whole board.
		return nil, fmt.Errorf("workday: board %s/%s still capped at %d after splitting %d facet dimensions deep",
			b.tenant, b.site, workdayCapTotal, maxFacetDepth)
	}

	nextUsed := make(map[string]bool, len(used)+1)
	for k := range used {
		nextUsed[k] = true
	}
	nextUsed[dim.Parameter] = true

	var mu sync.Mutex
	byExternalPath := map[string]workdayPosting{}
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(workdaySliceParallelism)
	for _, v := range dim.Values {
		sliceFacets := cloneAppliedFacets(appliedFacets)
		sliceFacets[dim.Parameter] = []string{v.ID}
		g.Go(func() error {
			inflight <- struct{}{}
			page, err := s.fetchListPage(gctx, url, sliceFacets, 0)
			<-inflight
			if err != nil {
				return err
			}
			var slice []workdayPosting
			if page.Total == workdayCapTotal {
				slice, err = s.splitByFacet(gctx, url, b, sliceFacets, page, nextUsed, depth+1, inflight)
			} else {
				inflight <- struct{}{}
				slice, err = s.pageAll(gctx, url, sliceFacets, page)
				<-inflight
			}
			if err != nil {
				return err
			}
			mu.Lock()
			for _, p := range slice {
				byExternalPath[p.ExternalPath] = p
			}
			mu.Unlock()
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	postings := make([]workdayPosting, 0, len(byExternalPath))
	for _, p := range byExternalPath {
		postings = append(postings, p)
	}
	return postings, nil
}

// splittableFacets flattens Workday's facet tree into the dimensions a query can actually be
// scoped by. A wrapper whose values are themselves facets ("locationMainGroup" holding
// locationCountry, locationRegionStateProvince and locations) is replaced by those nested facets,
// and a value without an id is dropped, since it cannot be applied. The wrapper itself, read as
// a dimension of zero-count values, would look like the best split and be applied with an
// empty id.
func splittableFacets(facets []workdayFacet) []workdayFacet {
	var out []workdayFacet
	var walk func(f workdayFacet)
	walk = func(f workdayFacet) {
		applicable := workdayFacet{Parameter: f.Parameter}
		for _, v := range f.Values {
			if v.Parameter != "" {
				walk(workdayFacet{Parameter: v.Parameter, Values: v.Values})
				continue
			}
			if v.ID != "" {
				applicable.Values = append(applicable.Values, workdayFacetValue{ID: v.ID, Count: v.Count})
			}
		}
		if len(applicable.Values) > 0 {
			out = append(out, applicable)
		}
	}
	for _, f := range facets {
		walk(f)
	}
	return out
}

// pickSplitDimension returns the facet dimension (not already in used) whose largest single
// value is smallest, so the recursion most likely clears the cap in one more level instead of
// hitting maxFacetDepth stuck on one enormous value. (Picking the dimension with the biggest
// single value anywhere makes no progress.) Returns nil if every dimension is already applied.
func pickSplitDimension(facets []workdayFacet, used map[string]bool) *workdayFacet {
	candidates := splittableFacets(facets)
	var best *workdayFacet
	bestMax := -1
	for i := range candidates {
		f := &candidates[i]
		if used[f.Parameter] || len(f.Values) == 0 {
			continue
		}
		max, sum := 0, 0
		for _, v := range f.Values {
			sum += v.Count
			if v.Count > max {
				max = v.Count
			}
		}
		// A capped slice holds at least workdayCapTotal postings, so a dimension whose values add
		// up to fewer cannot partition it (e.g. a "distance" facet summing to 5).
		if sum < workdayCapTotal {
			continue
		}
		if best == nil || max < bestMax {
			bestMax = max
			best = f
		}
	}
	return best
}

// cloneAppliedFacets copies an appliedFacets map so a recursion branch can add its own facet
// value without mutating the map a sibling branch is still using.
func cloneAppliedFacets(appliedFacets map[string]any) map[string]any {
	out := make(map[string]any, len(appliedFacets)+1)
	for k, v := range appliedFacets {
		out[k] = v
	}
	return out
}
