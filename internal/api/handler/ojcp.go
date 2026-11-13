package handler

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/api/ojcp"
	"github.com/strelov1/freehire/internal/api/ojcpmcp"
	"github.com/strelov1/freehire/internal/application/agentview"
	"github.com/strelov1/freehire/internal/ingest/applyform"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/db"
)

// OJCP REST transport. The three read tools, answered over plain HTTP so the conformance
// suite and any ordinary client can exercise them without speaking a protocol.
//
// Nothing here decides anything about the SHAPE of an answer: it loads data the way the
// neighbouring handlers do and hands it to internal/api/ojcp, which the MCP transport calls
// with the same values. That is what keeps the two from drifting.

// ojcpStore is the data this transport reads. *db.Queries satisfies it; tests inject a fake.
type ojcpStore interface {
	jobSlugReader
	GetApplyFormByJobID(ctx context.Context, jobID int64) (db.GetApplyFormByJobIDRow, error)
	GetCompany(ctx context.Context, slug string) (db.Company, error)
}

type ojcpHandlers struct {
	// agentViews receives the postings served; nil records nothing.
	agentViews agentview.Sink
	search     searcher
	store      ojcpStore
	// attachReality hangs the posting-reality signal on a view. It is a FIELD, not a method
	// calling a package function, because the real lookup needs the concrete *db.Queries
	// (ghostEvidenceFor takes one, not a port) and a test must still be able to drive the
	// behaviour. Nil means the signal is off — the shape every test that does not care
	// about it uses.
	attachReality func(ctx context.Context, row db.Job, view *jobview.Job)
	projector     ojcp.Projector
	// signer signs the manifest and publishes the key that verifies it. Nil means this
	// deployment has no signing key: the manifest goes out unsigned and no key set exists.
	signer *ojcp.Signer
	// origin is the site's address; the MCP server builds its icon and website link from it.
	origin string
}

