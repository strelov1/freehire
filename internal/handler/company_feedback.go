package handler

import (
	"context"
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/community"
	"github.com/strelov1/freehire/internal/companyfeedback"
)

// communityPersonas adapts community.Service to companyfeedback.PersonaSource,
// so feedback reuses the exact same site-wide persona discussion threads mint
// rather than a second identity being minted here.
type communityPersonas struct{ svc *community.Service }

func (p communityPersonas) PersonaFor(ctx context.Context, userID int64) (string, error) {
	persona, err := p.svc.PersonaFor(ctx, userID)
	if err != nil {
		return "", err
	}
	return persona.Handle, nil
}

// companyFeedbackHandlers serves company feedback: a signed-in user's 1-5 star
// rating + closed category + text about a company, shown under their site-wide
// pseudonymous persona, plus the reader-report/moderator-hide lever. The use
// cases live in companyfeedback.Service; the handlers translate wire ↔ domain
// and delegate to it.
type companyFeedbackHandlers struct {
	feedback *companyfeedback.Service
}

func newCompanyFeedbackHandlers(svc *companyfeedback.Service) *companyFeedbackHandlers {
	return &companyFeedbackHandlers{feedback: svc}
}

func (h *companyFeedbackHandlers) register(api fiber.Router, mw middleware) {
	// Reads are public — only pseudonymous persona handles are ever exposed, never
	// a user id — so feedback is browsable without signing in. Writes are
	// cookie-only, the same as thread create/reply: this is authored public
	// content, not the single-bit vote a leaked API key is trusted with.
	api.Get("/companies/:slug/feedback", h.ListFeedback)
	api.Get("/companies/:slug/feedback/mine", mw.cookie, h.GetMyFeedback)
	api.Post("/companies/:slug/feedback", mw.cookie, h.UpsertFeedback)
	api.Delete("/companies/:slug/feedback", mw.cookie, h.DeleteFeedback)
	// Reporting a specific review, and the moderator lever it feeds, are addressed
	// by the review's own id — flat routes, the same shape as /threads/:id/close —
	// rather than nested under a company slug the id doesn't need to be found.
	api.Post("/company-feedback/:id/report", mw.cookie, h.ReportFeedback)
	api.Get("/company-feedback/reported", mw.cookie, mw.moderator, h.ListReportedFeedback)
	api.Post("/company-feedback/:id/hide", mw.cookie, mw.moderator, h.HideFeedback)
}

// companyFeedbackSummaryResponse is a company's recomputed materialized
// feedback counters, carried alongside a write response so the caller can
// update its own view of the company without a second round trip — the same
// shape internal/vote.Result serves for a vote cast/clear.
type companyFeedbackSummaryResponse struct {
	Count     int32    `json:"feedback_count"`
	RatingAvg *float32 `json:"feedback_rating_avg"`
}

func toCompanyFeedbackSummaryResponse(s companyfeedback.Summary) companyFeedbackSummaryResponse {
	return companyFeedbackSummaryResponse{Count: s.Count, RatingAvg: s.RatingAvg}
}

