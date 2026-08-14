package handler

import (
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/linkimport"
	"github.com/strelov1/freehire/internal/moderation"
	"github.com/strelov1/freehire/internal/submission"
)

// submissionHandlers serves the public job-submission queue: any authenticated
// user submits a vacancy for review and reads their own queue; the review actions
// are moderator-gated. Approval mints a live job by delegating to moderation.
// Prefill reuses the same importer the paste-a-link contribution flow resolves
// through, but never writes — see PrefillSubmission.
type submissionHandlers struct {
	submission *submission.Service
	importer   *linkimport.Importer
}

func newSubmissionHandlers(queries *db.Queries, moderation *moderation.Service, importer *linkimport.Importer) *submissionHandlers {
	// Submission approval mints through the same moderation service, so derivation,
	// dedup, and the enrichment enqueue are reused rather than duplicated.
	return &submissionHandlers{
		submission: submission.New(submission.NewQueriesRepository(queries), moderation),
		importer:   importer,
	}
}

func (h *submissionHandlers) register(api fiber.Router, mw middleware) {
	// Public job submissions: any authenticated user submits a vacancy for review
	// (cookie or API key) and reads their own queue; the review actions (the pending
	// queue, approve, reject) are moderator-gated. Approval mints a live job — the same
	// path CreateJob uses — so an approved submission is indistinguishable from a
	// hand-curated one. Prefill makes the same class of outbound request /jobs/resolve
	// does, so it sits behind the same throttle.
	api.Post("/submissions", mw.key, h.CreateSubmission)
	api.Post("/submissions/prefill", mw.key, mw.outboundFetch, h.PrefillSubmission)
	api.Get("/me/submissions", mw.key, h.ListMySubmissions)
	api.Get("/submissions", mw.key, mw.moderator, h.ListPendingSubmissions)
	api.Post("/submissions/:id/approve", mw.key, mw.moderator, h.ApproveSubmission)
	api.Post("/submissions/:id/reject", mw.key, mw.moderator, h.RejectSubmission)
}

// submissionResponse is the public shape of a job submission. submitted_by is omitted
// (ownership, internal); submitter_email is set only on the moderator queue. The content
// fields mirror the submit body so a user can see exactly what they sent.
type submissionResponse struct {
	ID             int64      `json:"id"`
	URL            string     `json:"url"`
	Source         string     `json:"source,omitempty"`
	Title          string     `json:"title"`
	Company        string     `json:"company"`
	Location       string     `json:"location,omitempty"`
	Remote         bool       `json:"remote"`
	Description    string     `json:"description,omitempty"`
	PostedAt       *time.Time `json:"posted_at,omitempty"`
	Status         string     `json:"status"`
	ReviewReason   string     `json:"review_reason,omitempty"`
	ReviewedAt     *time.Time `json:"reviewed_at,omitempty"`
	CreatedAt      *time.Time `json:"created_at"`
	SubmitterEmail string     `json:"submitter_email,omitempty"`
	// JobSlug is the public slug of the minted live vacancy, set only on an approved
	// submission in the "my submissions" view, so the UI can link to /jobs/<slug>.
	JobSlug string `json:"job_slug,omitempty"`

	// The structured facets the submitter stated, echoed back so the submit confirmation
	// and the moderator queue show exactly what was entered.
	Skills         []string `json:"skills,omitempty"`
	Regions        []string `json:"regions,omitempty"`
	Cities         []string `json:"cities,omitempty"`
	WorkMode       string   `json:"work_mode,omitempty"`
	EmploymentType string   `json:"employment_type,omitempty"`
	Seniority      string   `json:"seniority,omitempty"`
	SalaryMin      *int     `json:"salary_min,omitempty"`
	SalaryMax      *int     `json:"salary_max,omitempty"`
	SalaryCurrency string   `json:"salary_currency,omitempty"`
	SalaryPeriod   string   `json:"salary_period,omitempty"`
}

