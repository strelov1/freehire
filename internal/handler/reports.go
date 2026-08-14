package handler

import (
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/report"
)

// reportHandlers serves the job-report moderation queue (file/list/resolve/dismiss);
// resolving may soft-close the reported job through the job-lifecycle close path.
type reportHandlers struct {
	report  *report.Service
	queries *db.Queries
}

func newReportHandlers(queries *db.Queries) *reportHandlers {
	// The report queue uses one QueriesRepository for both persistence and the
	// job soft-close (it implements report.Repository and report.JobCloser).
	repo := report.NewQueriesRepository(queries)
	return &reportHandlers{report: report.New(repo, repo), queries: queries}
}

func (h *reportHandlers) register(api fiber.Router, mw middleware) {
	// Job reports: any authenticated user flags a problem with a live vacancy (cookie or
	// API key), addressed by the job's public slug. The review actions (the pending queue,
	// resolve, dismiss) are moderator-gated; resolve may soft-close the reported job.
	api.Post("/jobs/:slug/reports", mw.key, h.CreateReport)
	api.Get("/reports", mw.key, mw.moderator, h.ListPendingReports)
	api.Post("/reports/:id/resolve", mw.key, mw.moderator, h.ResolveReport)
	api.Post("/reports/:id/dismiss", mw.key, mw.moderator, h.DismissReport)
}

// reportResponse is the public shape of a job report. reported_by is omitted (ownership,
// internal); reporter_email and job_slug/job_title are set only on the moderator queue so
// the reviewer can judge the report and link to the vacancy.
type reportResponse struct {
	ID              int64      `json:"id"`
	Reason          string     `json:"reason"`
	Details         string     `json:"details"`
	ContactTelegram string     `json:"contact_telegram,omitempty"`
	Status          string     `json:"status"`
	ReviewReason    string     `json:"review_reason,omitempty"`
	ReviewedAt      *time.Time `json:"reviewed_at,omitempty"`
	CreatedAt       *time.Time `json:"created_at"`
	ReporterEmail   string     `json:"reporter_email,omitempty"`
	JobSlug         string     `json:"job_slug,omitempty"`
	JobTitle        string     `json:"job_title,omitempty"`
	// Notified is set only on a decision response: whether the reporter was actually
	// told. It is a pointer so the create and queue shapes omit it entirely rather than
	// claiming a notice was never sent.
	Notified *bool `json:"notified,omitempty"`
}

// toDecisionResponse maps a moderator decision, adding whether the reporter was reached.
func toDecisionResponse(rev report.Review) reportResponse {
	out := toReportResponse(rev.Report)
	out.Notified = &rev.Notified
	return out
}

// toReportResponse maps a stored report to its wire shape (no reporter email or job fields).
func toReportResponse(r report.Report) reportResponse {
	return reportResponse{
		ID:              r.ID,
		Reason:          r.Reason,
		Details:         r.Details,
		ContactTelegram: r.ContactTelegram,
		Status:          r.Status,
		ReviewReason:    r.ReviewReason,
		ReviewedAt:      r.ReviewedAt,
		CreatedAt:       r.CreatedAt,
	}
}

// toReportDetailResponse maps a report with its context, adding the reporter's email and the
// reported job's slug and title.
func toReportDetailResponse(r report.ReportDetail) reportResponse {
	return reportResponse{
		ID:              r.ID,
		Reason:          r.Reason,
		Details:         r.Details,
		ContactTelegram: r.ContactTelegram,
		Status:          r.Status,
		ReviewReason:    r.ReviewReason,
		ReviewedAt:      r.ReviewedAt,
		CreatedAt:       r.CreatedAt,
		ReporterEmail:   r.ReporterEmail,
		JobSlug:         r.JobSlug,
		JobTitle:        r.JobTitle,
	}
}

// reportError maps the report sentinels onto HTTP statuses. report.ErrInvalid carries a
// user-facing 400 message; the rest map to not-found / conflict. Anything else falls
// through to RenderError as a 500.
func reportError(err error) error {
	switch {
	case errors.Is(err, report.ErrReportNotFound):
		return fiber.NewError(fiber.StatusNotFound, "report not found")
	case errors.Is(err, report.ErrDuplicateOpen):
		return fiber.NewError(fiber.StatusConflict, "you already have an open report for this job")
	case errors.Is(err, report.ErrAlreadyDecided):
		return fiber.NewError(fiber.StatusConflict, "report already decided")
	case errors.Is(err, report.ErrInvalid):
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	case errors.Is(err, report.ErrRateLimited):
		return fiber.NewError(fiber.StatusTooManyRequests, "too many reports today")
	default:
		return err
	}
}

