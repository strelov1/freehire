package handler

import "github.com/gofiber/fiber/v2"

// GetMyCredits returns the caller's current AI-credits balance — the points left this
// month and when the monthly grant resets — without consuming any. Cookie or API key;
// never calls the LLM. Powers the balance widget on the profile page.
func (a *API) GetMyCredits(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	bal, err := a.credits.Balance(c.Context(), userID)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": bal})
}