// newOJCPHandlers builds the OJCP surface.
//
// submittable is what this deployment can complete an application on without a person, and
// is what `supports_agent_submission` is published from. Callers pass
// atsapply.SubmittableProviders() rather than a list, because this repo already holds three
// sets that are easy to mistake for one: jobview.AutoApplyProviders (four ATSs a fill may be
// ATTEMPTED on), the enqueue set (five that may be QUEUED), and the submittable set — the
// only one that means "submitted". A fourth hand-written copy
// would be the one that goes stale, and the field it feeds is the one an agent plans around.
func newOJCPHandlers(s searcher, store ojcpStore, origin string, submittable map[string]bool) *ojcpHandlers {
	h := &ojcpHandlers{
		search:    s,
		store:     store,
		projector: ojcp.NewProjector(origin, submittable),
		origin:    origin,
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
	// Public and unauthenticated, like the other job reads.
	views := agentViews(h.agentViews, agentview.SurfaceOJCP)
	api.Post("/ojcp/v1/search", views, h.OJCPSearchJobs)
	api.Get("/ojcp/v1/jobs/:slug", views, h.OJCPJobDetail)
	api.Get("/ojcp/v1/employers/:slug", h.OJCPEmployerContext)

	// The MCP transport, mounted through Fiber's own adapter for a net/http handler. It
	// serves the SAME methods the three routes above call, so the two cannot answer one
	// question differently. `All` because MCP's streamable transport uses POST to call and
	// GET to open the server-to-client stream.
	api.All("/ojcp/mcp", views, adaptor.HTTPHandler(ojcpmcp.Handler(h, h.origin)))

	// The manifest, rendered. It is served to an agent at /.well-known/ojcp.json, which the
	// SPA proxies to this route — nginx sends /api/ here and everything else to the Node
	// process, so a route at the well-known path itself would never receive a request. That
	// was found the only way it could be: deployed, with the API path answering 200 and the
	// manifest answering 404.
	api.Get("/ojcp/manifest", h.OJCPManifest)
	// The key set the manifest's signature verifies against, proxied by the SPA to
	// /.well-known/ojcp-keys.json for the same reason the manifest is.
	api.Get("/ojcp/keys", h.OJCPKeys)
}

// OJCPManifest answers with this deployment's manifest.
func (h *ojcpHandlers) OJCPManifest(c *fiber.Ctx) error {
	// The spec requires this content type by name.
	c.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	m := h.manifest()
	if h.signer != nil {
		// Signed per request: a signature lives 24 hours, so one made at startup would
		// expire on any deployment that runs longer than a day.
		m = h.signer.Sign(m, time.Now())
	}
	return c.JSON(m)
}

// OJCPKeys answers with the JWK Set the manifest's signature verifies against.
func (h *ojcpHandlers) OJCPKeys(c *fiber.Ctx) error {
	if h.signer == nil {
		return fiber.NewError(fiber.StatusNotFound, "this provider does not sign its manifest")
	}
	// The spec requires this content type by name.
	return c.JSON(h.signer.KeySet(), "application/jwk-set+json")
}

// manifest derives its tool list from the MCP server registration.
func (h *ojcpHandlers) manifest() ojcp.Manifest {
	return ojcp.NewManifest(ojcp.ManifestConfig{
		Origin:         h.projector.Origin,
		Tools:          ojcpmcp.ToolNames(),
		ApplyPathTypes: ojcp.ApplyPathTypes,
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
	if err := c.JSON(resp); err != nil {
		return err
	}
	observeOJCPRESTSearch(resp)
	return nil
}

// SearchJobs answers `search_jobs` with no transport around it. Both transports call it, so
// the same question cannot be answered differently over REST and over MCP.
//
// It runs the query the public search runs, built from the same values. OJCP has no sort
// directive and no match vector, so neither is read — a text query gets the index's own
// relevance ordering, an empty one the freshest first (defaultSort).
func (h *ojcpHandlers) SearchJobs(ctx context.Context, input ojcp.SearchInput) (ojcp.SearchJobsResponse, error) {
	if h.search == nil {
		return ojcp.SearchJobsResponse{}, fiber.NewError(fiber.StatusServiceUnavailable, "search is not available")
	}

	values, unsupported := input.QueryValues()
	limit, offset := input.Page()
	res, outcome, err := runAgentSearch(ctx, h.search, values, limit, offset, input.WantsSemantic())
	if err != nil {
		return ojcp.SearchJobsResponse{}, err
	}
	// A semantic search over no text is the plain filtered listing, so the flag was not
	// honoured and says so like any dropped param.
	unsupported = outcome.noted(unsupported)

	jobs := make([]ojcp.JobPosting, 0, len(res.Hits))
	for _, hit := range res.Hits {
		jobs = append(jobs, h.projector.JobPosting(hit.Job, nil))
	}

	return ojcp.SearchJobsResponse{
		Query:             values.Get("q"),
		TotalResults:      int(res.Total),
		TotalIsLowerBound: res.TotalIsLowerBound,
		Offset:            offset,
		Jobs:              jobs,
		IgnoredParams:     unsupported,
		Semantic:          outcome.wire(ojcp.SemanticUnavailable),
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
	row, view, err := loadAgentJob(ctx, func(ctx context.Context, slug string) (db.Job, error) {
		return publishedJobBySlug(ctx, h.store, slug)
	}, h.attachReality, ojcpID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ojcp.JobDetailResponse{}, ojcpmcp.NotFoundError{What: "posting"}
		}
		return ojcp.JobDetailResponse{}, err
	}
	return ojcp.JobDetailResponse{
		Job: h.projector.JobPosting(view, applyFormFor(ctx, h.store, row.ID)),
	}.Finalize(), nil
}

// realityAttacher builds the function that computes the posting-reality signal and hangs it
// on a view — both the reality class and the ghost signal derived from it — which is what
// makes the OJCP projection's `agent_notes`, and the ChatGPT app's `get_job` liveness fields,
// say anything at all. Each projection publishes the part it has a field for.
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
		view.Reality = &reality
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

// publishedJobBySlug is the loader OJCP and similar_jobs use (get_job uses agentJobBySlug):
// slug-or-shortcode resolution, then publishedToAgents. A refused posting answers
// pgx.ErrNoRows, indistinguishable from an unknown slug.
func publishedJobBySlug(ctx context.Context, q jobSlugReader, slug string) (db.Job, error) {
	row, err := jobBySlugOrCode(ctx, q, slug)
	if err != nil {
		return db.Job{}, err
	}
	if !publishedToAgents(row) {
		return db.Job{}, pgx.ErrNoRows
	}
	return row, nil
}

// agentJobReader is what agentJobBySlug needs: the slug lookup plus a read by id to follow a
// duplicate to its canonical posting. *db.Queries and mcpappStore satisfy it.
type agentJobReader interface {
	jobSlugReader
	GetJobsByIDs(ctx context.Context, ids []int64) ([]db.Job, error)
}

// agentJobBySlug is get_job's loader on the MCP surfaces. Unlike publishedJobBySlug it answers
// for every slug search_jobs can hand out: the index trails the database, so a posting closed
// or folded into a duplicate since the last push is still a search hit (four agent reports,
// 2026-11-07, all "get_job cannot find the slug search_jobs returned"). A closed posting is
// served with its closed_at, as the job page serves it; a duplicate is served as its canonical
// posting, or as itself when that one is not public. Private stays a miss.
func agentJobBySlug(ctx context.Context, q agentJobReader, slug string) (db.Job, error) {
	row, err := jobBySlugOrCode(ctx, q, slug)
	if err != nil {
		return db.Job{}, err
	}
	if row.IsPrivate {
		return db.Job{}, pgx.ErrNoRows
	}
	if row.DuplicateOf.Valid {
		owners, err := q.GetJobsByIDs(ctx, []int64{row.DuplicateOf.Int64})
		if err != nil {
			return db.Job{}, err
		}
		if len(owners) == 1 && !owners[0].IsPrivate {
			return owners[0], nil
		}
	}
	return row, nil
}

// applyFormFor reads the posting's captured application form, or nil where there is none.
//
// Best-effort by design: most of the catalogue has no captured form, and a store that
// cannot answer must not turn a job read into a failure. The posting simply falls back to
// the external-redirect apply path.
func applyFormFor(ctx context.Context, store ojcpStore, jobID int64) *applyform.Form {
	row, err := store.GetApplyFormByJobID(ctx, jobID)
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
	case ojcp.ErrorProviderError:
		return fiber.StatusServiceUnavailable
	default:
		return fiber.StatusInternalServerError
	}
}
