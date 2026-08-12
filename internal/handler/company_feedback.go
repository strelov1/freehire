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
// pseudonymous persona. The use cases live in companyfeedback.Service; the
// handlers translate wire ↔ domain and delegate to it.
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
}

func toCompanyFeedbackResponse(f companyfeedback.Feedback) companyFeedbackResponse {
	return companyFeedbackResponse{
		ID: f.ID, Author: personaOrDeleted(f.AuthorHandle), Rating: f.Rating,
		FeedbackType: f.FeedbackType, Body: f.Body, CreatedAt: f.CreatedAt, UpdatedAt: f.UpdatedAt,
	}
}

type upsertCompanyFeedbackBody struct {
	Rating       int16  `json:"rating"`
	FeedbackType string `json:"feedback_type"`
	Body         string `json:"body"`
}

// companyFeedbackError maps companyfeedback sentinels to HTTP statuses; anything
// else falls through to RenderError's 500.
func companyFeedbackError(err error) error {
	switch {
	case errors.Is(err, companyfeedback.ErrCompanyNotFound):
		return fiber.NewError(fiber.StatusNotFound, "company not found")
	case errors.Is(err, companyfeedback.ErrInvalidRating):
		return fiber.NewError(fiber.StatusBadRequest, "rating must be between 1 and 5")
	case errors.Is(err, companyfeedback.ErrInvalidFeedbackType):
		return fiber.NewError(fiber.StatusBadRequest, "invalid feedback type")
	case errors.Is(err, companyfeedback.ErrEmptyBody):
		return fiber.NewError(fiber.StatusUnprocessableEntity, "body is required")
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

// GetMyFeedback returns the caller's own feedback on a company, or null data
// when they have not left one yet — the edit form's prefill.
func (h *companyFeedbackHandlers) GetMyFeedback(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	f, err := h.feedback.Mine(c.Context(), userID, c.Params("slug"))
	if err != nil {
		if errors.Is(err, companyfeedback.ErrNotFound) {
			return c.JSON(fiber.Map{"data": nil})
		}
		return companyFeedbackError(err)
	}
	return c.JSON(fiber.Map{"data": toCompanyFeedbackResponse(f)})
}

// UpsertFeedback creates or overwrites the caller's feedback on a company; 400/404/422.
func (h *companyFeedbackHandlers) UpsertFeedback(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	var in upsertCompanyFeedbackBody
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	f, err := h.feedback.Upsert(c.Context(), userID, c.Params("slug"), in.Rating, in.FeedbackType, in.Body)
	if err != nil {
		return companyFeedbackError(err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": toCompanyFeedbackResponse(f)})
}

// DeleteFeedback removes the caller's own feedback on a company (no-op when absent).
func (h *companyFeedbackHandlers) DeleteFeedback(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	if err := h.feedback.Delete(c.Context(), userID, c.Params("slug")); err != nil {
		return companyFeedbackError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}
