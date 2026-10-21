package handler

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/api/mcpapp"
	"github.com/strelov1/freehire/internal/ingest/applyform"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/cache"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/search"
)

// The ChatGPT app's MCP surface: what loads the data the tools answer with.
//
// Nothing here decides the SHAPE of an answer — that is internal/api/mcpapp's projector.
// This is the other half of the same split the OJCP handlers make: load the way the
// neighbouring handlers load, hand the values to a projection that knows nothing about
// Fiber or Postgres.
//
// It reuses ojcpStore rather than declaring an identical interface beside it. The two
// surfaces read exactly the same three rows, and a second interface with the same methods
// would be a second thing to keep in step for no gain.

// mcpappStore is what the app's surface reads: the OJCP surface's store plus the one batch
// read check_jobs_open needs. *db.Queries satisfies it; tests inject a fake.
type mcpappStore interface {
	ojcpStore
	similarReader
	salaryReader
	ListJobLivenessBySlugs(ctx context.Context, slugs []string) ([]db.ListJobLivenessBySlugsRow, error)
}

type mcpappHandlers struct {
	search        searcher
	companySearch companySearcher
	store         mcpappStore
	// attachReality hangs the liveness signals (reality and ghost) on the posting get_job
	// reads. A field for the reason ojcpHandlers' is one: the real lookup needs the concrete
	// *db.Queries, and a test must still drive the seam. Nil leaves the signals off.
	attachReality func(ctx context.Context, row db.Job, view *jobview.Job)
	// facets and cache back job_market_stats with the website's facet read and its cache
	// slots. Set after construction by the router, like ojcpHandlers.signer; nil facets
	// refuses the tool rather than answering an empty market.
	facets    facetCounter
	cache     cache.Cache
	projector mcpapp.Projector
	// semantic answers search_jobs' opt-in `semantic: true`; the same value the OJCP and
	// agent search surfaces hold.
	semantic semanticSearch
	// origin is the site's address; the MCP server builds its icon and website link from it.
	origin string
}

func newMCPAppHandlers(s searcher, cs companySearcher, store mcpappStore, origin string) *mcpappHandlers {
	h := &mcpappHandlers{
		search:        s,
		companySearch: cs,
		store:         store,
		projector:     mcpapp.NewProjector(origin),
		origin:        origin,
	}
	if q, ok := store.(*db.Queries); ok && q != nil {
		h.attachReality = realityAttacher(q)
	}
	return h
}

func (h *mcpappHandlers) register(api fiber.Router, mw middleware) {
	// `All` because MCP's streamable transport uses POST to call and GET to open the
	// server-to-client stream.
	//
	// The path is /api/v1/mcp and cannot be a bare /mcp: nginx routes /api/ here and
	// everything else to the SPA, so a route at a bare path never receives a request. That
	// was found the only way it could be — deployed, with the OJCP manifest answering 404
	// at its well-known path while the API path beside it answered 200.
	//
	// Public and unauthenticated like the other job reads, under the same agent-search rate
	// limit the OJCP endpoints use. A separate figure here would be a second answer to "how
	// hard may a machine ask", with nothing deciding which one is right.
	api.All("/mcp", chatAgentLimiter(mw.public), adaptor.HTTPHandler(mcpapp.Handler(h, h.origin)))
}

// SearchJobs answers `search_jobs`.
func (h *mcpappHandlers) SearchJobs(ctx context.Context, in mcpapp.SearchInput) (mcpapp.SearchJobsResult, error) {
	if h.search == nil {
		return mcpapp.SearchJobsResult{}, fiber.NewError(fiber.StatusServiceUnavailable, "search is not available")
	}

	values := in.QueryValues()
	limit, offset := in.Page()

	params := search.SearchParams{
		Query:  values.Get("q"),
		Filter: search.FilterFromValues(values),
		Limit:  limit,
		Offset: offset,
	}
	ignored := in.Unsupported()
	res, outcome, err := h.semantic.search(ctx, h.search, params, in.WantsSemantic())
	if outcome == semanticDropped {
		// Nothing to embed, so the flag was not honoured and says so like any dropped filter.
		ignored = append(ignored, "semantic")
	}
	if err != nil {
		return mcpapp.SearchJobsResult{}, err
	}

	// The hits carry the index's truncated description and are NOT rehydrated from Postgres.
	// /agent/jobs/search does rehydrate, deliberately, because a programmatic consumer
	// reading whole bodies is what it is for. Here it would spend the turn's context on ten
	// full descriptions the model reduces to a line each; get_job reads the one that was
	// picked.
	jobs := make([]mcpapp.JobSummary, 0, len(res.Hits))
	for _, hit := range res.Hits {
		jobs = append(jobs, h.projector.JobSummary(hit.Job))
	}

	return mcpapp.SearchJobsResult{
		Query:             values.Get("q"),
		Total:             int(res.Total),
		Offset:            offset,
		Jobs:              jobs,
		IgnoredParams:     ignored,
		Semantic:          outcome.wire(mcpapp.SemanticApplied, mcpapp.SemanticUnavailable),
		TotalIsLowerBound: res.TotalIsLowerBound,
	}, nil
}

