package handler

import (
	"context"
	"errors"
	"math"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/engage/community"
	"github.com/strelov1/freehire/internal/engage/companyfeedback"
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

// registerPublic mounts pseudonymous feedback reads without requiring sign-in.
func (h *companyFeedbackHandlers) registerPublic(api fiber.Router, contract fiber.Handler) {
	// Only pseudonymous persona handles are ever exposed, never a user id, so feedback is
	// browsable without signing in.
	api.Get("/companies/:slug/feedback", contract)
}

func (h *companyFeedbackHandlers) register(api fiber.Router, mw middleware, contract fiber.Handler) {
	h.registerPublic(api, contract)
	// Writes are cookie-only, the same as thread create/reply: this is authored public
	// content, not the single-bit vote a leaked API key is trusted with.
	api.Get("/companies/:slug/feedback/mine", mw.cookie, contract)
	api.Post("/companies/:slug/feedback", mw.cookie, contract)
	api.Delete("/companies/:slug/feedback", mw.cookie, contract)
	// Reporting a specific review, and the moderator lever it feeds, are addressed
	// by the review's own id — flat routes, the same shape as /threads/:id/close —
	// rather than nested under a company slug the id doesn't need to be found.
	api.Post("/company-feedback/:id/report", mw.cookie, contract)
	api.Get("/company-feedback/reported", mw.cookie, mw.moderator, contract)
	api.Post("/company-feedback/:id/hide", mw.cookie, mw.moderator, contract)
}

func toCompanyFeedbackSummary(s companyfeedback.Summary) apigen.CompanyFeedbackSummary {
	out := apigen.CompanyFeedbackSummary{FeedbackCount: int64(s.Count)}
	if s.RatingAvg != nil {
		out.FeedbackRatingAvg = apigen.NewNilFloat32(*s.RatingAvg)
	} else {
		out.FeedbackRatingAvg.SetToNull()
	}
	return out
}

// toCompanyFeedback is the public shape of one feedback entry: the persona handle is the
// only author identity ever sent to a client.
func toCompanyFeedback(f companyfeedback.Feedback) apigen.CompanyFeedback {
	return apigen.CompanyFeedback{
		ID: f.ID, Author: personaOrDeleted(f.AuthorHandle), Rating: int64(f.Rating),
		FeedbackType: f.FeedbackType, Body: f.Body, CreatedAt: f.CreatedAt, UpdatedAt: f.UpdatedAt,
	}
}

func toCompanyFeedbacks(items []companyfeedback.Feedback) []apigen.CompanyFeedback {
	out := make([]apigen.CompanyFeedback, len(items))
	for i, f := range items {
		out[i] = toCompanyFeedback(f)
	}
	return out
}

func toReportedCompanyFeedback(r companyfeedback.ReportedFeedback) apigen.ReportedCompanyFeedback {
	f := toCompanyFeedback(r.Feedback)
	return apigen.ReportedCompanyFeedback{
		ID: f.ID, Author: f.Author, Rating: f.Rating, FeedbackType: f.FeedbackType, Body: f.Body,
		CreatedAt: f.CreatedAt, UpdatedAt: f.UpdatedAt,
		CompanySlug: r.CompanySlug, ReportCount: r.ReportCount, ReportReasons: r.ReportReasons,
	}
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
	default:
		return err
	}
}

// ListCompanyFeedback returns a company's feedback, newest first, offset-paginated. Public.
func (h *companyFeedbackHandlers) ListCompanyFeedback(ctx context.Context, params apigen.ListCompanyFeedbackParams) (*apigen.CompanyFeedbackPage, error) {
	limit, offset, err := contractPageWindowed(ctx, defaultLimit, maxLimit)
	if err != nil {
		return nil, err
	}
	items, err := h.feedback.List(ctx, params.Slug, int32(limit), int32(offset))
	if err != nil {
		return nil, companyFeedbackError(err)
	}
	total, err := h.feedback.Count(ctx, params.Slug)
	if err != nil {
		return nil, companyFeedbackError(err)
	}
	return &apigen.CompanyFeedbackPage{
		Data: toCompanyFeedbacks(items),
		Meta: apigen.PageMeta{Total: total, Limit: int64(limit), Offset: int64(offset)},
	}, nil
}