// withStructured copies the submission's structured facets onto the response — the shared
// tail of all three mappers, which build the same-named fields from the embedded Submission.
func withStructured(resp submissionResponse, s submission.Submission) submissionResponse {
	resp.Skills = s.Skills
	resp.Regions = s.Regions
	resp.Cities = s.Cities
	resp.WorkMode = s.WorkMode
	resp.EmploymentType = s.EmploymentType
	resp.Seniority = s.Seniority
	resp.SalaryMin = s.SalaryMin
	resp.SalaryMax = s.SalaryMax
	resp.SalaryCurrency = s.SalaryCurrency
	resp.SalaryPeriod = s.SalaryPeriod
	return resp
}

// toSubmissionResponse maps a stored submission to its wire shape (no submitter email).
func toSubmissionResponse(s submission.Submission) submissionResponse {
	return withStructured(submissionResponse{
		ID:           s.ID,
		URL:          s.URL,
		Source:       s.Source,
		Title:        s.Title,
		Company:      s.Company,
		Location:     s.Location,
		Remote:       s.Remote,
		Description:  s.Description,
		PostedAt:     s.PostedAt,
		Status:       s.Status,
		ReviewReason: s.ReviewReason,
		ReviewedAt:   s.ReviewedAt,
		CreatedAt:    s.CreatedAt,
	}, s)
}

// toPendingSubmissionResponse maps a moderator-queue row, adding the submitter's email.
func toPendingSubmissionResponse(r submission.PendingSubmission) submissionResponse {
	return withStructured(submissionResponse{
		ID:             r.ID,
		URL:            r.URL,
		Source:         r.Source,
		Title:          r.Title,
		Company:        r.Company,
		Location:       r.Location,
		Remote:         r.Remote,
		Description:    r.Description,
		PostedAt:       r.PostedAt,
		Status:         r.Status,
		ReviewReason:   r.ReviewReason,
		ReviewedAt:     r.ReviewedAt,
		CreatedAt:      r.CreatedAt,
		SubmitterEmail: r.SubmitterEmail,
	}, r.Submission)
}

// toMySubmissionResponse maps a "my submissions" row, adding the minted job's slug when
// the submission was approved (empty otherwise).
func toMySubmissionResponse(r submission.UserSubmission) submissionResponse {
	return withStructured(submissionResponse{
		ID:           r.ID,
		URL:          r.URL,
		Source:       r.Source,
		Title:        r.Title,
		Company:      r.Company,
		Location:     r.Location,
		Remote:       r.Remote,
		Description:  r.Description,
		PostedAt:     r.PostedAt,
		Status:       r.Status,
		ReviewReason: r.ReviewReason,
		ReviewedAt:   r.ReviewedAt,
		CreatedAt:    r.CreatedAt,
		JobSlug:      r.JobSlug,
	}, r.Submission)
}

// submissionError maps the submission sentinels onto HTTP statuses. moderation.ErrInvalid
// (raised by the shared content validation) carries a user-facing 400 message; the rest
// map to not-found / conflict. Anything else falls through to RenderError as a 500.
func submissionError(err error) error {
	switch {
	case errors.Is(err, submission.ErrSubmissionNotFound):
		return fiber.NewError(fiber.StatusNotFound, "submission not found")
	case errors.Is(err, submission.ErrDuplicatePending):
		return fiber.NewError(fiber.StatusConflict, "a pending submission for this URL already exists")
	case errors.Is(err, submission.ErrAlreadyDecided):
		return fiber.NewError(fiber.StatusConflict, "submission already decided")
	case errors.Is(err, moderation.ErrInvalid):
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	default:
		return err
	}
}

