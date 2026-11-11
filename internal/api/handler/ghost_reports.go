package handler

import (
	"context"
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/job/ghostreport"
	"github.com/strelov1/freehire/internal/platform/db"
)

// ghostReportHandlers serves the ghost-signal evidence channel: one person states they
// applied to a posting and were never answered, and may withdraw that statement.
//
// Separate from reportHandlers on purpose. That queue reaches a moderator whose lever is
// closing the job; nothing here reaches a moderator and nothing here closes anything.
type ghostReportHandlers struct {
	reports *ghostreport.Service
	queries *db.Queries
}

func newGhostReportHandlers(queries *db.Queries) *ghostReportHandlers {
	return &ghostReportHandlers{
		reports: ghostreport.New(ghostreport.NewQueriesRepository(queries)),
		queries: queries,
	}
}

func (h *ghostReportHandlers) register(api fiber.Router, mw middleware, contract fiber.Handler) {
	// Cookie or full-scope API key, like the rest of the per-user surface, so a user's
	// own agent harness can file and withdraw on their behalf.
	api.Post("/jobs/:slug/ghost-report", mw.key, contract)
	api.Delete("/jobs/:slug/ghost-report", mw.key, contract)
}

// appliedOnLayout is a plain calendar date. The reporter is stating a day, not an
// instant, and asking a browser for a timezone-bearing timestamp would invite a claim
// that reads as tomorrow in one zone and today in another.
const appliedOnLayout = "2006-01-02"

// CreateGhostReport files the caller's claim about the job named by :slug. The slug is
// resolved before any write (a miss is a 404 via RenderError); an unparseable or
// out-of-range date is a 400; an unproven address is a 403; a closed job or a claim the
// caller already has is a 409.
func (h *ghostReportHandlers) CreateGhostReport(ctx context.Context, req *apigen.GhostReportRequest, params apigen.CreateGhostReportParams) (*apigen.CreateGhostReportCreated, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	jobID, err := h.queries.GetJobIDBySlug(ctx, params.Slug)
	if err != nil {
		return nil, err // pgx.ErrNoRows → 404 in classify
	}

	appliedOn, err := time.Parse(appliedOnLayout, req.AppliedOn.Value)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusBadRequest, "applied_on must be a date like 2026-07-29")
	}

	rep, err := h.reports.File(ctx, userID, jobID, appliedOn, time.Now().UTC())
	if err != nil {
		return nil, ghostReportError(err)
	}
	return &apigen.CreateGhostReportCreated{Data: ghostReportOut(rep)}, nil
}

// RetractGhostReport withdraws the caller's claim about :slug. A claim that is absent or
// already withdrawn is a 404.
//
// It answers 204, not 200: Fiber's SendStatus(200) writes the body "OK", which breaks a
// JSON client on a call that in fact succeeded.
func (h *ghostReportHandlers) RetractGhostReport(ctx context.Context, params apigen.RetractGhostReportParams) error {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return err
	}
	jobID, err := h.queries.GetJobIDBySlug(ctx, params.Slug)
	if err != nil {
		return err
	}
	return ghostReportError(h.reports.Retract(ctx, userID, jobID))
}

// ghostReportError maps the service sentinels onto HTTP statuses.
func ghostReportError(err error) error {
	switch {
	case errors.Is(err, ghostreport.ErrInvalid):
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	case errors.Is(err, ghostreport.ErrUnverified):
		return fiber.NewError(fiber.StatusForbidden,
			"confirm your email address before reporting a job")
	case errors.Is(err, ghostreport.ErrJobClosed):
		return fiber.NewError(fiber.StatusConflict, "this job is already closed")
	case errors.Is(err, ghostreport.ErrDuplicate):
		return fiber.NewError(fiber.StatusConflict, "you have already reported this job")
	case errors.Is(err, ghostreport.ErrNotFound):
		return fiber.NewError(fiber.StatusNotFound, "no report to withdraw")
	default:
		return err
	}
}

func ghostReportOut(r ghostreport.Report) apigen.GhostReport {
	out := apigen.GhostReport{JobID: r.JobID, AppliedOn: r.AppliedOn.Format(appliedOnLayout)}
	if r.CreatedAt != nil {
		out.CreatedAt = apigen.NewOptDateTime(r.CreatedAt.UTC())
	}
	return out
}
