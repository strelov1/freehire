package handler

import (
	"context"
	"errors"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/engage/report"
	"github.com/strelov1/freehire/internal/platform/db"
)

// reportHandlers serves the job-report moderation queue (file/list/resolve/dismiss);
// resolving may soft-close the reported job through the job-lifecycle close path.
type reportHandlers struct {
	report  *report.Service
	queries *db.Queries
}

func newReportHandlers(queries *db.Queries) *reportHandlers {
	// One repository serves both report.Repository and report.JobCloser.
	repo := report.NewQueriesRepository(queries)
	return &reportHandlers{report: report.New(repo, repo), queries: queries}
}

func (h *reportHandlers) register(api fiber.Router, mw middleware, contract fiber.Handler) {
	// Any authenticated user (cookie or key) may file; the review actions are moderator-gated.
	api.Post("/jobs/:slug/reports", mw.key, contract)
	api.Get("/reports", mw.key, mw.moderator, contract)
	api.Post("/reports/:id/resolve", mw.key, mw.moderator, contract)
	api.Post("/reports/:id/dismiss", mw.key, mw.moderator, contract)
}

// toJobReport maps a stored report to its wire shape: no reporter email, job fields or reported_by.
func toJobReport(r report.Report) apigen.JobReport {
	out := apigen.JobReport{
		ID:              r.ID,
		Reason:          apigen.JobReportReason(r.Reason),
		Details:         r.Details,
		ContactTelegram: optString(r.ContactTelegram),
		Status:          apigen.JobReportStatus(r.Status),
		ReviewReason:    optString(r.ReviewReason),
	}
	if r.ReviewedAt != nil {
		out.ReviewedAt = apigen.NewOptDateTime(*r.ReviewedAt)
	}
	if r.CreatedAt != nil {
		out.CreatedAt = apigen.NewNilDateTime(*r.CreatedAt)
	} else {
		out.CreatedAt.SetToNull()
	}
	return out
}

// toJobReportDetail maps a report with its context, adding the reporter's email and the
// reported job's slug and title for the moderator queue.
func toJobReportDetail(r report.ReportDetail) apigen.JobReport {
	out := toJobReport(report.Report{
		ID: r.ID, Reason: r.Reason, Details: r.Details, ContactTelegram: r.ContactTelegram,
		Status: r.Status, ReviewReason: r.ReviewReason, ReviewedAt: r.ReviewedAt, CreatedAt: r.CreatedAt,
	})
	out.ReporterEmail = optString(r.ReporterEmail)
	out.JobSlug = optString(r.JobSlug)
	out.JobTitle = optString(r.JobTitle)
	return out
}

// toJobReportDecision maps a moderator decision, adding whether the reporter was reached. The
// flag is set only here, so the create and queue shapes omit it rather than claim no notice went out.
func toJobReportDecision(rev report.Review) apigen.JobReport {
	out := toJobReport(rev.Report)
	out.Notified = apigen.NewOptBool(rev.Notified)
	return out
}

// reportError maps the report sentinels onto HTTP statuses; report.ErrInvalid carries a
// user-facing 400 message, anything else falls through as a 500.
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

// CreateJobReport files a complaint about the job named by :slug. A second open report of the
// same job by the same user is a 409.
func (h *reportHandlers) CreateJobReport(ctx context.Context, req *apigen.JobReportRequest, params apigen.CreateJobReportParams) (*apigen.CreateJobReportCreated, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	jobID, err := h.queries.GetJobIDBySlug(ctx, db.GetJobIDBySlugParams{PublicSlug: params.Slug, CallerID: userID})
	if err != nil {
		return nil, err // pgx.ErrNoRows → 404 in RenderError
	}

	rep, err := h.report.File(ctx, userID, jobID, report.FileInput{
		Reason:          req.Reason.Value,
		Details:         req.Details.Value,
		ContactTelegram: req.ContactTelegram.Value,
	})
	if err != nil {
		return nil, reportError(err)
	}
	return &apigen.CreateJobReportCreated{Data: toJobReport(rep)}, nil
}

// ListPendingJobReports returns the moderator review queue (role-gated at the route).
func (h *reportHandlers) ListPendingJobReports(ctx context.Context) (*apigen.ListPendingJobReportsOK, error) {
	rows, err := h.report.ListPending(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]apigen.JobReport, len(rows))
	for i, r := range rows {
		out[i] = toJobReportDetail(r)
	}
	return &apigen.ListPendingJobReportsOK{Data: out}, nil
}

// ResolveJobReport marks a pending report resolved, optionally soft-closing the reported job.
// The body is optional: close_job and notify_reporter default to false.
func (h *reportHandlers) ResolveJobReport(ctx context.Context, req apigen.OptResolveJobReportRequest, params apigen.ResolveJobReportParams) (*apigen.ResolveJobReportOK, error) {
	reviewerID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	id, err := contractPathID(params.ID)
	if err != nil {
		return nil, err
	}

	rev, err := h.report.Resolve(ctx, reviewerID, id, report.ResolveInput{
		CloseJob:       req.Value.CloseJob.Value,
		NotifyReporter: req.Value.NotifyReporter.Value,
		Note:           req.Value.Note.Value,
	})
	if err != nil {
		return nil, reportError(err)
	}
	return &apigen.ResolveJobReportOK{Data: toJobReportDecision(rev)}, nil
}

// DismissJobReport marks a pending report dismissed with an optional reason, leaving the job unchanged.
func (h *reportHandlers) DismissJobReport(ctx context.Context, req apigen.OptDismissJobReportRequest, params apigen.DismissJobReportParams) (*apigen.DismissJobReportOK, error) {
	reviewerID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	id, err := contractPathID(params.ID)
	if err != nil {
		return nil, err
	}

	rev, err := h.report.Dismiss(ctx, reviewerID, id, report.DismissInput{
		NotifyReporter: req.Value.NotifyReporter.Value,
		Reason:         req.Value.Reason.Value,
	})
	if err != nil {
		return nil, reportError(err)
	}
	return &apigen.DismissJobReportOK{Data: toJobReportDecision(rev)}, nil
}
