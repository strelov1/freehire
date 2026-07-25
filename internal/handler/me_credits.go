package handler

import (
	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/credits"
	"github.com/strelov1/freehire/internal/db"
)

// creditsHandlers serves the caller's AI-points balance and ledger history.
type creditsHandlers struct {
	credits *credits.Store
	queries *db.Queries
}

func newCreditsHandlers(credits *credits.Store, queries *db.Queries) *creditsHandlers {
	return &creditsHandlers{credits: credits, queries: queries}
}

func (h *creditsHandlers) register(api fiber.Router, mw middleware) {
	api.Get("/me/credits", mw.key, h.GetMyCredits)
	api.Get("/me/credits/history", mw.key, h.GetMyCreditsHistory)
}

// GetMyCredits returns the caller's current AI-credits balance — the points left this
// month and when the monthly grant resets — without consuming any. Cookie or API key;
// never calls the LLM. Powers the balance widget on the profile page.
func (h *creditsHandlers) GetMyCredits(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	bal, err := h.credits.Balance(c.Context(), userID)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": bal})
}
