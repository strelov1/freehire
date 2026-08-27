package handler

import (
	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/search/search"
)

// SwipeDeck returns a batch of open jobs for the swipe triage deck. It runs the
// same Meilisearch query as SearchJobs (same facets, free-text q, and sort) so
// the deck's ranking matches the list, then excludes the caller's already-judged
// jobs (saved or dismissed) via a server-built `id NOT IN [...]` filter. It is
// authenticated (both swipe actions are per-user); the response is the standard
// list envelope of job views, batched via limit/offset for prefetch.
func (h *trackingHandlers) SwipeDeck(c *fiber.Ctx) error {
	if h.search == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "search is not available")
	}
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}

	limit, offset := pageParams(c)
	if offset+limit > maxSearchWindow {
		return fiber.NewError(fiber.StatusBadRequest, "pagination too deep")
	}

	excluded, err := h.tracking.ExcludedJobIDs(c.Context(), userID)
	if err != nil {
		return err
	}

	res, err := h.search.Search(c.Context(), search.SearchParams{
		Query:  c.Query("q"),
		Filter: withDeckExclusion(buildSearchFilter(c), excluded),
		// false: the deck sends no match vector. It is an obvious candidate for the
		// match sort — it is already authenticated — but ordering the deck is its own
		// product decision, so it keeps attribute sorting until that is made.
		Sort:   searchSort(c, false),
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		return err
	}

	views := make([]jobview.Job, len(res.Hits))
	for i, hit := range res.Hits {
		views[i] = hit.Job
	}
	return listResponse(c, views, res.Total, limit, offset)
}

// withDeckExclusion adds an `id NOT IN [...]` group to the facet filter so the
// deck skips the caller's judged jobs. base is buildSearchFilter's result —
// either a [][]string of facet groups or nil. An empty exclusion set leaves the
// filter untouched.
func withDeckExclusion(base any, excluded []int64) any {
	frag := search.NotIn("id", excluded)
	if frag == "" {
		return base
	}
	groups, _ := base.([][]string) // nil base → nil groups; append allocates
	return append(groups, []string{frag})
}
