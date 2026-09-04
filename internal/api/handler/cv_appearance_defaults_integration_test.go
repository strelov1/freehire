//go:build integration

// Integration tests for the CV appearance-defaults HTTP surface (add-cv-appearance-defaults):
// GET returns the system defaults when nothing is saved, and PUT persists+validates.
// Run with: go test -tags=integration ./internal/api/handler/
package handler

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
)

func buildCVAppearanceDefaultsApp(h *cvHandlers, iss *auth.Issuer) *fiber.App {
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	saved := auth.RequireAuth(iss, testVersions)
	app.Get("/api/v1/me/cv-appearance-defaults", saved, h.GetCVAppearanceDefaults)
	app.Put("/api/v1/me/cv-appearance-defaults", saved, h.SetCVAppearanceDefaults)
	return app
}

type cvAppearanceDefaultsBody struct {
	Data struct {
		TemplateID string     `json:"template_id"`
		Style      cv.Style   `json:"style"`
		Margins    cv.Margins `json:"margins"`
	} `json:"data"`
}

type appearanceDefaultsFixture struct {
	app    *fiber.App
	store  *cv.Store
	userID int64
	token  string
}

func newAppearanceDefaultsFixture(t *testing.T, email string) appearanceDefaultsFixture {
	t.Helper()
	pool := startPostgres(t)
	queries := db.New(pool)
	if _, err := pool.Exec(context.Background(), "TRUNCATE cvs, users RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	iss := auth.NewIssuer("test-secret", time.Hour)
	store := cv.NewStore(cv.NewQueriesRepository(queries))
	h := &cvHandlers{queries: queries, cvStore: store}
	user := seedAccount(t, pool, email, false)
	tok, err := iss.Issue(user, testTokenVersion)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	return appearanceDefaultsFixture{app: buildCVAppearanceDefaultsApp(h, iss), store: store, userID: user, token: tok}
}

func TestGetCVAppearanceDefaults_FallsBackToSystemDefaults(t *testing.T) {
	f := newAppearanceDefaultsFixture(t, "get-defaults@example.test")

	resp := doCV(t, f.app, fiber.MethodGet, "/api/v1/me/cv-appearance-defaults", f.token, nil)
	if resp.StatusCode != fiber.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
	var out cvAppearanceDefaultsBody
	decodeJSON(t, resp, &out)
	if out.Data.TemplateID != cv.DefaultTemplateID {
		t.Errorf("template = %q, want system default %q", out.Data.TemplateID, cv.DefaultTemplateID)
	}
	if out.Data.Margins != cv.DefaultMargins() {
		t.Errorf("margins = %+v, want system default %+v", out.Data.Margins, cv.DefaultMargins())
	}
}

func TestGetCVAppearanceDefaults_ReturnsSaved(t *testing.T) {
	f := newAppearanceDefaultsFixture(t, "get-saved-defaults@example.test")

	saved := cv.AppearanceDefaults{
		TemplateID: "sidebar",
		Style:      cv.Style{FontFamily: "carlito", FontSize: 10, LineHeight: 0.65},
		Margins:    cv.Margins{Top: 1, Right: 1, Bottom: 1, Left: 1},
	}
	if _, err := f.store.SetAppearanceDefaults(context.Background(), f.userID, saved); err != nil {
		t.Fatalf("set: %v", err)
	}

	resp := doCV(t, f.app, fiber.MethodGet, "/api/v1/me/cv-appearance-defaults", f.token, nil)
	if resp.StatusCode != fiber.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
	var out cvAppearanceDefaultsBody
	decodeJSON(t, resp, &out)
	if out.Data.TemplateID != saved.TemplateID || out.Data.Style != saved.Style || out.Data.Margins != saved.Margins {
		t.Errorf("defaults = %+v, want %+v", out.Data, saved)
	}
}

func TestSetCVAppearanceDefaults_PersistsValidInput(t *testing.T) {
	f := newAppearanceDefaultsFixture(t, "set-defaults@example.test")

	in := setCVAppearanceDefaultsRequest{
		TemplateID: "compact",
		Style:      cv.Style{FontFamily: "tinos", FontSize: 9, LineHeight: 0.4},
		Margins:    cv.Margins{Top: 0.75, Right: 0.75, Bottom: 0.75, Left: 0.75},
	}
	resp := doCV(t, f.app, fiber.MethodPut, "/api/v1/me/cv-appearance-defaults", f.token, in)
	if resp.StatusCode != fiber.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
	var out cvAppearanceDefaultsBody
	decodeJSON(t, resp, &out)
	if out.Data.TemplateID != in.TemplateID || out.Data.Style != in.Style || out.Data.Margins != in.Margins {
		t.Errorf("response = %+v, want %+v", out.Data, in)
	}

	got, ok, err := f.store.GetAppearanceDefaults(context.Background(), f.userID)
	if err != nil || !ok {
		t.Fatalf("GetAppearanceDefaults: ok=%v err=%v", ok, err)
	}
	if got.TemplateID != in.TemplateID || got.Style != in.Style || got.Margins != in.Margins {
		t.Errorf("stored = %+v, want %+v", got, in)
	}
}

func TestSetCVAppearanceDefaults_RejectsUnknownTemplate(t *testing.T) {
	f := newAppearanceDefaultsFixture(t, "reject-defaults@example.test")

	in := setCVAppearanceDefaultsRequest{TemplateID: "not-a-real-template"}
	resp := doCV(t, f.app, fiber.MethodPut, "/api/v1/me/cv-appearance-defaults", f.token, in)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 400, body = %s", resp.StatusCode, body)
	}
	if _, ok, _ := f.store.GetAppearanceDefaults(context.Background(), f.userID); ok {
		t.Errorf("defaults were saved despite the unknown template")
	}
}

