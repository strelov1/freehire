package handler

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/api/atsapply"
	"github.com/strelov1/freehire/internal/api/ojcp"
	"github.com/strelov1/freehire/internal/api/ojcpmcp"
	"github.com/strelov1/freehire/internal/ingest/applyform"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/search"
)

// OJCP REST transport. The three read tools, answered over plain HTTP so the conformance
// suite and any ordinary client can exercise them without speaking a protocol.
//
// Nothing here decides anything about the SHAPE of an answer: it loads data the way the
// neighbouring handlers do and hands it to internal/api/ojcp, which the MCP transport calls
// with the same values. That is what keeps the two from drifting.

// ojcpStore is the data this transport reads. *db.Queries satisfies it; tests inject a fake.
type ojcpStore interface {
	GetJobBySlug(ctx context.Context, publicSlug string) (db.Job, error)
	GetApplyFormByJobID(ctx context.Context, jobID int64) (db.GetApplyFormByJobIDRow, error)
	GetCompany(ctx context.Context, slug string) (db.Company, error)
}

type ojcpHandlers struct {
	search    searcher
	store     ojcpStore
	projector ojcp.Projector
}

// ojcpSubmittableProviders is what this deployment can complete an application on without
// a person — the value `supports_agent_submission` is published from.
//
// It asks atsapply rather than listing providers, because this repo already holds three
// lists that are easy to mistake for one: jobview.AutoApplyProviders (four ATSs a fill may
// be ATTEMPTED on), the enqueue set (five that may be QUEUED), and the fill set — which
// is the only one that means "submitted", and is Greenhouse alone today. A fourth
// hand-written copy here would be the one that goes stale, and the field it feeds is the
// one an agent plans around.
func ojcpSubmittableProviders() map[string]bool {
	return atsapply.SubmittableProviders()
}

func newOJCPHandlers(s searcher, store ojcpStore, origin string, submittable map[string]bool) *ojcpHandlers {
	return &ojcpHandlers{
		search:    s,
		store:     store,
		projector: ojcp.NewProjector(origin, submittable),
	}
}

func (h *ojcpHandlers) register(api fiber.Router, mw middleware) {
	// Public and unauthenticated, like the other job reads. The rate limit is the same
	// agent-search one: these calls cost what /agent/jobs/search costs, and the manifest
	// declares that figure — a limit declared and not enforced breaks a MUST in the spec.
	limit := agentSearchLimiter(mw.throttler)
	api.Post("/ojcp/v1/search", limit, h.OJCPSearchJobs)
	api.Get("/ojcp/v1/jobs/:slug", limit, h.OJCPJobDetail)
	api.Get("/ojcp/v1/employers/:slug", limit, h.OJCPEmployerContext)

	// The MCP transport, mounted through Fiber's own adapter for a net/http handler. It
	// serves the SAME methods the three routes above call, so the two cannot answer one
	// question differently. `All` because MCP's streamable transport uses POST to call and
	// GET to open the server-to-client stream.
	api.All("/ojcp/mcp", limit, adaptor.HTTPHandler(ojcpmcp.Handler(h)))
}

// registerManifest serves the one document an OJCP provider MUST publish. It is mounted on
// the app root, not under /api/v1: the well-known path is fixed by RFC 8615 and by the
// spec, and an agent looks for it there and nowhere else.
func (h *ojcpHandlers) registerManifest(app *fiber.App) {
	app.Get("/.well-known/ojcp.json", func(c *fiber.Ctx) error {
		// The spec requires this content type by name.
		c.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
		return c.JSON(h.manifest())
	})
}

// manifest describes this deployment. Both binding claims are DERIVED rather than written
// out: `tools` comes from what the MCP server actually registered, and `rate_limits` from
// the same constant the limiter on these routes enforces — the spec makes a declared limit
// a MUST, so the two must not be able to disagree.
func (h *ojcpHandlers) manifest() ojcp.Manifest {
	return ojcp.NewManifest(ojcp.ManifestConfig{
		Origin: h.projector.Origin,
		Tools:  ojcpmcp.ToolNames(),
		// Per second, from the per-minute budget the routes above are limited by.
		AnonymousRPS:   agentSearchPerMinute / 60,
		ApplyPathTypes: []string{"ats_direct", "external_redirect"},
	})
}

// OJCPSearchJobs answers `search_jobs`.
func (h *ojcpHandlers) OJCPSearchJobs(c *fiber.Ctx) error {
	var input ojcp.SearchInput
	// An unrecognised field is IGNORED rather than refused, per the standard's own
	// extensibility rule: an agent written against a later spec version must still get an
	// answer. encoding/json does this by default; the explicit note is here so nobody
	// "fixes" it with DisallowUnknownFields later.
	if err := json.Unmarshal(c.Body(), &input); len(c.Body()) > 0 && err != nil {
		return ojcpError(c, fiber.StatusBadRequest, "invalid_input", "request body is not valid JSON")
	}

	resp, err := h.SearchJobs(c.Context(), input)
	if err != nil {
		var fe *fiber.Error
		if errors.As(err, &fe) {
			return ojcpError(c, fe.Code, "unavailable", fe.Message)
		}
		return err
	}
	return c.JSON(resp)
}

