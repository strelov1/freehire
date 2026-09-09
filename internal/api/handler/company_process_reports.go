package handler

import (
	"errors"

	"github.com/gofiber/fiber/v2"

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

func (h *companyProcessReportHandlers) register(api fiber.Router, mw middleware) {
	// Cookie-only, like every other authored claim about an employer: this is a
	// statement attributed to a person, not the single-bit action a leaked API key is
	// trusted with. There is no public read — the COUNT is public and travels on the
	// job and company payloads; what is behind auth is only "did I report this",
	// which is the caller's own state.
	api.Get("/companies/:slug/process-reports/mine", mw.cookie, h.MyProcessReports)
	api.Post("/companies/:slug/process-reports", mw.cookie, h.FileProcessReport)
	api.Delete("/companies/:slug/process-reports", mw.cookie, h.RetractProcessReport)
}

// fileProcessReportBody is the POST payload. Only a kind: there is nothing to
// elaborate, since the kind is the whole claim.
type fileProcessReportBody struct {
	Kind string `json:"kind"`
}

// processReportResponse carries the kind acted on and the company's resulting count.
// The count comes back on every write so the caller can render the badge from the
// answer it already has, rather than re-reading the company.
type processReportResponse struct {
	Kind  string `json:"kind"`
	Count int32  `json:"count"`
}

// FileProcessReport records that the caller met this practice at this company.
// 201 on success, 409 if they already reported it, 400 for an unknown kind,
// 404 for an unknown company, 429 over the daily cap.
func (h *companyProcessReportHandlers) FileProcessReport(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	var in fileProcessReportBody
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	count, err := h.reports.File(c.Context(), userID, c.Params("slug"), in.Kind)
	if err != nil {
		return companyProcessReportError(err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"data": processReportResponse{Kind: in.Kind, Count: count},
	})
}

// RetractProcessReport withdraws the caller's own report. The kind rides as a query
// param rather than a body, the same shape DeleteFeedback uses for its category.
// 200 on success, 404 if they never filed one or already withdrew it.
func (h *companyProcessReportHandlers) RetractProcessReport(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	kind := c.Query("kind")
	count, err := h.reports.Retract(c.Context(), userID, c.Params("slug"), kind)
	if err != nil {
		return companyProcessReportError(err)
	}
	return c.JSON(fiber.Map{"data": processReportResponse{Kind: kind, Count: count}})
}

// MyProcessReports lists the kinds the caller currently has live against this company,
// so the write surface opens in the right state instead of finding out by being
// refused. Always an array, never null.
func (h *companyProcessReportHandlers) MyProcessReports(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	kinds, err := h.reports.Mine(c.Context(), userID, c.Params("slug"))
	if err != nil {
		return companyProcessReportError(err)
	}
	return c.JSON(fiber.Map{"data": kinds})
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
	case errors.Is(err, processreport.ErrRateLimited):
		return fiber.NewError(fiber.StatusTooManyRequests, "that's a lot of reports today — try again tomorrow")
	default:
		return err
	}
}
