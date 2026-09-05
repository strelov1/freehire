//go:build integration

// Integration tests for the saved-search webhook HTTP flow against a real
// Postgres: create/update, get, patch toggles, delete removes the
// destination, and every endpoint requires the session cookie. Run with:
// go test -tags=integration ./internal/api/handler/
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
)

func TestWebhookEndToEnd(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	var userID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email, email_verified) VALUES ('webhook@example.test', true) RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	iss := auth.NewIssuer("test-secret", time.Hour)
	cookie, _ := iss.Issue(userID, testTokenVersion)
	queries := db.New(pool)
	h := newWebhookHandlers(queries)

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Post("/api/v1/me/webhook", auth.RequireAuth(iss, testVersions), h.CreateOrUpdateWebhook)
	app.Get("/api/v1/me/webhook", auth.RequireAuth(iss, testVersions), h.GetWebhook)
	app.Patch("/api/v1/me/webhook", auth.RequireAuth(iss, testVersions), h.SetWebhookEnabled)
	app.Delete("/api/v1/me/webhook", auth.RequireAuth(iss, testVersions), h.DeleteWebhook)

	cookieReq := func(method, path string, body []byte) *http.Request {
		var r *http.Request
		if body != nil {
			r = httptest.NewRequestWithContext(ctx, method, path, bytes.NewReader(body))
			r.Header.Set("Content-Type", "application/json")
		} else {
			r = httptest.NewRequestWithContext(ctx, method, path, nil)
		}
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
		return r
	}

	// GET before any destination exists returns a null data field, not a 404.
	getResp, err := app.Test(cookieReq(fiber.MethodGet, "/api/v1/me/webhook", nil))
	if err != nil {
		t.Fatalf("get (unconfigured): %v", err)
	}
	defer getResp.Body.Close()
	var unconfigured struct {
		Data *struct{} `json:"data"`
	}
	if err := json.NewDecoder(getResp.Body).Decode(&unconfigured); err != nil {
		t.Fatalf("decode get (unconfigured): %v", err)
	}
	if unconfigured.Data != nil {
		t.Errorf("GET before creation: data = %+v, want null", unconfigured.Data)
	}

	// Create.
	createResp, err := app.Test(cookieReq(fiber.MethodPost, "/api/v1/me/webhook",
		[]byte(`{"url":"https://example.test/hook"}`)))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer createResp.Body.Close()
	if createResp.StatusCode != fiber.StatusCreated {
		body, _ := io.ReadAll(createResp.Body)
		t.Fatalf("create status = %d, want 201 (body %s)", createResp.StatusCode, body)
	}
	var created struct {
		Data struct {
			URL     string `json:"url"`
			Enabled bool   `json:"enabled"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if created.Data.URL != "https://example.test/hook" || !created.Data.Enabled {
		t.Errorf("created = %+v, want the given URL and enabled=true", created.Data)
	}

	// Disable, then re-enable.
	disableResp, err := app.Test(cookieReq(fiber.MethodPatch, "/api/v1/me/webhook", []byte(`{"enabled":false}`)))
	if err != nil {
		t.Fatalf("disable: %v", err)
	}
	defer disableResp.Body.Close()
	var toggled struct {
		Data struct {
			URL     string `json:"url"`
			Enabled bool   `json:"enabled"`
		} `json:"data"`
	}
	if err := json.NewDecoder(disableResp.Body).Decode(&toggled); err != nil {
		t.Fatalf("decode disable: %v", err)
	}
	if toggled.Data.Enabled {
		t.Error("after PATCH enabled=false, want enabled=false")
	}
	if toggled.Data.URL != "https://example.test/hook" {
		t.Errorf("disable must not change the url, got %q", toggled.Data.URL)
	}

	enableResp, err := app.Test(cookieReq(fiber.MethodPatch, "/api/v1/me/webhook", []byte(`{"enabled":true}`)))
	if err != nil {
		t.Fatalf("enable: %v", err)
	}
	defer enableResp.Body.Close()
	if err := json.NewDecoder(enableResp.Body).Decode(&toggled); err != nil {
		t.Fatalf("decode enable: %v", err)
	}
	if !toggled.Data.Enabled {
		t.Error("after PATCH enabled=true, want enabled=true")
	}

	// Updating the URL replaces it in place.
	updateResp, err := app.Test(cookieReq(fiber.MethodPost, "/api/v1/me/webhook",
		[]byte(`{"url":"https://example.test/hook-2"}`)))
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	defer updateResp.Body.Close()
	var updated struct {
		Data struct {
			URL string `json:"url"`
		} `json:"data"`
	}
	if err := json.NewDecoder(updateResp.Body).Decode(&updated); err != nil {
		t.Fatalf("decode update: %v", err)
	}
	if updated.Data.URL != "https://example.test/hook-2" {
		t.Errorf("updated url = %q, want the new URL", updated.Data.URL)
	}

	// Delete removes it; a second delete 404s.
	deleteResp, err := app.Test(cookieReq(fiber.MethodDelete, "/api/v1/me/webhook", nil))
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	defer deleteResp.Body.Close()
	if deleteResp.StatusCode != fiber.StatusNoContent {
		t.Errorf("delete status = %d, want 204", deleteResp.StatusCode)
	}
	deleteAgainResp, err := app.Test(cookieReq(fiber.MethodDelete, "/api/v1/me/webhook", nil))
	if err != nil {
		t.Fatalf("delete again: %v", err)
	}
	defer deleteAgainResp.Body.Close()
	if deleteAgainResp.StatusCode != fiber.StatusNotFound {
		t.Errorf("second delete status = %d, want 404", deleteAgainResp.StatusCode)
	}

	// Every endpoint requires the session cookie.
	noCookie := httptest.NewRequestWithContext(ctx, fiber.MethodGet, "/api/v1/me/webhook", nil)
	noCookieResp, err := app.Test(noCookie)
	if err != nil {
		t.Fatalf("no-cookie get: %v", err)
	}
	defer noCookieResp.Body.Close()
	if noCookieResp.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("no-cookie get status = %d, want 401", noCookieResp.StatusCode)
	}
}

// RecordWebhookDeliverySuccess (called by notify.Runner's deliverOne on a
// successful webhook send) actually stamps last_success_at, and GetWebhook
// surfaces it.
func TestRecordWebhookDeliverySuccessStampsLastSuccessAt(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	var userID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email, email_verified) VALUES ('webhook-success@example.test', true) RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	queries := db.New(pool)
	if _, err := queries.UpsertWebhookConfig(ctx, db.UpsertWebhookConfigParams{
		UserID: userID, URL: "https://example.test/hook",
	}); err != nil {
		t.Fatalf("create webhook config: %v", err)
	}

	before, err := queries.GetWebhookConfig(ctx, userID)
	if err != nil {
		t.Fatalf("get before: %v", err)
	}
	if before.LastSuccessAt.Valid {
		t.Fatal("last_success_at should start unset")
	}

	if err := queries.RecordWebhookDeliverySuccess(ctx, userID); err != nil {
		t.Fatalf("record delivery success: %v", err)
	}

	after, err := queries.GetWebhookConfig(ctx, userID)
	if err != nil {
		t.Fatalf("get after: %v", err)
	}
	if !after.LastSuccessAt.Valid {
		t.Error("last_success_at should be set after RecordWebhookDeliverySuccess")
	}
}
