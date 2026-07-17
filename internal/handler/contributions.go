package handler

import (
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/contribution"
)

// contributionRequest is the submit body: just the pasted job URL.
type contributionRequest struct {
	URL string `json:"url"`
}

// contributionResponse is the public shape of a recorded contribution. submitted_by is
// omitted (ownership, internal); source + board name the company board the user discovered.
type contributionResponse struct {
	ID        int64      `json:"id"`
	URL       string     `json:"url"`
	Source    string     `json:"source"`
	Board     string     `json:"board"`
	Status    string     `json:"status"`
	CreatedAt *time.Time `json:"created_at"`
}

// toContributionResponse maps a domain contribution to its wire shape.
func toContributionResponse(c contribution.Contribution) contributionResponse {
	return contributionResponse{
		ID:        c.ID,
		URL:       c.URL,
		Source:    c.Source,
		Board:     c.Board,
		Status:    c.Status,
		CreatedAt: c.CreatedAt,
	}
}

// contributionError maps the contribution sentinels to HTTP statuses: an unsupported link
// is 422 (well-formed request, unprocessable target); a board we already crawl or already
// recorded is 409. Anything else falls through to RenderError as a 500.
func contributionError(err error) error {
	switch {
	case errors.Is(err, contribution.ErrUnsupportedATS):
		return fiber.NewError(fiber.StatusUnprocessableEntity, "link is not from a supported ATS board")
	case errors.Is(err, contribution.ErrBoardAlreadyTracked):
		return fiber.NewError(fiber.StatusConflict, "this board is already in the catalogue")
	case errors.Is(err, contribution.ErrBoardAlreadyContributed):
		return fiber.NewError(fiber.StatusConflict, "this board has already been contributed")
	default:
		return err
	}
}

// CreateContribution records a user-contributed job link and awards a point for a novel one.
// Authenticated by cookie or API key. A non-ATS link is 422, a duplicate is 409; a novel
// link returns 201 with the recorded contribution.
func (a *API) CreateContribution(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}

	var in contributionRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}

	rec, err := a.contribution.Submit(c.Context(), userID, in.URL)
	if err != nil {
		// On "already tracked", enrich the 409 with the company we cover so the UI can link to
		// it (and say the exact role will land on the next crawl).
		if errors.Is(err, contribution.ErrBoardAlreadyTracked) {
			body := fiber.Map{"error": "this board is already in the catalogue"}
			if name, slug, ok := a.contribution.TrackedCompany(c.Context(), in.URL); ok {
				body["company_name"] = name
				body["company_slug"] = slug
			}
			return c.Status(fiber.StatusConflict).JSON(body)
		}
		return contributionError(err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": toContributionResponse(rec)})
}

// ListMyContributions returns the caller's own contributions, newest first. Scoped to the
// authenticated user, so it never reveals another user's.
func (a *API) ListMyContributions(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}

	rows, err := a.contribution.ListMine(c.Context(), userID)
	if err != nil {
		return err
	}
	out := make([]contributionResponse, len(rows))
	for i, r := range rows {
		out[i] = toContributionResponse(r)
	}
	return c.JSON(fiber.Map{"data": out})
}
