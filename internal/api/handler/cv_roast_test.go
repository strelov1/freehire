package handler

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// roastCtxFor runs fn with a Fiber context carrying the given query string, since
// roastRoleValues reads the request's params.
func roastCtxFor(t *testing.T, query string, fn func(c *fiber.Ctx)) {
	t.Helper()
	app := fiber.New()
	app.Get("/probe", func(c *fiber.Ctx) error {
		fn(c)
		return nil
	})
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/probe"+query, nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("probe request: %v", err)
	}
	defer resp.Body.Close()
}

func TestRoastRoleValues_UsesTheFirstInferredCategory(t *testing.T) {
	roastCtxFor(t, "", func(c *fiber.Ctx) {
		vals, role := roastRoleValues(c, []string{"backend", "ml"})
		if role != "backend" {
			t.Errorf("role = %q, want backend (the primary category)", role)
		}
		if got := vals["category"]; len(got) != 1 || got[0] != "backend" {
			t.Errorf("category filter = %v, want [backend]", got)
		}
	})
}

func TestRoastRoleValues_AnExplicitCategoryWins(t *testing.T) {
	roastCtxFor(t, "?category=devops", func(c *fiber.Ctx) {
		vals, role := roastRoleValues(c, []string{"backend"})
		if role != "devops" {
			t.Errorf("role = %q, want devops (the caller's override)", role)
		}
		if got := vals["category"]; len(got) != 1 || got[0] != "devops" {
			t.Errorf("category filter = %v, want [devops]", got)
		}
	})
}

func TestRoastRoleValues_NoCategoryResolvedMeansNoFilter(t *testing.T) {
	roastCtxFor(t, "", func(c *fiber.Ctx) {
		vals, role := roastRoleValues(c, nil)
		if role != "" {
			t.Errorf("role = %q, want empty — the dictionary resolved nothing and we do not guess", role)
		}
		if hasNonEmpty(vals["category"]) {
			t.Errorf("category filter = %v, want none", vals["category"])
		}
	})
}

func TestRoastRoleValues_DropsACallerSuppliedSkillsFilter(t *testing.T) {
	roastCtxFor(t, "?skills=rust", func(c *fiber.Ctx) {
		vals, _ := roastRoleValues(c, []string{"backend"})
		if hasNonEmpty(vals["skills"]) {
			t.Errorf("skills = %v, want stripped — the CV's skills are the measured set, not a filter", vals["skills"])
		}
	})
}
