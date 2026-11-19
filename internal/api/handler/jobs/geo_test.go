package jobs

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/apigen"
)

// The route tests through the contract server are in handler (geo_test.go); these hold the
// operation itself and the route Register mounts.
func TestSearchCitiesNarrowsByCountry(t *testing.T) {
	out, err := NewGeoHandlers().SearchCities(context.Background(), apigen.SearchCitiesParams{
		Q: apigen.NewOptString("San"), Country: apigen.NewOptString("us"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Data) == 0 || len(out.Data) > citySearchLimit {
		t.Fatalf("got %d matches, want 1..%d", len(out.Data), citySearchLimit)
	}
	for _, m := range out.Data {
		if m.Country != "us" {
			t.Errorf("%+v: country filter not applied", m)
		}
	}
}

func TestRegisterMountsCitySearch(t *testing.T) {
	app := fiber.New()
	NewGeoHandlers().Register(app.Group("/api/v1"), func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusTeapot) })
	resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/api/v1/geo/cities", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusTeapot {
		t.Errorf("status = %d, want the contract handler's %d", resp.StatusCode, fiber.StatusTeapot)
	}
}
