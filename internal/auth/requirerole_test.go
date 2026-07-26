package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// fakeRoleLoader returns a canned role (and optional error) for any user id.
type fakeRoleLoader struct {
	role string
	err  error
}

func (f fakeRoleLoader) GetUserRole(_ context.Context, _ int64) (string, error) {
	return f.role, f.err
}

// roleApp mounts RequireRole behind a stub that injects an authenticated user id
// into the locals (as RequireAuth/RequireAuthOrKey would). When inject is false the
// locals stay empty, simulating an unauthenticated request reaching the guard.
func roleApp(loader RoleLoader, role string, inject bool) *fiber.App {
	app := fiber.New()
	if inject {
		app.Use(func(c *fiber.Ctx) error {
			c.Locals(localsUserID, int64(5))
			return c.Next()
		})
	}
	app.Get("/admin", RequireRole(loader, role), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})
	return app
}

func statusOf(t *testing.T, app *fiber.App) int {
	t.Helper()
	resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/admin", nil))
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	return resp.StatusCode
}

func TestRequireRole_AllowsMatchingRole(t *testing.T) {
	if got := statusOf(t, roleApp(fakeRoleLoader{role: "moderator"}, "moderator", true)); got != http.StatusOK {
		t.Errorf("status = %d, want 200", got)
	}
}

func TestRequireRole_ForbidsWrongRole(t *testing.T) {
	if got := statusOf(t, roleApp(fakeRoleLoader{role: "user"}, "moderator", true)); got != http.StatusForbidden {
		t.Errorf("status = %d, want 403", got)
	}
}

func TestRequireRole_UnauthenticatedWithoutUserID(t *testing.T) {
	if got := statusOf(t, roleApp(fakeRoleLoader{role: "moderator"}, "moderator", false)); got != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", got)
	}
}

func TestRequireRole_UnauthorizedWhenLoaderErrors(t *testing.T) {
	loader := fakeRoleLoader{err: errors.New("no such user")}
	if got := statusOf(t, roleApp(loader, "moderator", true)); got != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", got)
	}
}
