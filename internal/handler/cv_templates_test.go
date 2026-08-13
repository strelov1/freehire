package handler

import (
	"context"
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
	app.Get("/api/v1/cv-templates", (&cvHandlers{}).ListCVTemplates)

	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/api/v1/cv-templates", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
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
	if _, ok := got["classic-ats"]; !ok {
		t.Error("classic-ats not registered")
	}
}

// The font picker reads this endpoint rather than carrying its own list, so every field it
// renders with — including the CSS stack the live preview needs — has to come back populated.
// A second registry in TypeScript is exactly what this is here to prevent.
func TestListCVFonts(t *testing.T) {
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/cv-fonts", (&cvHandlers{}).ListCVFonts)

	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/api/v1/cv-fonts", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var body struct {
		Data []cv.FontInfo `json:"data"`
	}
	raw, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	if len(body.Data) != len(cv.Fonts()) {
		t.Fatalf("returned %d fonts, want %d", len(body.Data), len(cv.Fonts()))
	}
	notes := 0
	for _, f := range body.Data {
		if f.ID == "" || f.Label == "" || f.CSS == "" {
			t.Errorf("font %+v is missing an id, label, or CSS stack", f)
		}
		if f.Note != "" {
			notes++
		}
	}
	// The note is what makes the list useful — "Calibri metrics" is why someone picks Carlito —
	// so it must survive the projection, not just exist in the registry.
	if notes == 0 {
		t.Error("no font carried a note; the picker has nothing to say about what each face matches")
	}
}
