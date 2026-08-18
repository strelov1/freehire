package handler

import (
	"testing"

	"github.com/gofiber/fiber/v2"
)

func geoApp() *fiber.App {
	h := newGeoHandlers()
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	// A nil throttler fails open, so this exercises the handler and not the limiter.
	h.register(app, middleware{})
	return app
}

func TestSearchCitiesEndpoint_ReturnsMatches(t *testing.T) {
	app := geoApp()

	status, body := doGet(t, app, "/geo/cities?q=Florian")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	data, _ := body["data"].([]any)
	found := false
	for _, d := range data {
		item, _ := d.(map[string]any)
		if item["value"] == "Florianópolis" && item["country"] == "br" {
			found = true
		}
	}
	if !found {
		t.Errorf("data = %v, missing Florianópolis/br", data)
	}
}

func TestSearchCitiesEndpoint_CountryNarrows(t *testing.T) {
	app := geoApp()

	status, body := doGet(t, app, "/geo/cities?q=San&country=us")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	data, _ := body["data"].([]any)
	if len(data) == 0 {
		t.Fatal("expected at least one match for q=San&country=us")
	}
	for _, d := range data {
		item, _ := d.(map[string]any)
		if item["country"] != "us" {
			t.Errorf("result %v has country %v, want us (filter not applied)", item, item["country"])
		}
	}
}

func TestSearchCitiesEndpoint_BlankQueryReturnsEmptyList(t *testing.T) {
	app := geoApp()

	status, body := doGet(t, app, "/geo/cities")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	data, ok := body["data"].([]any)
	if !ok {
		t.Fatalf("data = %v, want an array (even when empty)", body["data"])
	}
	if len(data) != 0 {
		t.Errorf("data = %v, want empty for a blank query", data)
	}
}

func TestSearchCitiesEndpoint_RequiresNoAuth(t *testing.T) {
	app := geoApp()

	status, _ := doGet(t, app, "/geo/cities?q=Berlin")
	if status == fiber.StatusUnauthorized {
		t.Error("anonymous request got 401, want city search to be public")
	}
}
