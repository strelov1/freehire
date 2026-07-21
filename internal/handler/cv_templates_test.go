package handler

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/cv"
)

// TestListCVTemplates checks the static templates endpoint returns every registered template
// with its display metadata. It needs no DB: the handler just projects cv.Templates().
func TestListCVTemplates(t *testing.T) {
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/cv-templates", (&API{}).ListCVTemplates)

	req := httptest.NewRequest(fiber.MethodGet, "/api/v1/cv-templates", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var body struct {
		Data []cv.TemplateInfo `json:"data"`
	}
	raw, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	if len(body.Data) != len(cv.Templates()) {
		t.Fatalf("returned %d templates, want %d", len(body.Data), len(cv.Templates()))
	}

	got := map[string]cv.TemplateInfo{}
	for _, ti := range body.Data {
		if ti.Label == "" || ti.Style == "" {
			t.Errorf("template %q missing label/style: %+v", ti.ID, ti)
		}
		got[ti.ID] = ti
	}
	if !got["classic-ats"].ATSSafe {
		t.Error("classic-ats should be ats_safe")
	}
	if got["sidebar"].ATSSafe {
		t.Error("sidebar should not be ats_safe")
	}
}
