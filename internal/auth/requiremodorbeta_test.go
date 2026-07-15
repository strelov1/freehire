package auth

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// fakeModBeta satisfies both RoleLoader and BetaLoader with canned values.
type fakeModBeta struct {
	role    string
	roleErr error
	beta    bool
	betaErr error
}

func (f fakeModBeta) GetUserRole(_ context.Context, _ int64) (string, error) {
	return f.role, f.roleErr
}
func (f fakeModBeta) IsBetaTester(_ context.Context, _ int64) (bool, error) { return f.beta, f.betaErr }

func modBetaApp(loader fakeModBeta, inject bool) *fiber.App {
	app := fiber.New()
	if inject {
		app.Use(func(c *fiber.Ctx) error {
			c.Locals(localsUserID, int64(5))
			return c.Next()
		})
	}
	app.Get("/admin", RequireModeratorOrBeta(loader, loader), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})
	return app
}

func TestRequireModeratorOrBeta(t *testing.T) {
	cases := []struct {
		name   string
		loader fakeModBeta
		inject bool
		want   int
	}{
		{"moderator is allowed", fakeModBeta{role: "moderator"}, true, http.StatusOK},
		{"beta tester is allowed", fakeModBeta{role: "user", beta: true}, true, http.StatusOK},
		{"neither is forbidden", fakeModBeta{role: "user", beta: false}, true, http.StatusForbidden},
		{"role-load error but beta still allows", fakeModBeta{roleErr: errors.New("x"), beta: true}, true, http.StatusOK},
		{"both loaders error is unauthorized", fakeModBeta{roleErr: errors.New("x"), betaErr: errors.New("y")}, true, http.StatusUnauthorized},
		{"no user id is unauthenticated", fakeModBeta{role: "moderator"}, false, http.StatusUnauthorized},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := statusOf(t, modBetaApp(c.loader, c.inject)); got != c.want {
				t.Errorf("status = %d, want %d", got, c.want)
			}
		})
	}
}
