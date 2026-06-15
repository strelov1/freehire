package handler

import (
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/report"
)

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
}

// toReportResponse maps a stored report to its wire shape (no reporter email or job fields).
func toReportResponse(r db.JobReport) reportResponse {
	return reportResponse{
		ID:              r.ID,
		Reason:          r.Reason,
		Details:         r.Details,
		ContactTelegram: r.ContactTelegram,
		Status:          r.Status,
		ReviewReason:    r.ReviewReason,
		ReviewedAt:      timePtr(r.ReviewedAt),
		CreatedAt:       timePtr(r.CreatedAt),
	}
}

// toPendingReportResponse maps a moderator-queue row, adding the reporter's email and the
// reported job's slug and title.
func toPendingReportResponse(r db.ListPendingReportsRow) reportResponse {
	return reportResponse{
		ID:              r.ID,
		Reason:          r.Reason,
		Details:         r.Details,
		ContactTelegram: r.ContactTelegram,
		Status:          r.Status,
		ReviewReason:    r.ReviewReason,
		ReviewedAt:      timePtr(r.ReviewedAt),
		CreatedAt:       timePtr(r.CreatedAt),
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
func (a *API) CreateReport(c *fiber.Ctx) error {
	userID, ok := auth.UserID(c)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}

	jobID, err := a.queries.GetJobIDBySlug(c.Context(), c.Params("slug"))
	if err != nil {
		return err // pgx.ErrNoRows → 404 in RenderError
	}

	var in createReportRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}

	rep, err := a.report.File(c.Context(), userID, jobID, report.FileInput{
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
func (a *API) ListPendingReports(c *fiber.Ctx) error {
	rows, err := a.report.ListPending(c.Context())
	if err != nil {
		return err
	}
	out := make([]reportResponse, len(rows))
	for i, r := range rows {
		out[i] = toPendingReportResponse(r)
	}
	return c.JSON(fiber.Map{"data": out})
}

// resolveReportRequest is the optional resolve body: whether to also close the reported job.
type resolveReportRequest struct {
	CloseJob bool `json:"close_job"`
}

// ResolveReport marks a pending report resolved, optionally soft-closing the reported job.
// Role-gated. An unknown id is a 404; a report already decided is a 409. The body is
// optional (a parse failure leaves close_job false).
func (a *API) ResolveReport(c *fiber.Ctx) error {
	reviewerID, ok := auth.UserID(c)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid report id")
	}

	var in resolveReportRequest
	_ = c.BodyParser(&in)

	rep, err := a.report.Resolve(c.Context(), reviewerID, int64(id), in.CloseJob)
	if err != nil {
		return reportError(err)
	}
	return c.JSON(fiber.Map{"data": toReportResponse(rep)})
}

// dismissReportRequest is the optional dismissal reason body.
type dismissReportRequest struct {
	Reason string `json:"reason"`
}

// DismissReport marks a pending report dismissed with an optional reason, leaving the job
// unchanged. Role-gated. The reason body is optional, so a parse failure leaves it blank.
func (a *API) DismissReport(c *fiber.Ctx) error {
	reviewerID, ok := auth.UserID(c)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid report id")
	}

	var in dismissReportRequest
	_ = c.BodyParser(&in)

	rep, err := a.report.Dismiss(c.Context(), reviewerID, int64(id), in.Reason)
	if err != nil {
		return reportError(err)
	}
	return c.JSON(fiber.Map{"data": toReportResponse(rep)})
}
