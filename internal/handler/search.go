package handler

import (
	"time"

	"context"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/jobview"
	"github.com/strelov1/freehire/internal/search"
)

// searchHandlers serves the Meilisearch-backed job search surfaces: the keyword
// search, the agent search (always full descriptions in a selectable format), the
// similar-jobs read, and the facet distribution. search/facets are two narrow
// views of the same client, kept separate so the concerns stay decoupled; both
// nil when Meilisearch is unconfigured (the endpoints then report 503).
type searchHandlers struct {
	search searcher
	// descriptions loads full job descriptions by internal id, to rehydrate the
	// truncated search preview on the agent search endpoint. *db.Queries in
	// production; a fake in tests.
	descriptions jobDescriptions
	queries      *db.Queries
	facets       facetCounter
}

func newSearchHandlers(search searcher, facets facetCounter, queries *db.Queries) *searchHandlers {
	return &searchHandlers{search: search, descriptions: queries, queries: queries, facets: facets}
}

func (h *searchHandlers) register(api fiber.Router, mw middleware) {
	// Literal routes are registered before the /jobs/:slug param route (see
	// Register) so they are not read as slugs.
	api.Get("/jobs/search", h.SearchJobs)
	// Agent search: same query as /jobs/search, but always full descriptions in a
	// selectable format for programmatic consumers. Public, like the other reads.
	api.Get("/agent/jobs/search", h.AgentSearchJobs)
	api.Get("/jobs/facets", h.JobFacets)
	api.Get("/jobs/:slug/similar", h.SimilarJobs)
}

// searcher is the search backend the handler depends on. *search.Client
// satisfies it; tests inject a fake. A nil searcher means search is not
// configured (no MEILI_MASTER_KEY) and the endpoint reports 503.
type searcher interface {
	Search(ctx context.Context, p search.SearchParams) (search.SearchResult, error)
	SimilarJobs(ctx context.Context, id int64, limit int) ([]search.JobDocument, error)
	// EmbedText returns a vector for text in the jobs' embedding space plus the
	// embedder identity that produced it (used to embed a CV on upload).
	EmbedText(ctx context.Context, text string) ([]float64, string, error)
	// RecommendByVector ranks open jobs by similarity to a raw vector (the CV feed),
	// constrained to an optional facet filter (nil for none).
	RecommendByVector(ctx context.Context, vector []float64, filter any, limit, offset int) (search.SearchResult, error)
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
func (h *searchHandlers) SearchJobs(c *fiber.Ctx) error {
	res, limit, offset, err := h.runJobSearch(c)
	if err != nil {
		return err
	}

	views := make([]jobview.Job, len(res.Hits))
	for i, hit := range res.Hits {
		views[i] = hit.Job
	}
	h.attachGhost(c, res.Hits, views)

	return listResponse(c, views, res.Total, limit, offset)
}

// runJobSearch performs the request handling shared by both job-search endpoints:
// the availability check, the pagination-window guard, the semantic ratio, and the
// index query. It is the single place the query is built, so the public and agent
// search endpoints cannot drift. The availability and deep-pagination guards return
// a fiber *Error the caller can return directly; on success it returns the raw hits
// and the applied limit/offset.
func (h *searchHandlers) runJobSearch(c *fiber.Ctx) (search.SearchResult, int, int, error) {
	if h.search == nil {
		return search.SearchResult{}, 0, 0, fiber.NewError(fiber.StatusServiceUnavailable, "search is not available")
	}

	limit, offset := pageParams(c)
	if offset+limit > maxSearchWindow {
		return search.SearchResult{}, 0, 0, fiber.NewError(fiber.StatusBadRequest, "pagination too deep")
	}
	ratio := min(max(c.QueryFloat("semantic_ratio", defaultSemanticRatio), 0), 1)

	res, err := h.search.Search(c.Context(), search.SearchParams{
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
		return search.SearchResult{}, 0, 0, err
	}

	return res, limit, offset, nil
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
	return search.FilterFromValues(queryValues(c))
}

// attachGhost attaches the ghost signal to a page of search hits.
//
// Search is the surface where job cards are actually browsed, so leaving it out
// would mean the badge existed almost nowhere. It costs two extra reads per page:
// the outcome evidence (sparse — only jobs anyone reported or applied to) and the
// absence stamps.
//
// The stamps have to come from Postgres because they cannot be in the index:
// cmd/reindex pushes only documents whose content_hash moved, and no adapter writes
// ats_absent_at, so the column would never reach Meilisearch on its own. That is the
// same trap is_tech already fell into. The reality class, by contrast, IS on the
// document (search.FromJob promotes it), so it is read from the hit rather than
// recomputed.
//
// Best-effort: a failed lookup leaves the signal off the page rather than failing
// the search.
func (h *searchHandlers) attachGhost(c *fiber.Ctx, hits []search.JobDocument, views []jobview.Job) {
	if len(hits) == 0 || h.queries == nil {
		return
	}
	ids := make([]int64, len(hits))
	for i, hit := range hits {
		ids[i] = hit.ID
	}

	stampRows, err := h.queries.ListJobGhostStamps(c.Context(), ids)
	if err != nil {
		return
	}
	stamps := make(map[int64]db.ListJobGhostStampsRow, len(stampRows))
	for _, r := range stampRows {
		stamps[r.ID] = r
	}
	evidence := ghostEvidenceFor(c.Context(), h.queries, ids)

	now := time.Now()
	for i, hit := range hits {
		realityClass := ""
		if hit.Reality != nil {
			realityClass = hit.Reality.Class
		}
		row := stamps[hit.ID]
		views[i].Ghost = jobview.ClassifyGhost(jobview.GhostInput{
			Now: now,
			// Read from Postgres, not from the hit: a sweep-closed job stays in the
			// index until a reindex, whose timer is disabled, so the index cannot be
			// trusted to say a posting is still up. Without this a warning would
			// appear on postings already taken down.
			Closed:       row.ClosedAt.Valid,
			RealityClass: realityClass,
			ATSAbsentAt:  row.AtsAbsentAt.Time,
			HasATSAbsent: row.AtsAbsentAt.Valid,
			Evidence:     evidence[hit.ID],
		})
	}
}