// CreateSubmission queues a user-contributed vacancy for moderation. Authenticated by
// cookie or API key; the content is validated by the service (a bad body is a 400 before
// any write), and a duplicate pending URL is a 409. Returns the pending submission with 201.
func (h *submissionHandlers) CreateSubmission(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}

	var in createJobRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}

	sub, err := h.submission.Submit(c.Context(), userID, in.toCreateInput())
	if err != nil {
		return submissionError(err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": toSubmissionResponse(sub)})
}

// prefillRequest is the prefill body: a job URL and nothing else.
type prefillRequest struct {
	URL string `json:"url"`
}

// prefillResponse is what freehire could parse from the URL, for the submitter to review
// and edit before submitting — never persisted, never awarded a credit. Skills is
// included only when the platform states it in a STRUCTURED field (sources.Job.Skills;
// most adapters leave it empty, since freehire's own skilltag dictionary over the
// description usually does better than a source page's own tagging). Cities has no
// structured source equivalent and is never included.
type prefillResponse struct {
	Title          string   `json:"title,omitempty"`
	Company        string   `json:"company,omitempty"`
	Location       string   `json:"location,omitempty"`
	Description    string   `json:"description,omitempty"`
	WorkMode       string   `json:"work_mode,omitempty"`
	EmploymentType string   `json:"employment_type,omitempty"`
	Seniority      string   `json:"seniority,omitempty"`
	Skills         []string `json:"skills,omitempty"`
	Source         string   `json:"source,omitempty"`
}

// PrefillSubmission parses a job URL through the same destination-recognition registry
// the paste-a-link contribution flow resolves through (internal/linksource), but writes
// nothing — no job, no submission, no dedup check, no credit reward, no enrichment
// enqueue, no search push. An unrecognized URL, or one that is not a single vacancy page,
// is not an error: it responds 200 with every field empty, and the submitter keeps typing.
func (h *submissionHandlers) PrefillSubmission(c *fiber.Ctx) error {
	if _, err := requireUserID(c); err != nil {
		return err
	}

	var in prefillRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}

	resolved, ok, err := h.importer.Resolve(c.Context(), in.URL, linkimport.Board{})
	if err != nil || !ok {
		return c.JSON(fiber.Map{"data": prefillResponse{}})
	}
	return c.JSON(fiber.Map{"data": prefillResponse{
		Title:          resolved.Job.Title,
		Company:        resolved.Job.Company,
		Location:       resolved.Job.Location,
		Description:    resolved.Job.Description,
		WorkMode:       resolved.Job.WorkMode,
		EmploymentType: resolved.Job.EmploymentType,
		Seniority:      resolved.Job.Seniority,
		Skills:         resolved.Job.Skills,
		Source:         resolved.Source,
	}})
}

// ListMySubmissions returns the caller's own submissions with their status and any
// rejection reason. Scoped to the authenticated user, so it never reveals another user's.
func (h *submissionHandlers) ListMySubmissions(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}

	subs, err := h.submission.ListMine(c.Context(), userID)
	if err != nil {
		return err
	}
	out := make([]submissionResponse, len(subs))
	for i, s := range subs {
		out[i] = toMySubmissionResponse(s)
	}
	return c.JSON(fiber.Map{"data": out})
}

// ListPendingSubmissions returns the moderator review queue (with submitter emails). The
// route is role-gated, so reaching this handler already implies a moderator.
func (h *submissionHandlers) ListPendingSubmissions(c *fiber.Ctx) error {
	rows, err := h.submission.ListPending(c.Context())
	if err != nil {
		return err
	}
	out := make([]submissionResponse, len(rows))
	for i, r := range rows {
		out[i] = toPendingSubmissionResponse(r)
	}
	return c.JSON(fiber.Map{"data": out})
}

// ApproveSubmission mints a live vacancy from a pending submission and marks it approved.
// Role-gated. An unknown id is a 404; a submission already decided is a 409.
func (h *submissionHandlers) ApproveSubmission(c *fiber.Ctx) error {
	reviewerID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := pathID(c)
	if err != nil {
		return err
	}

	sub, err := h.submission.Approve(c.Context(), reviewerID, id)
	if err != nil {
		return submissionError(err)
	}
	return c.JSON(fiber.Map{"data": toSubmissionResponse(sub)})
}

// rejectRequest is the optional rejection reason body.
type rejectRequest struct {
	Reason string `json:"reason"`
}

// RejectSubmission marks a pending submission rejected with an optional reason. Role-gated.
// The reason body is optional, so a parse failure (e.g. empty body) leaves the reason blank
// rather than rejecting the request.
func (h *submissionHandlers) RejectSubmission(c *fiber.Ctx) error {
	reviewerID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := pathID(c)
	if err != nil {
		return err
	}

	var in rejectRequest
	_ = c.BodyParser(&in)

	sub, err := h.submission.Reject(c.Context(), reviewerID, id, in.Reason)
	if err != nil {
		return submissionError(err)
	}
	return c.JSON(fiber.Map{"data": toSubmissionResponse(sub)})
}
