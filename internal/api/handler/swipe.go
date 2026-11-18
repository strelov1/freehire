package handler

import (
	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/search/search"
)

// SwipeDeck returns a batch of open jobs for the swipe triage deck. It runs the same
// Meilisearch query as SearchJobs so the ranking matches the list, then excludes the caller's
// already-judged (saved or dismissed) jobs via an `id NOT IN [...]` filter. Authenticated;
// batched via limit/offset.
//
// It reports ignored params like /jobs/search: a deck silently widened to the whole
// catalogue looks like a deck that was narrowed and found a lot.
func (h *trackingHandlers) SwipeDeck(c *fiber.Ctx) error {
	if h.search == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "search is not available")
	}
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}

	limit, offset, err := pageParams(c)
	if err != nil {
		return err
	}

	excluded, err := h.tracking.ExcludedJobIDs(c.Context(), userID)
	if err != nil {
		return err
	}

	res, err := h.search.Search(c.Context(), search.SearchParams{
		Query:  c.Query("q"),
		Filter: withExcludedIDs(buildSearchFilter(queryValues(c)), excluded),
		// false: the deck sends no match vector and keeps attribute sorting (match-sorting it is a product decision).
		Sort:   searchSortValues(queryValues(c), false),
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
	return listResponseWithIgnored(c, views, res.Total, limit, offset, ignoredParams(queryValues(c), searchParams), ignoredJobValues(queryValues(c)))
}

// withExcludedIDs adds an `id NOT IN [...]` group to the facet filter so the search skips
// already-seen jobs (the swipe deck, and /jobs/search's hide_seen). base is
// buildSearchFilter's result, [][]string or nil. An empty set leaves the filter untouched.
func withExcludedIDs(base any, excluded []int64) any {
	return withFilterGroup(base, search.NotIn("id", excluded))
}
