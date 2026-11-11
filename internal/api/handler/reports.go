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
	// The report queue uses one QueriesRepository for both persistence and the
	// job soft-close (it implements report.Repository and report.JobCloser).
	repo := report.NewQueriesRepository(queries)
	return &reportHandlers{report: report.New(repo, repo), queries: queries}
}

func (h *reportHandlers) register(api fiber.Router, mw middleware, contract fiber.Handler) {
	// Job reports: any authenticated user flags a problem with a live vacancy (cookie or
	// API key), addressed by the job's public slug. The review actions (the pending queue,
	// resolve, dismiss) are moderator-gated; resolve may soft-close the reported job.
	api.Post("/jobs/:slug/reports", mw.key, contract)
	api.Get("/reports", mw.key, mw.moderator, contract)
	api.Post("/reports/:id/resolve", mw.key, mw.moderator, contract)
	api.Post("/reports/:id/dismiss", mw.key, mw.moderator, contract)
}

// toJobReport maps a stored report to its wire shape (no reporter email or job fields).
// reported_by is never sent (ownership, internal).
func toJobReport(r report.Report) apigen.JobReport {
	out := apigen.JobReport{
		ID:              r.ID,
		Reason:          r.Reason,
		Details:         r.Details,
		ContactTelegram: optString(r.ContactTelegram),
		Status:          r.Status,
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

// CreateJobReport files a complaint about the job named by :slug. Authenticated by cookie or
// API key; the slug is resolved to the internal id (a miss is a 404 via RenderError) before
// any write, the content is validated by the service (a bad body is a 400), and a second
// open report of the same job by the same user is a 409. Returns the pending report with 201.
func (h *reportHandlers) CreateJobReport(ctx context.Context, req *apigen.JobReportRequest, params apigen.CreateJobReportParams) (*apigen.CreateJobReportCreated, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	jobID, err := h.queries.GetJobIDBySlug(ctx, params.Slug)
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

// ListPendingJobReports returns the moderator review queue (with reporter email and job
// slug/title). The route is role-gated, so reaching this handler already implies a moderator.
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
// Role-gated. An unknown id is a 404; a report already decided is a 409. The body is
// optional: close_job and notify_reporter default to false on absence, so a caller that wants
// the reporter contacted says so explicitly.
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

// DismissJobReport marks a pending report dismissed with an optional reason, leaving the job
// unchanged. Role-gated. The body is optional, so a missing one leaves the reason blank.
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
