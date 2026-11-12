package handler

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/identity/auth"
)

// An email id that is not a number names no email: a 404, not the 400 of the other id routes.
func TestMailContract_NonNumericEmailIDIs404(t *testing.T) {
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	signedIn := func(c *fiber.Ctx) error { c.Locals(auth.LocalsUserID, int64(7)); return c.Next() }
	app.Get("/api/v1/me/emails/:id", signedIn, newContract(contractHandlers{inboxHandlers: &inboxHandlers{}}))

	req := httptest.NewRequestWithContext(context.Background(), "GET", "/api/v1/me/emails/abc", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusNotFound {
		t.Errorf("non-numeric email id = %d, want 404", resp.StatusCode)
	}
}
