package handler

import (
	"context"
	"slices"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/dict/skillvec"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/identity/userprofile"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/cache"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/search"
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
	// cache holds facet distributions for facetCacheTTL. Best-effort and optional:
	// a nil cache (search configured but no Redis) simply recomputes every request,
	// the shape every facet test but the caching ones runs under.
	cache cache.Cache
	// userProfile reads the caller's profile skills for the match sort. Nil disables
	// that sort entirely, which is exactly how it degrades everywhere else.
	userProfile profileReader
	// facetStats reads the skill-rarity snapshot the match sort weights by.
	// *db.Queries in production; a fake in tests.
	facetStats facetStatsReader
}

// profileReader loads a caller's profile. *userprofile.Service satisfies it; tests
// inject a fake. Only the skills are read here — the match sort has no opinion on
// location preferences or specializations.
type profileReader interface {
	Get(ctx context.Context, userID int64) (userprofile.Profile, error)
}

// facetStatsReader reads the facet-distribution snapshot backing the skill-rarity
// weights. It mirrors search.FacetStatsReader rather than importing it as the
// handler's dependency, so the fake a test injects stays local to this package.
type facetStatsReader interface {
	ListFacetStats(ctx context.Context) ([]db.InsightsFacetStat, error)
}

func newSearchHandlers(search searcher, facets facetCounter, queries *db.Queries, c cache.Cache, profiles profileReader) *searchHandlers {
	return &searchHandlers{
		search: search, descriptions: queries, queries: queries, facets: facets, cache: c,
		userProfile: profiles, facetStats: queries,
	}
}

func (h *searchHandlers) register(api fiber.Router, mw middleware) {
	// Literal routes are registered before the /jobs/:slug param route (see
	// Register) so they are not read as slugs.
	readLimit := publicReadLimiter(mw.throttler)
	api.Get("/jobs/search", readLimit, mw.optional, h.SearchJobs)
	// Agent search: same query as /jobs/search, but always full descriptions in a
	// selectable format for programmatic consumers. Public, like the other reads.
	api.Get("/agent/jobs/search", agentSearchLimiter(mw.throttler), h.AgentSearchJobs)
	api.Get("/jobs/facets", readLimit, h.JobFacets)
	api.Get("/jobs/:slug/similar", readLimit, h.SimilarJobs)
}

// searcher is the search backend the handler depends on. *search.Client
// satisfies it; tests inject a fake. A nil searcher means search is not
// configured (no MEILI_MASTER_KEY) and the endpoint reports 503. SimilarJobs and
// RecommendByVector were dropped from this interface when the Meili-backed
// jobs_semantic index they queried was removed — /similar now reads a precomputed
// pgvector lookup (see internal/api/handler/similar.go) and /me/recommendations was
// removed outright (see openspec/changes/drop-hybrid-search-pgvector-similar).
type searcher interface {
	Search(ctx context.Context, p search.SearchParams) (search.SearchResult, error)
}

// maxSearchWindow bounds how deep search pagination may reach (offset+limit). It
// is the explicit pagination guard, decoupled from the index's maxTotalHits
// (which now only sets how high the reported total may count): the total can read
// the true filtered count while deep offset paging — the expensive part — stays
// refused. ~500 pages at the default limit is far beyond any real browsing.
const maxSearchWindow = 10000

// searchParams are the query params the search endpoints read themselves rather
// than hand to the filter: the query text, the sort directive and the pagination
// window. search.UnknownParams owns the filter vocabulary and nothing else, so
// each endpoint declares its own transport params beside itself (see
// facetsParams in facets.go, companiesParams in companies.go).
var searchParams = []string{"q", "sort", "order", "limit", "offset"}

// agentSearchParams is searchParams plus the agent endpoint's response-format
// selector.
var agentSearchParams = slices.Concat(searchParams, []string{"description_format"})

// ignoredParams reports the query params of this request that neither the filter
// nor the endpoint itself reads. They are echoed in the response meta instead of
// being refused: rejecting them would break saved searches and shared links that
// still carry retired params, while staying silent is what let a mistyped
// `country=it` pass for a search of Italy.
func ignoredParams(c *fiber.Ctx, own []string) []search.UnknownParam {
	return search.UnknownParams(queryValues(c), own)
}

// searchSortable is the allowlist of sort params mapped to their index attribute;
// anything else is ignored so a bad param cannot make Meilisearch reject the query.
var searchSortable = map[string]string{
	"created_at": "created_at",
	"posted_at":  "posted_at",
	"salary_min": "enrichment.salary_min",
	"salary_max": "enrichment.salary_max",
}

