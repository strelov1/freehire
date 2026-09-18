package handler

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/api/mcpapp"
	"github.com/strelov1/freehire/internal/ingest/applyform"
	"github.com/strelov1/freehire/internal/job/jobview"
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

type mcpappHandlers struct {
	search        searcher
	companySearch companySearcher
	store         ojcpStore
	projector     mcpapp.Projector
}

func newMCPAppHandlers(s searcher, cs companySearcher, store ojcpStore, origin string) *mcpappHandlers {
	return &mcpappHandlers{
		search:        s,
		companySearch: cs,
		store:         store,
		projector:     mcpapp.NewProjector(origin),
	}
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
	api.All("/mcp", agentSearchLimiter(mw.throttler), adaptor.HTTPHandler(mcpapp.Handler(h)))
}

// SearchJobs answers `search_jobs`.
func (h *mcpappHandlers) SearchJobs(ctx context.Context, in mcpapp.SearchInput) (mcpapp.SearchJobsResult, error) {
	if h.search == nil {
		return mcpapp.SearchJobsResult{}, fiber.NewError(fiber.StatusServiceUnavailable, "search is not available")
	}

	values := in.QueryValues()
	limit, offset := in.Page()

	res, err := h.search.Search(ctx, search.SearchParams{
		Query:  values.Get("q"),
		Filter: search.FilterFromValues(values),
		Limit:  limit,
		Offset: offset,
	})
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
		Query:         values.Get("q"),
		Total:         int(res.Total),
		Offset:        offset,
		Jobs:          jobs,
		IgnoredParams: in.Unsupported(),
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