// JobDetail answers `get_job`.
func (h *mcpappHandlers) JobDetail(ctx context.Context, slug string) (mcpapp.JobResult, error) {
	row, err := h.store.GetJobBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return mcpapp.JobResult{}, mcpapp.NotFoundError{What: "posting"}
		}
		return mcpapp.JobResult{}, err
	}
	// The same predicate the OJCP surface applies, and for the same reason: GetJobBySlug
	// carries none of its own, and a refused posting must answer NOT FOUND rather than
	// forbidden — whether a private posting exists under some slug is not an anonymous
	// caller's business.
	if !publishedToAgents(row) {
		return mcpapp.JobResult{}, mcpapp.NotFoundError{What: "posting"}
	}

	view, err := jobview.FromRow(row)
	if err != nil {
		return mcpapp.JobResult{}, err
	}
	// Computed here, on read, rather than taken from the index: the index's reality class
	// is frozen at push time, and the ghost signal is never stored at all.
	if h.attachReality != nil {
		h.attachReality(ctx, row, &view)
	}
	return h.projector.JobDetail(view, h.applyFormFor(ctx, row.ID)), nil
}

// applyFormFor reads the posting's captured application form, or nil where there is none.
//
// Best-effort, exactly as on the OJCP surface: most of the catalogue has no captured form,
// and a store that cannot answer must not turn a job read into a failure — the posting
// simply says nothing about what applying will ask for, which is the honest answer when we
// do not know.
func (h *mcpappHandlers) applyFormFor(ctx context.Context, jobID int64) *applyform.Form {
	row, err := h.store.GetApplyFormByJobID(ctx, jobID)
	if err != nil {
		return nil
	}
	var form applyform.Form
	if err := json.Unmarshal(row.Payload, &form); err != nil {
		return nil
	}
	if form.Provider == "" {
		form.Provider = row.Provider
	}
	return &form
}

// SearchCompanies answers `search_companies`.
func (h *mcpappHandlers) SearchCompanies(ctx context.Context, in mcpapp.CompanySearchInput) (mcpapp.SearchCompaniesResult, error) {
	if h.companySearch == nil {
		return mcpapp.SearchCompaniesResult{}, fiber.NewError(fiber.StatusServiceUnavailable, "company search is not available")
	}

	values := in.QueryValues()
	limit, offset := in.Page()

	res, err := h.companySearch.SearchCompanies(ctx, search.CompanySearchParams{
		Query:  values.Get("q"),
		Filter: search.CompanyFilterFromValues(values),
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		return mcpapp.SearchCompaniesResult{}, err
	}

	companies := make([]mcpapp.CompanySummary, 0, len(res.Hits))
	for _, hit := range res.Hits {
		companies = append(companies, h.projector.CompanySummary(hit))
	}

	return mcpapp.SearchCompaniesResult{
		Query:         values.Get("q"),
		Total:         int(res.Total),
		Offset:        offset,
		Companies:     companies,
		IgnoredParams: in.Unsupported(),
	}, nil
}

// CompanyDetail answers `get_company`.
func (h *mcpappHandlers) CompanyDetail(ctx context.Context, slug string) (mcpapp.CompanyResult, error) {
	company, err := h.store.GetCompany(ctx, slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return mcpapp.CompanyResult{}, mcpapp.NotFoundError{What: "employer"}
		}
		return mcpapp.CompanyResult{}, err
	}
	return h.projector.CompanyDetail(company), nil
}

// CheckJobsOpen answers `check_jobs_open`: one state per slug, in the order asked.
//
// closed is reported as closed — telling a shortlist which postings went down is the point
// of the tool, which is why it does not reuse publishedToAgents (that predicate refuses a
// closed posting outright). Private and duplicate postings answer not_found, the same
// indistinguishability get_job keeps.
//
// A failed lookup is an error, never a page of not_found: "none of your postings exist" is a
// confident, wrong answer a person would act on.
func (h *mcpappHandlers) CheckJobsOpen(ctx context.Context, slugs []string) ([]mcpapp.JobOpenState, error) {
	rows, err := h.store.ListJobLivenessBySlugs(ctx, slugs)
	if err != nil {
		return nil, err
	}
	bySlug := make(map[string]db.ListJobLivenessBySlugsRow, len(rows))
	for _, r := range rows {
		bySlug[r.PublicSlug] = r
	}

	out := make([]mcpapp.JobOpenState, 0, len(slugs))
	for _, slug := range slugs {
		row, found := bySlug[slug]
		out = append(out, jobOpenState(slug, row, found))
	}
	return out, nil
}

