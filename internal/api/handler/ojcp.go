package handler

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/jackc/pgx/v5"

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
	search searcher
	store  ojcpStore
	// attachReality hangs the posting-reality signal on a view. It is a FIELD, not a method
	// calling a package function, because the real lookup needs the concrete *db.Queries
	// (ghostEvidenceFor takes one, not a port) and a test must still be able to drive the
	// behaviour. Nil means the signal is off — the shape every test that does not care
	// about it uses.
	attachReality func(ctx context.Context, row db.Job, view *jobview.Job)
	projector     ojcp.Projector
}

// newOJCPHandlers builds the OJCP surface.
//
// submittable is what this deployment can complete an application on without a person, and
// is what `supports_agent_submission` is published from. Callers pass
// atsapply.SubmittableProviders() rather than a list, because this repo already holds three
// sets that are easy to mistake for one: jobview.AutoApplyProviders (four ATSs a fill may be
// ATTEMPTED on), the enqueue set (five that may be QUEUED), and the fill set — which is the
// only one that means "submitted", and is Greenhouse alone today. A fourth hand-written copy
// would be the one that goes stale, and the field it feeds is the one an agent plans around.
func newOJCPHandlers(s searcher, store ojcpStore, origin string, submittable map[string]bool) *ojcpHandlers {
	h := &ojcpHandlers{
		search:    s,
		store:     store,
		projector: ojcp.NewProjector(origin, submittable),
	}
	// The reality lookups need the concrete queries, which the production store is. The nil
	// check is here rather than inside the attacher so it runs once per construction instead
	// of once per request — and a store that cannot answer simply leaves the signal off.
	if q, ok := store.(*db.Queries); ok && q != nil {
		h.attachReality = realityAttacher(q)
	}
	return h
}

func (h *ojcpHandlers) register(api fiber.Router, mw middleware) {
	// Public and unauthenticated, like the other job reads. The rate limit is the same
	// agent-search one: these calls cost what /agent/jobs/search costs, and the manifest
	// declares that figure — a limit declared and not enforced breaks a MUST in the spec.
	limit := ojcpRateLimited(agentSearchLimiter(mw.throttler))
	api.Post("/ojcp/v1/search", limit, h.OJCPSearchJobs)
	api.Get("/ojcp/v1/jobs/:slug", limit, h.OJCPJobDetail)
	api.Get("/ojcp/v1/employers/:slug", limit, h.OJCPEmployerContext)

	// The MCP transport, mounted through Fiber's own adapter for a net/http handler. It
	// serves the SAME methods the three routes above call, so the two cannot answer one
	// question differently. `All` because MCP's streamable transport uses POST to call and
	// GET to open the server-to-client stream.
	api.All("/ojcp/mcp", limit, adaptor.HTTPHandler(ojcpmcp.Handler(h)))

	// The manifest, rendered. It is served to an agent at /.well-known/ojcp.json, which the
	// SPA proxies to this route — nginx sends /api/ here and everything else to the Node
	// process, so a route at the well-known path itself would never receive a request. That
	// was found the only way it could be: deployed, with the API path answering 200 and the
	// manifest answering 404.
	api.Get("/ojcp/manifest", limit, h.OJCPManifest)
}

// OJCPManifest answers with this deployment's manifest.
func (h *ojcpHandlers) OJCPManifest(c *fiber.Ctx) error {
	// The spec requires this content type by name.
	c.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	return c.JSON(h.manifest())
}