// MyCompanyFeedback returns all of the caller's own feedback on a company, across
// every category they've reviewed it under (an empty array when they have not
// left any yet) — the write form's per-category prefill, so picking a category
// the caller has already used edits that review instead of starting a new one.
func (h *companyFeedbackHandlers) MyCompanyFeedback(ctx context.Context, params apigen.MyCompanyFeedbackParams) (*apigen.MyCompanyFeedbackOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	items, err := h.feedback.Mine(ctx, userID, params.Slug)
	if err != nil {
		return nil, companyFeedbackError(err)
	}
	return &apigen.MyCompanyFeedbackOK{Data: toCompanyFeedbacks(items)}, nil
}

// UpsertCompanyFeedback creates or overwrites the caller's feedback in one category
// on a company; 400/404/422/429. The response's nested `company` field is the
// freshly recomputed counters, so the caller can update its own view of the
// company (e.g. a header badge) without a second round trip.
func (h *companyFeedbackHandlers) UpsertCompanyFeedback(ctx context.Context, req *apigen.CompanyFeedbackRequest, params apigen.UpsertCompanyFeedbackParams) (*apigen.UpsertCompanyFeedbackCreated, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	// The rating was an int16 on the wire; a number outside it was a body that did not parse.
	if req.Rating.Value < math.MinInt16 || req.Rating.Value > math.MaxInt16 {
		return nil, fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	f, summary, err := h.feedback.Upsert(ctx, userID, params.Slug, int16(req.Rating.Value), req.FeedbackType.Value, req.Body.Value)
	if err != nil {
		return nil, companyFeedbackError(err)
	}
	out := toCompanyFeedback(f)
	out.Company = apigen.NewOptCompanyFeedbackSummary(toCompanyFeedbackSummary(summary))
	return &apigen.UpsertCompanyFeedbackCreated{Data: out}, nil
}

// DeleteCompanyFeedback removes the caller's own feedback in one category
// (?feedback_type=) on a company (no-op when absent) and returns the
// company's freshly recomputed counters — the same cast/clear-returns-the-tally
// shape as VoteControl's clear endpoints, so the caller never needs a
// follow-up fetch to learn the new count.
func (h *companyFeedbackHandlers) DeleteCompanyFeedback(ctx context.Context, params apigen.DeleteCompanyFeedbackParams) (*apigen.DeleteCompanyFeedbackOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	summary, err := h.feedback.Delete(ctx, userID, params.Slug, params.FeedbackType.Value)
	if err != nil {
		return nil, companyFeedbackError(err)
	}
	return &apigen.DeleteCompanyFeedbackOK{Data: toCompanyFeedbackSummary(summary)}, nil
}

// ReportCompanyFeedback files a complaint against a specific review, addressed by its
// id. Any signed-in user; a second report of the same review by the same user
// is a silent no-op. 400/404.
func (h *companyFeedbackHandlers) ReportCompanyFeedback(ctx context.Context, req *apigen.CompanyFeedbackReportRequest, params apigen.ReportCompanyFeedbackParams) error {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return err
	}
	id, err := contractPathID(params.ID)
	if err != nil {
		return err
	}
	if err := h.feedback.Report(ctx, userID, id, req.Reason.Value); err != nil {
		return companyFeedbackError(err)
	}
	return nil
}

// ListReportedCompanyFeedback returns every review with at least one report,
// most-reported first — the moderator queue's read. Role-gated.
func (h *companyFeedbackHandlers) ListReportedCompanyFeedback(ctx context.Context) (*apigen.ListReportedCompanyFeedbackOK, error) {
	rows, err := h.feedback.ListReported(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]apigen.ReportedCompanyFeedback, len(rows))
	for i, r := range rows {
		out[i] = toReportedCompanyFeedback(r)
	}
	return &apigen.ListReportedCompanyFeedbackOK{Data: out}, nil
}

// HideCompanyFeedback is the moderator lever: hide a specific review, addressed by
// its id, and drop it out of its company's public list and average in the
// same step. Idempotent; role-gated. 404 for an unknown id.
func (h *companyFeedbackHandlers) HideCompanyFeedback(ctx context.Context, params apigen.HideCompanyFeedbackParams) error {
	id, err := contractPathID(params.ID)
	if err != nil {
		return err
	}
	if err := h.feedback.Hide(ctx, id); err != nil {
		return companyFeedbackError(err)
	}
	return nil
}