// companyFeedbackResponse is the public shape of one feedback entry: the
// persona handle is the only author identity ever sent to a client.
type companyFeedbackResponse struct {
	ID           int64     `json:"id"`
	Author       string    `json:"author"`
	Rating       int16     `json:"rating"`
	FeedbackType string    `json:"feedback_type"`
	Body         string    `json:"body"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	// Company carries the company's freshly recomputed feedback counters — set
	// only on Upsert's response (see UpsertFeedback), so the caller can update
	// its own view of the company without a second round trip. Omitted on
	// List/Mine, where nothing just changed.
	Company *companyFeedbackSummaryResponse `json:"company,omitempty"`
}

func toCompanyFeedbackResponse(f companyfeedback.Feedback) companyFeedbackResponse {
	return companyFeedbackResponse{
		ID: f.ID, Author: personaOrDeleted(f.AuthorHandle), Rating: f.Rating,
		FeedbackType: f.FeedbackType, Body: f.Body, CreatedAt: f.CreatedAt, UpdatedAt: f.UpdatedAt,
	}
}

// reportedFeedbackResponse is the moderator queue's shape: a review plus the
// aggregated report info a moderator triages by.
type reportedFeedbackResponse struct {
	companyFeedbackResponse
	CompanySlug   string   `json:"company_slug"`
	ReportCount   int64    `json:"report_count"`
	ReportReasons []string `json:"report_reasons"`
}

func toReportedFeedbackResponse(r companyfeedback.ReportedFeedback) reportedFeedbackResponse {
	return reportedFeedbackResponse{
		companyFeedbackResponse: toCompanyFeedbackResponse(r.Feedback),
		CompanySlug:             r.CompanySlug,
		ReportCount:             r.ReportCount,
		ReportReasons:           r.ReportReasons,
	}
}

type upsertCompanyFeedbackBody struct {
	Rating       int16  `json:"rating"`
	FeedbackType string `json:"feedback_type"`
	Body         string `json:"body"`
}

type reportCompanyFeedbackBody struct {
	Reason string `json:"reason"`
}

// companyFeedbackError maps companyfeedback sentinels to HTTP statuses; anything
// else falls through to RenderError's 500.
func companyFeedbackError(err error) error {
	switch {
	case errors.Is(err, companyfeedback.ErrCompanyNotFound), errors.Is(err, companyfeedback.ErrNotFound):
		return fiber.NewError(fiber.StatusNotFound, "not found")
	case errors.Is(err, companyfeedback.ErrInvalidRating):
		return fiber.NewError(fiber.StatusBadRequest, "rating must be between 1 and 5")
	case errors.Is(err, companyfeedback.ErrInvalidFeedbackType):
		return fiber.NewError(fiber.StatusBadRequest, "invalid feedback type")
	case errors.Is(err, companyfeedback.ErrInvalidReportReason):
		return fiber.NewError(fiber.StatusBadRequest, "invalid report reason")
	case errors.Is(err, companyfeedback.ErrEmptyBody):
		return fiber.NewError(fiber.StatusUnprocessableEntity, "body is required")
	case errors.Is(err, companyfeedback.ErrBodyTooLong):
		return fiber.NewError(fiber.StatusUnprocessableEntity, "feedback is too long")
	case errors.Is(err, companyfeedback.ErrRateLimited):
		return fiber.NewError(fiber.StatusTooManyRequests, "you've reviewed a lot of companies today — try again tomorrow")
	default:
		return err
	}
}

// ListFeedback returns a company's feedback, newest first, offset-paginated. Public.
func (h *companyFeedbackHandlers) ListFeedback(c *fiber.Ctx) error {
	slug := c.Params("slug")
	limit, offset := pageParams(c)
	items, err := h.feedback.List(c.Context(), slug, int32(limit), int32(offset))
	if err != nil {
		return companyFeedbackError(err)
	}
	total, err := h.feedback.Count(c.Context(), slug)
	if err != nil {
		return companyFeedbackError(err)
	}
	out := make([]companyFeedbackResponse, len(items))
	for i, f := range items {
		out[i] = toCompanyFeedbackResponse(f)
	}
	return listResponse(c, out, total, limit, offset)
}

// GetMyFeedback returns all of the caller's own feedback on a company, across
// every category they've reviewed it under (an empty array when they have not
// left any yet) — the write form's prefill and its per-category "already
// used" read, so it can only ever offer a genuinely new category alongside
// editing an existing one.
func (h *companyFeedbackHandlers) GetMyFeedback(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	items, err := h.feedback.Mine(c.Context(), userID, c.Params("slug"))
	if err != nil {
		return companyFeedbackError(err)
	}
	out := make([]companyFeedbackResponse, len(items))
	for i, f := range items {
		out[i] = toCompanyFeedbackResponse(f)
	}
	return c.JSON(fiber.Map{"data": out})
}

// UpsertFeedback creates or overwrites the caller's feedback in one category
// on a company; 400/404/422/429. The response's nested `company` field is the
// freshly recomputed counters, so the caller can update its own view of the
// company (e.g. a header badge) without a second round trip.
func (h *companyFeedbackHandlers) UpsertFeedback(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	var in upsertCompanyFeedbackBody
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	f, summary, err := h.feedback.Upsert(c.Context(), userID, c.Params("slug"), in.Rating, in.FeedbackType, in.Body)
	if err != nil {
		return companyFeedbackError(err)
	}
	out := toCompanyFeedbackResponse(f)
	companySummary := toCompanyFeedbackSummaryResponse(summary)
	out.Company = &companySummary
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": out})
}

// DeleteFeedback removes the caller's own feedback in one category
// (?feedback_type=) on a company (no-op when absent) and returns the
// company's freshly recomputed counters — the same cast/clear-returns-the-tally
// shape as VoteControl's clear endpoints, so the caller never needs a
// follow-up fetch to learn the new count.
func (h *companyFeedbackHandlers) DeleteFeedback(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	summary, err := h.feedback.Delete(c.Context(), userID, c.Params("slug"), c.Query("feedback_type"))
	if err != nil {
		return companyFeedbackError(err)
	}
	return c.JSON(fiber.Map{"data": toCompanyFeedbackSummaryResponse(summary)})
}

// ReportFeedback files a complaint against a specific review, addressed by its
// id. Any signed-in user; a second report of the same review by the same user
// is a silent no-op. 400/404.
func (h *companyFeedbackHandlers) ReportFeedback(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := pathID(c)
	if err != nil {
		return err
	}
	var in reportCompanyFeedbackBody
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if err := h.feedback.Report(c.Context(), userID, id, in.Reason); err != nil {
		return companyFeedbackError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// ListReportedFeedback returns every review with at least one report,
// most-reported first — the moderator queue's read. Role-gated.
func (h *companyFeedbackHandlers) ListReportedFeedback(c *fiber.Ctx) error {
	rows, err := h.feedback.ListReported(c.Context())
	if err != nil {
		return err
	}
	out := make([]reportedFeedbackResponse, len(rows))
	for i, r := range rows {
		out[i] = toReportedFeedbackResponse(r)
	}
	return c.JSON(fiber.Map{"data": out})
}

// HideFeedback is the moderator lever: hide a specific review, addressed by
// its id, and drop it out of its company's public list and average in the
// same step. Idempotent; role-gated. 404 for an unknown id.
func (h *companyFeedbackHandlers) HideFeedback(c *fiber.Ctx) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	if err := h.feedback.Hide(c.Context(), id); err != nil {
		return companyFeedbackError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}