// SearchJobs answers `search_jobs` with no transport around it. Both transports call it, so
// the same question cannot be answered differently over REST and over MCP.
//
// It runs the query the public search runs, built from the same values. OJCP has no sort
// directive and no match vector, so neither is read — the index's own relevance ordering is
// what an agent gets.
func (h *ojcpHandlers) SearchJobs(ctx context.Context, input ojcp.SearchInput) (ojcp.SearchJobsResponse, error) {
	if h.search == nil {
		return ojcp.SearchJobsResponse{}, fiber.NewError(fiber.StatusServiceUnavailable, "search is not available")
	}

	values, unsupported := input.QueryValues()
	res, err := h.search.Search(ctx, search.SearchParams{
		Query:  values.Get("q"),
		Filter: search.FilterFromValues(values),
		Limit:  intOr(values.Get("limit"), 10),
		Offset: intOr(values.Get("offset"), 0),
	})
	if err != nil {
		return ojcp.SearchJobsResponse{}, err
	}

	jobs := make([]ojcp.JobPosting, 0, len(res.Hits))
	for _, hit := range res.Hits {
		jobs = append(jobs, h.projector.JobPosting(hit.Job, nil))
	}

	return ojcp.SearchJobsResponse{
		Query:         values.Get("q"),
		TotalResults:  int(res.Total),
		Offset:        intOr(values.Get("offset"), 0),
		Jobs:          jobs,
		IgnoredParams: unsupported,
	}.Finalize(), nil
}

// OJCPJobDetail answers `get_job_detail` over REST.
func (h *ojcpHandlers) OJCPJobDetail(c *fiber.Ctx) error {
	resp, err := h.JobDetail(c.Context(), c.Params("slug"))
	if err != nil {
		var notFound ojcpmcp.NotFoundError
		if errors.As(err, &notFound) {
			return ojcpError(c, fiber.StatusNotFound, "not_found", "no posting with that ojcp_id")
		}
		return err
	}
	return c.JSON(resp)
}

// JobDetail answers `get_job_detail` with no transport around it.
func (h *ojcpHandlers) JobDetail(ctx context.Context, ojcpID string) (ojcp.JobDetailResponse, error) {
	row, err := h.store.GetJobBySlug(ctx, ojcpID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ojcp.JobDetailResponse{}, ojcpmcp.NotFoundError{What: "posting"}
		}
		return ojcp.JobDetailResponse{}, err
	}

	view, err := jobview.FromRow(row)
	if err != nil {
		return ojcp.JobDetailResponse{}, err
	}

	return ojcp.JobDetailResponse{
		Job: h.projector.JobPosting(view, h.applyFormFor(ctx, row.ID)),
	}.Finalize(), nil
}

// applyFormFor reads the posting's captured application form, or nil where there is none.
//
// Best-effort by design: most of the catalogue has no captured form, and a store that
// cannot answer must not turn a job read into a failure. The posting simply falls back to
// the external-redirect apply path.
func (h *ojcpHandlers) applyFormFor(ctx context.Context, jobID int64) *applyform.Form {
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

// OJCPEmployerContext answers `get_employer_context` over REST.
func (h *ojcpHandlers) OJCPEmployerContext(c *fiber.Ctx) error {
	resp, err := h.EmployerContext(c.Context(), c.Params("slug"))
	if err != nil {
		var notFound ojcpmcp.NotFoundError
		if errors.As(err, &notFound) {
			return ojcpError(c, fiber.StatusNotFound, "not_found", "no employer with that employer_id")
		}
		return err
	}
	return c.JSON(resp)
}

// EmployerContext answers `get_employer_context` with no transport around it.
func (h *ojcpHandlers) EmployerContext(ctx context.Context, employerID string) (ojcp.EmployerContextResponse, error) {
	company, err := h.store.GetCompany(ctx, employerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ojcp.EmployerContextResponse{}, ojcpmcp.NotFoundError{What: "employer"}
		}
		return ojcp.EmployerContextResponse{}, err
	}
	return ojcp.EmployerContextFrom(company), nil
}

// intOr reads a value this package itself wrote into the query values a moment earlier, so
// an unparseable one means a bug here rather than bad input — the fallback keeps the page
// sane instead of failing a read over it.
func intOr(raw string, fallback int) int {
	n, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return n
}

// ojcpError renders a failure in the standard's envelope with the matching HTTP status.
// Every failure on this surface goes through it, so an OJCP client never receives this
// API's own error shape by accident.
func ojcpError(c *fiber.Ctx, status int, code, message string) error {
	return c.Status(status).JSON(ojcp.NewError(code, message))
}