func TestSetCVAppearanceDefaults_ClampsOutOfRangeValues(t *testing.T) {
	f := newAppearanceDefaultsFixture(t, "clamp-defaults@example.test")

	in := setCVAppearanceDefaultsRequest{
		TemplateID: cv.DefaultTemplateID,
		Style:      cv.Style{FontSize: 99, LineHeight: 99},
		Margins:    cv.Margins{Top: 99, Right: -5, Bottom: 99, Left: -5},
	}
	resp := doCV(t, f.app, fiber.MethodPut, "/api/v1/me/cv-appearance-defaults", f.token, in)
	if resp.StatusCode != fiber.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
	var out cvAppearanceDefaultsBody
	decodeJSON(t, resp, &out)
	if out.Data.Style.FontSize == 99 || out.Data.Margins.Top == 99 {
		t.Errorf("out-of-range values were not clamped: %+v", out.Data)
	}
}

func TestSetCVAppearanceDefaults_ReplacesPreviousValue(t *testing.T) {
	f := newAppearanceDefaultsFixture(t, "replace-defaults@example.test")

	first := setCVAppearanceDefaultsRequest{TemplateID: "compact", Margins: cv.Margins{Top: 0.75, Right: 0.75, Bottom: 0.75, Left: 0.75}}
	firstResp := doCV(t, f.app, fiber.MethodPut, "/api/v1/me/cv-appearance-defaults", f.token, first)
	firstResp.Body.Close()

	second := setCVAppearanceDefaultsRequest{TemplateID: "sidebar", Margins: cv.Margins{Top: 1, Right: 1, Bottom: 1, Left: 1}}
	resp := doCV(t, f.app, fiber.MethodPut, "/api/v1/me/cv-appearance-defaults", f.token, second)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}

	got, ok, err := f.store.GetAppearanceDefaults(context.Background(), f.userID)
	if err != nil || !ok {
		t.Fatalf("GetAppearanceDefaults: ok=%v err=%v", ok, err)
	}
	if got.TemplateID != second.TemplateID || got.Margins != second.Margins {
		t.Errorf("stored = %+v, want the second save %+v (not a merge of both)", got, second)
	}
}