// sortMatch is the profile-match sort. Unlike every value in searchSortable it names
// no index attribute: it ranks by cosine against the caller's own skill vector, so it
// is resolved from the request rather than looked up in that table.
const sortMatch = "match"

// skillWeightsCacheTTL bounds how stale the skill-rarity weights may be. They change
// only when cmd/rollup-facets runs, and the design treats weight drift as harmless —
// a stale weight nudges the order, it cannot corrupt a stored vector. So this is
// generous where facetCacheTTL is deliberately short.
const skillWeightsCacheTTL = 15 * time.Minute

// skillWeightsCacheKey is a constant: the snapshot is catalogue-wide, identical for
// every caller.
const skillWeightsCacheKey = "search:skill-weights:v1"

// matchVector builds the caller's skill vector for ?sort=match, or nil when the sort
// was not asked for or cannot be served.
//
// EVERY "cannot" degrades to the default feed: no session, no profile service wired,
// no profile, no skills, no recognised skills, no weights. A saved search or a shared
// link carrying sort=match must never 400 when opened by someone it cannot be served
// for — the same reason the jobs list ignores unknown filters rather than refusing
// them.
func (h *searchHandlers) matchVector(c *fiber.Ctx) []float32 {
	if c.Query("sort") != sortMatch || h.userProfile == nil || h.facetStats == nil {
		return nil
	}
	// auth.UserID is what mw.optional populates; it reports false for an anonymous
	// caller rather than erroring. requireUserID is the wrong helper here — its 401 is
	// exactly the outcome this must avoid.
	userID, ok := auth.UserID(c)
	if !ok {
		return nil
	}
	profile, err := h.userProfile.Get(c.Context(), userID)
	if err != nil || len(profile.Skills) == 0 {
		return nil
	}
	weights, err := h.skillWeights(c.Context())
	if err != nil {
		return nil
	}
	// ProfileVector, never JobVector: the ballast belongs to the stored side only.
	return weights.ProfileVector(profile.Skills)
}

// skillWeights reads the rarity snapshot through a best-effort cache. The snapshot is
// the whole facet distribution, so reading it per request would be a needless query
// for a value identical across callers and near-constant in time. A nil cache, a
// miss, or any cache error falls through to a live read.
func (h *searchHandlers) skillWeights(ctx context.Context) (skillvec.Weights, error) {
	if h.cache == nil {
		return search.LoadSkillWeights(ctx, h.facetStats)
	}
	if cached, found, err := cache.GetJSON[skillvec.Weights](ctx, h.cache, skillWeightsCacheKey); err == nil && found {
		return cached, nil
	}
	w, err := search.LoadSkillWeights(ctx, h.facetStats)
	if err != nil {
		return w, err
	}
	// Best-effort: a failed write just means the next request reloads.
	_ = cache.SetJSON(ctx, h.cache, skillWeightsCacheKey, w, skillWeightsCacheTTL)
	return w, nil
}

// SearchJobs runs a full-text/faceted keyword search over the jobs index. It is
// public (unauthenticated) like the other job reads. Response: {"data": [job
// view...], "meta": {total, limit, offset}} — results carry public_slug and never
// the internal id.
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

	return listResponseWithIgnored(c, views, res.Total, limit, offset, ignoredParams(c, searchParams))
}

// runJobSearch performs the request handling shared by both job-search endpoints:
// the availability check, the pagination-window guard, and the index query. It is
// the single place the query is built, so the public and agent search endpoints
// cannot drift. The availability and deep-pagination guards return a fiber *Error
// the caller can return directly; on success it returns the raw hits and the
// applied limit/offset.
func (h *searchHandlers) runJobSearch(c *fiber.Ctx) (search.SearchResult, int, int, error) {
	if h.search == nil {
		return search.SearchResult{}, 0, 0, fiber.NewError(fiber.StatusServiceUnavailable, "search is not available")
	}

	limit, offset := pageParams(c)
	if offset+limit > maxSearchWindow {
		return search.SearchResult{}, 0, 0, fiber.NewError(fiber.StatusBadRequest, "pagination too deep")
	}

	vector := h.matchVector(c)
	res, err := h.search.Search(c.Context(), search.SearchParams{
		Query:  c.Query("q"),
		Filter: buildSearchFilter(c),
		Sort:   searchSort(c, vector != nil),
		Vector: vector,
		Limit:  limit,
		Offset: offset,
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
//
// ranked reports that the request carries a match vector. Meilisearch lets an explicit
// sort directive take precedence over vector ranking, so returning one here would
// silently discard the match order the caller asked for — not error, just quietly
// serve something else.
func searchSort(c *fiber.Ctx, ranked bool) []string {
	if ranked {
		return nil
	}
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