// createReportRequest is the report body: a reason from the controlled vocabulary, required
// details, and an optional Telegram contact.
type createReportRequest struct {
	Reason          string `json:"reason"`
	Details         string `json:"details"`
	ContactTelegram string `json:"contact_telegram"`
}

// CreateReport files a complaint about the job named by :slug. Authenticated by cookie or
// API key; the slug is resolved to the internal id (a miss is a 404 via RenderError) before
// any write, the content is validated by the service (a bad body is a 400), and a second
// open report of the same job by the same user is a 409. Returns the pending report with 201.
func (h *reportHandlers) CreateReport(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}

	jobID, err := h.queries.GetJobIDBySlug(c.Context(), c.Params("slug"))
	if err != nil {
		return err // pgx.ErrNoRows → 404 in RenderError
	}

	var in createReportRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}

	rep, err := h.report.File(c.Context(), userID, jobID, report.FileInput{
		Reason:          in.Reason,
		Details:         in.Details,
		ContactTelegram: in.ContactTelegram,
	})
	if err != nil {
		return reportError(err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": toReportResponse(rep)})
}

// ListPendingReports returns the moderator review queue (with reporter email and job
// slug/title). The route is role-gated, so reaching this handler already implies a moderator.
func (h *reportHandlers) ListPendingReports(c *fiber.Ctx) error {
	rows, err := h.report.ListPending(c.Context())
	if err != nil {
		return err
	}
	out := make([]reportResponse, len(rows))
	for i, r := range rows {
		out[i] = toReportDetailResponse(r)
	}
	return c.JSON(fiber.Map{"data": out})
}

// resolveReportRequest is the optional resolve body: whether to also close the reported job,
// the note the reporter is told, and whether to tell them at all. notify_reporter defaults
// to false on absence, so a caller that wants the reporter contacted says so explicitly.
type resolveReportRequest struct {
	CloseJob       bool   `json:"close_job"`
	Note           string `json:"note"`
	NotifyReporter bool   `json:"notify_reporter"`
}

// ResolveReport marks a pending report resolved, optionally soft-closing the reported job.
// Role-gated. An unknown id is a 404; a report already decided is a 409. The body is
// optional (a parse failure leaves close_job false).
func (h *reportHandlers) ResolveReport(c *fiber.Ctx) error {
	reviewerID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := pathID(c)
	if err != nil {
		return err
	}

	var in resolveReportRequest
	_ = c.BodyParser(&in)

	rev, err := h.report.Resolve(c.Context(), reviewerID, id, report.ResolveInput{
		CloseJob:       in.CloseJob,
		NotifyReporter: in.NotifyReporter,
		Note:           in.Note,
	})
	if err != nil {
		return reportError(err)
	}
	return c.JSON(fiber.Map{"data": toDecisionResponse(rev)})
}

// dismissReportRequest is the optional dismissal body: the reason the report changed
// nothing, and whether to tell the reporter (see resolveReportRequest on the default).
type dismissReportRequest struct {
	Reason         string `json:"reason"`
	NotifyReporter bool   `json:"notify_reporter"`
}

// DismissReport marks a pending report dismissed with an optional reason, leaving the job
// unchanged. Role-gated. The reason body is optional, so a parse failure leaves it blank.
func (h *reportHandlers) DismissReport(c *fiber.Ctx) error {
	reviewerID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := pathID(c)
	if err != nil {
		return err
	}

	var in dismissReportRequest
	_ = c.BodyParser(&in)

	rev, err := h.report.Dismiss(c.Context(), reviewerID, id, report.DismissInput{
		NotifyReporter: in.NotifyReporter,
		Reason:         in.Reason,
	})
	if err != nil {
		return reportError(err)
	}
	return c.JSON(fiber.Map{"data": toDecisionResponse(rev)})
}
