package handler

import (
	"context"
	"errors"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/engage/processreport"
)

// companyProcessReportHandlers serve candidate-reported facts about how a company
// hires — today only that it screens with an AI interviewer. The use cases live in
// processreport.Service; these handlers translate wire ↔ domain and delegate.
//
// They are addressed by COMPANY slug even though the report is filed from a job page,
// because that is what the report is about. Routing it through the job report endpoint
// would make a route documented as storing a pending moderation report against a job
// do neither of those things.
type companyProcessReportHandlers struct {
	reports *processreport.Service
}

func newCompanyProcessReportHandlers(svc *processreport.Service) *companyProcessReportHandlers {
	return &companyProcessReportHandlers{reports: svc}
}

func (h *companyProcessReportHandlers) register(api fiber.Router, mw middleware, contract fiber.Handler) {
	// Cookie-only, like every other authored claim about an employer: this is a
	// statement attributed to a person, not the single-bit action a leaked API key is
	// trusted with. There is no public read — the COUNT is public and travels on the
	// job and company payloads; what is behind auth is only "did I report this",
	// which is the caller's own state.
	api.Get("/companies/:slug/process-reports/mine", mw.cookie, contract)
	api.Post("/companies/:slug/process-reports", mw.cookie, contract)
	api.Delete("/companies/:slug/process-reports", mw.cookie, contract)
}

// FileProcessReport records that the caller met this practice at this company.
// 201 on success, 409 if they already reported it, 400 for an unknown kind,
// 404 for an unknown company.
func (h *companyProcessReportHandlers) FileProcessReport(ctx context.Context, req *apigen.ProcessReportRequest, params apigen.FileProcessReportParams) (*apigen.FileProcessReportCreated, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	kind := req.Kind.Value
	count, err := h.reports.File(ctx, userID, params.Slug, kind)
	if err != nil {
		return nil, companyProcessReportError(err)
	}
	return &apigen.FileProcessReportCreated{Data: apigen.ProcessReport{Kind: kind, Count: int64(count)}}, nil
}

// RetractProcessReport withdraws the caller's own report. The kind rides as a query
// param rather than a body, the same shape DeleteFeedback uses for its category.
// 200 on success, 404 if they never filed one or already withdrew it.
func (h *companyProcessReportHandlers) RetractProcessReport(ctx context.Context, params apigen.RetractProcessReportParams) (*apigen.RetractProcessReportOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	kind := params.Kind.Value
	count, err := h.reports.Retract(ctx, userID, params.Slug, kind)
	if err != nil {
		return nil, companyProcessReportError(err)
	}
	return &apigen.RetractProcessReportOK{Data: apigen.ProcessReport{Kind: kind, Count: int64(count)}}, nil
}

// MyProcessReports lists the kinds the caller currently has live against this company,
// so the write surface opens in the right state instead of finding out by being
// refused. Always an array, never null.
func (h *companyProcessReportHandlers) MyProcessReports(ctx context.Context, params apigen.MyProcessReportsParams) (*apigen.MyProcessReportsOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	kinds, err := h.reports.Mine(ctx, userID, params.Slug)
	if err != nil {
		return nil, companyProcessReportError(err)
	}
	return &apigen.MyProcessReportsOK{Data: kinds}, nil
}

// companyProcessReportError maps the service's sentinels onto statuses. The default
// case passes an unrecognised error through untouched, so an infrastructure failure
// never renders as a client mistake.
func companyProcessReportError(err error) error {
	switch {
	case errors.Is(err, processreport.ErrCompanyNotFound), errors.Is(err, processreport.ErrNotFound):
		return fiber.NewError(fiber.StatusNotFound, "not found")
	case errors.Is(err, processreport.ErrInvalidKind):
		return fiber.NewError(fiber.StatusBadRequest, "invalid kind")
	case errors.Is(err, processreport.ErrAlreadyReported):
		return fiber.NewError(fiber.StatusConflict, "you already reported this")
	default:
		return err
	}
}