// manifest describes this deployment. Both binding claims are DERIVED rather than written
// out: `tools` comes from what the MCP server actually registered, and `rate_limits` from
// the same constant the limiter on these routes enforces — the spec makes a declared limit
// a MUST, so the two must not be able to disagree.
func (h *ojcpHandlers) manifest() ojcp.Manifest {
	return ojcp.NewManifest(ojcp.ManifestConfig{
		Origin: h.projector.Origin,
		Tools:  ojcpmcp.ToolNames(),
		// The manifest's only rate field is per SECOND, while the limiter holds a per-MINUTE
		// bucket — so this is the sustained average, and a burst well above it is allowed
		// before the bucket empties. The error is deliberately in the safe direction: an agent
		// pacing at the declared figure never meets a 429, which is what the declaration is
		// for. Declaring the burst instead would invite a rate we refuse.
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
		return ojcpError(c, ojcp.ErrorInvalidRequest, "request body is not valid JSON")
	}

	resp, err := h.SearchJobs(c.Context(), input)
	if err != nil {
		var fe *fiber.Error
		if errors.As(err, &fe) {
			return ojcpError(c, ojcp.ErrorProviderError, fe.Message)
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
	limit, offset := input.Page()

	res, err := h.search.Search(ctx, search.SearchParams{
		Query:  values.Get("q"),
		Filter: search.FilterFromValues(values),
		Limit:  limit,
		Offset: offset,
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
		Offset:        offset,
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
			return ojcpError(c, ojcp.ErrorJobNotFound, "no posting with that ojcp_id")
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
	if !publishedToAgents(row) {
		return ojcp.JobDetailResponse{}, ojcpmcp.NotFoundError{What: "posting"}
	}

	view, err := jobview.FromRow(row)
	if err != nil {
		return ojcp.JobDetailResponse{}, err
	}
	if h.attachReality != nil {
		h.attachReality(ctx, row, &view)
	}

	return ojcp.JobDetailResponse{
		Job: h.projector.JobPosting(view, h.applyFormFor(ctx, row.ID)),
	}.Finalize(), nil
}

// realityAttacher builds the function that computes the posting-reality signal and hangs it
// on a view, which is what makes the projection's `agent_notes` say anything at all.
//
// It has to be done explicitly: Ghost is NOT intrinsic to a jobview — it is time-dependent
// and never stored, so every surface that wants it attaches it (jobs.go, search.go,
// me_tracking.go). A projection that merely READS j.Ghost therefore publishes nothing, for
// every posting, forever — which is what this surface did until a review walked the call
// graph rather than the tests.
//
// Two lookups per posting, and only on the DETAIL read. The search tool deliberately does
// not carry the verdict: it would be two more queries per page for a field an agent has to
// open the posting to act on anyway, and `get_job_detail` is one call away.
//
// Best-effort throughout, like every other caller: a failed lookup leaves the signal off
// rather than failing the read, because the honest direction for a missing lookup is to say
// nothing.
func realityAttacher(q *db.Queries) func(context.Context, db.Job, *jobview.Job) {
	return func(ctx context.Context, row db.Job, view *jobview.Job) {
		repost, mass := int64(1), int64(1)
		if cnt, err := q.RoleClusterCount(ctx, db.RoleClusterCountParams{
			CompanySlug:     row.CompanySlug,
			RoleFingerprint: row.RoleFingerprint,
		}); err == nil {
			repost, mass = cnt.RepostCount, cnt.MassCount
		}

		now := time.Now()
		reality := jobview.ClassifyReality(row, now, int(repost), int(mass))
		view.Ghost = jobview.ClassifyGhost(jobview.GhostInput{
			Now:          now,
			Closed:       row.ClosedAt.Valid,
			RealityClass: reality.Class,
			ATSAbsentAt:  row.AtsAbsentAt.Time,
			HasATSAbsent: row.AtsAbsentAt.Valid,
			Evidence:     ghostEvidenceFor(ctx, q, []int64{row.ID})[row.ID],
		})
	}
}

// publishedToAgents reports whether a stored posting belongs on the OJCP surface: open,
// canonical, and not private — the set the public search publishes.
//
// The filter has to live here because `GetJobBySlug` carries NO predicate at all. It is the
// read a private job's own creator uses, and the detail page relies on that to serve a
// closed posting with its `closed_at` rendered. Neither is right for this surface: an agent
// enumerates, caches and republishes what it is handed, so a private posting reaching one is
// a different kind of exposure from a person following a link they were given.
//
// A refused posting answers NOT FOUND rather than forbidden. Whether a private posting
// exists under some slug is itself not an anonymous caller's business.
func publishedToAgents(row db.Job) bool {
	return !row.IsPrivate && !row.ClosedAt.Valid && !row.DuplicateOf.Valid
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
			return ojcpError(c, ojcp.ErrorEmployerNotFound, "no employer with that employer_id")
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

// ojcpRateLimited wraps the shared rate limiter so a refusal reaches an agent in the
// STANDARD's error envelope rather than this API's own `{"error": msg}`.
//
// Without it, `ojcpError`'s promise that an OJCP client never receives this API's shape by
// accident was false for the one refusal an agent is most likely to meet — and the schema
// makes `retry_after_seconds` mandatory alongside `rate_limited`, which our own body has no
// field for. The seconds come from the `Retry-After` header the limiter has already set, so
// the two cannot disagree.
func ojcpRateLimited(limiter fiber.Handler) fiber.Handler {
	return func(c *fiber.Ctx) error {
		err := limiter(c)

		var fe *fiber.Error
		if errors.As(err, &fe) && fe.Code == fiber.StatusTooManyRequests {
			return c.Status(fiber.StatusTooManyRequests).
				JSON(ojcp.NewRateLimitError(retryAfterSeconds(c)))
		}
		return err
	}
}

// retryAfterSeconds reads back the header the limiter set. A value it did not set, or one
// that will not parse, falls back to a second rather than to zero: the schema wants a real
// delay, and "retry immediately" is the one answer that turns a refusal into a loop.
func retryAfterSeconds(c *fiber.Ctx) int {
	seconds, err := strconv.Atoi(c.GetRespHeader("Retry-After"))
	if err != nil || seconds < 1 {
		return 1
	}
	return seconds
}

// ojcpError renders a failure in the standard's envelope with the matching HTTP status.
// Every failure on this surface goes through it, so an OJCP client never receives this
// API's own error shape by accident.
func ojcpError(c *fiber.Ctx, code, message string) error {
	return c.Status(ojcpHTTPStatus(code)).JSON(ojcp.NewError(code, message))
}

// ojcpHTTPStatus is the HTTP status each error code is served with. One mapping rather than a
// status argument at every call site: the two always said the same thing, and two places to
// say it is one place for them to disagree.
//
// An unrecognised code answers 500. An earlier version was a map, and argued in a comment
// that a missing entry "would serve status 0, which Fiber rejects" — measured, Fiber serves
// status 0 as **200 OK** with the error body, so an agent would read success. A switch with
// a real default is the protection; the sentence was not one.
func ojcpHTTPStatus(code string) int {
	switch code {
	case ojcp.ErrorInvalidRequest:
		return fiber.StatusBadRequest
	case ojcp.ErrorJobNotFound, ojcp.ErrorEmployerNotFound:
		return fiber.StatusNotFound
	case ojcp.ErrorRateLimited:
		return fiber.StatusTooManyRequests
	case ojcp.ErrorProviderError:
		return fiber.StatusServiceUnavailable
	default:
		return fiber.StatusInternalServerError
	}
}