// jobOpenState is one slug's answer. found is false for a slug the lookup did not return.
func jobOpenState(slug string, row db.ListJobLivenessBySlugsRow, found bool) mcpapp.JobOpenState {
	switch {
	case !found || row.IsPrivate || row.DuplicateOf.Valid:
		return mcpapp.JobOpenState{Slug: slug, Status: mcpapp.StatusNotFound}
	case row.ClosedAt.Valid:
		return mcpapp.JobOpenState{Slug: slug, Status: mcpapp.StatusClosed, ClosedAt: utcRFC3339(row.ClosedAt)}
	default:
		return mcpapp.JobOpenState{Slug: slug, Status: mcpapp.StatusOpen, LastSeenAt: utcRFC3339(row.LastSeenAt)}
	}
}

// utcRFC3339 renders a nullable timestamp the way every timestamp on this surface reads, or
// nothing for NULL.
func utcRFC3339(ts pgtype.Timestamptz) string {
	if !ts.Valid {
		return ""
	}
	return ts.Time.UTC().Format(time.RFC3339)
}

// SimilarJobs answers `similar_jobs`.
//
// The source posting is checked against publishedToAgents first: answering a private slug
// with its neighbours, while an unknown one answers not found, would tell an anonymous caller
// the private posting exists. The neighbours pass the same predicate — the website's rule
// plus duplicates, which no tool on this surface publishes.
func (h *mcpappHandlers) SimilarJobs(ctx context.Context, slug string, limit int) (mcpapp.SimilarJobsResult, error) {
	row, err := h.store.GetJobBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return mcpapp.SimilarJobsResult{}, mcpapp.NotFoundError{What: "posting"}
		}
		return mcpapp.SimilarJobsResult{}, err
	}
	if !publishedToAgents(row) {
		return mcpapp.SimilarJobsResult{}, mcpapp.NotFoundError{What: "posting"}
	}

	views, err := similarViews(ctx, h.store, row.ID, limit, publishedToAgents)
	if err != nil {
		return mcpapp.SimilarJobsResult{}, err
	}
	jobs := make([]mcpapp.JobSummary, 0, len(views))
	for _, v := range views {
		jobs = append(jobs, h.projector.JobSummary(v))
	}
	return mcpapp.SimilarJobsResult{Jobs: jobs}, nil
}

// SalaryStats answers `salary_stats` from the same rollup read /insights/salary serves.
func (h *mcpappHandlers) SalaryStats(ctx context.Context, in mcpapp.SalaryInput) (mcpapp.SalaryStatsResult, error) {
	category, seniority, country := in.Scope()
	bands, _, err := salaryBands(ctx, h.store, category, seniority, country)
	if err != nil {
		return mcpapp.SalaryStatsResult{}, err
	}
	out := make([]mcpapp.SalaryBand, len(bands))
	for i, b := range bands {
		out[i] = mcpapp.SalaryBand{
			Seniority: b.Seniority, Currency: b.Currency, Period: b.Period,
			SampleSize: int(b.SampleSize), P25: int(b.P25), P50: int(b.P50), P75: int(b.P75),
		}
	}
	return mcpapp.SalaryStatsResult{Bands: out, IgnoredParams: in.Unsupported()}, nil
}

// MarketStats answers `job_market_stats`: the website's facet read over exactly the filter
// search_jobs would run for the same input, re-keyed by facetView to the public parameter
// names and cut to the top values by the projector.
func (h *mcpappHandlers) MarketStats(ctx context.Context, in mcpapp.SearchInput) (mcpapp.MarketStatsResult, error) {
	if h.facets == nil {
		return mcpapp.MarketStatsResult{}, fiber.NewError(fiber.StatusServiceUnavailable, "search is not available")
	}
	values := in.QueryValues()
	res, err := cachedFacetCounts(ctx, h.facets, h.cache, search.FacetParams{
		Query:  values.Get("q"),
		Filter: search.FilterFromValues(values),
		Facets: facetAttributes(),
	})
	if err != nil {
		return mcpapp.MarketStatsResult{}, err
	}
	view := facetView(res)
	out := h.projector.MarketStats(view.Total, view.Facets)
	out.IgnoredParams = in.Unsupported()
	return out, nil
}
