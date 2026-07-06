package handler

import (
	"context"
	"net/url"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/jobview"
	"github.com/strelov1/freehire/internal/search"
)

// searcher is the search backend the handler depends on. *search.Client
// satisfies it; tests inject a fake. A nil searcher means search is not
// configured (no MEILI_MASTER_KEY) and the endpoint reports 503.
type searcher interface {
	Search(ctx context.Context, p search.SearchParams) (search.SearchResult, error)
	SimilarJobs(ctx context.Context, id int64, limit int) ([]search.JobDocument, error)
	// EmbedText returns a vector for text in the jobs' embedding space plus the
	// embedder identity that produced it (used to embed a CV on upload).
	EmbedText(ctx context.Context, key, text string) ([]float64, string, error)
	// RecommendByVector ranks open jobs by similarity to a raw vector (the CV feed).
	RecommendByVector(ctx context.Context, vector []float64, limit, offset int) (search.SearchResult, error)
}

// defaultSemanticRatio is 0 — pure keyword search against the always-fresh facet
// index — because semantic search is opt-in: the embedder lives on a separate
// index built by an optional reindex --semantic pass, so a default of 0 never
// routes unprepared traffic to a stale or absent semantic index. A client opts in
// per request with semantic_ratio>0; the SPA already does so explicitly.
const defaultSemanticRatio = 0

// maxSearchWindow bounds how deep search pagination may reach (offset+limit). It
// is the explicit pagination guard, decoupled from the index's maxTotalHits
// (which now only sets how high the reported total may count): the total can read
// the true filtered count while deep offset paging — the expensive part — stays
// refused. ~500 pages at the default limit is far beyond any real browsing.
const maxSearchWindow = 10000

// searchSortable is the allowlist of sort params mapped to their index attribute;
// anything else is ignored so a bad param cannot make Meilisearch reject the query.
var searchSortable = map[string]string{
	"created_at": "created_at",
	"posted_at":  "posted_at",
	"salary_min": "enrichment.salary_min",
	"salary_max": "enrichment.salary_max",
}

// SearchJobs runs a full-text + hybrid search over the jobs index. It is public
// (unauthenticated) like the other job reads. Response: {"data": [job view...],
// "meta": {total, limit, offset}} — results carry public_slug and never the
// internal id.
func (a *API) SearchJobs(c *fiber.Ctx) error {
	if a.search == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "search is not available")
	}

	limit, offset := pageParams(c)
	if offset+limit > maxSearchWindow {
		return fiber.NewError(fiber.StatusBadRequest, "pagination too deep")
	}
	ratio := min(max(c.QueryFloat("semantic_ratio", defaultSemanticRatio), 0), 1)

	res, err := a.search.Search(c.Context(), search.SearchParams{
		Query:         c.Query("q"),
		Filter:        buildSearchFilter(c),
		Sort:          searchSort(c),
		Limit:         limit,
		Offset:        offset,
		SemanticRatio: ratio,
	})
	if err != nil {
		// RenderError renders a generic 500; returning the error keeps the
		// Meilisearch failure cause visible to logging instead of swallowing it.
		return err
	}

	views := make([]jobview.Job, len(res.Hits))
	for i, hit := range res.Hits {
		views[i] = hit.Job
	}

	return listResponse(c, views, res.Total, limit, offset)
}

// searchSort builds the Meilisearch sort directive from ?sort=<field>&order=<dir>.
// Without a valid sort param, a no-text browse defaults to the freshest postings
// first (posted_at desc) — relevance is meaningless for an empty query — while a
// text query keeps relevance order (nil).
func searchSort(c *fiber.Ctx) []string {
	attr, ok := searchSortable[c.Query("sort")]
	if !ok {
		if c.Query("q") == "" {
			return []string{"posted_at:desc"}
		}
		return nil
	}
	order := c.Query("order", "desc")
	if order != "asc" && order != "desc" {
		order = "desc"
	}
	return []string{attr + ":" + order}
}

// buildSearchFilter turns the request's facet query params into a Meilisearch
// filter by delegating to the shared, pure search.FilterFromValues — the same
// translation the notification matcher applies to a saved search's stored query,
// so the two cannot drift. Returns nil when no facet is set.
func buildSearchFilter(c *fiber.Ctx) any {
	vals, _ := url.ParseQuery(string(c.Request().URI().QueryString()))
	return search.FilterFromValues(vals)
}
