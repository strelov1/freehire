package handler

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/plan"
)

// The pricing page reads every feature with one entry per tier, and lists that are empty
// rather than absent when billing is not configured.
func TestPlans_ListsEveryFeaturePerTierWithoutBilling(t *testing.T) {
	h := newPlansHandlers(plan.DefaultConfig(), nil)
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	h.register(app.Group("/api/v1"), newContract(contractHandlers{plansHandlers: h}))

	resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/api/v1/plans", nil))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	var body struct {
		Data struct {
			Features []struct {
				Feature string                     `json:"feature"`
				Free    map[string]json.RawMessage `json:"free"`
				Pro     map[string]json.RawMessage `json:"pro"`
				Ultra   map[string]json.RawMessage `json:"ultra"`
			} `json:"features"`
			Prices   []json.RawMessage `json:"prices"`
			Enforced []string          `json:"enforced"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil || resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status %d, decode: %v", resp.StatusCode, err)
	}
	if len(body.Data.Features) != len(plan.AllFeatures()) {
		t.Errorf("features = %d, want one per metered feature (%d)", len(body.Data.Features), len(plan.AllFeatures()))
	}
	for _, f := range body.Data.Features {
		for tier, a := range map[string]map[string]json.RawMessage{"free": f.Free, "pro": f.Pro, "ultra": f.Ultra} {
			if a["limit"] == nil || a["unlimited"] == nil {
				t.Errorf("%s %s = %v, want limit and unlimited", f.Feature, tier, a)
			}
		}
	}
	if body.Data.Prices == nil || body.Data.Enforced == nil {
		t.Errorf("prices %v, enforced %v: want empty lists, not absent", body.Data.Prices, body.Data.Enforced)
	}
}
