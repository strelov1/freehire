//go:build integration

// Integration tests for the mentor profile withdraw/reactivate HTTP flow against a real
// Postgres: a repeat withdrawal succeeds instead of 404ing, a withdrawn profile can be
// resubmitted for moderation, and resubmitting a profile that was never withdrawn is
// refused. Run with: go test -tags=integration ./internal/api/handler/
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

	"github.com/strelov1/freehire/internal/engage/mentorship"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
)

func TestMentorshipWithdrawAndReactivateHTTPFlow(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	var userID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email, email_verified) VALUES ('mentor-http@example.test', true) RETURNING id`,
	).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO companies (slug, name) VALUES ('httpco', 'httpco') ON CONFLICT DO NOTHING`); err != nil {
		t.Fatalf("seed company: %v", err)
	}

	queries := db.New(pool)
	h := newMentorshipHandlers(mentorship.New(
		mentorship.NewQueriesRepository(queries, pool),
		mentorship.Config{},
	), nil)

	iss := auth.NewIssuer("test-secret", time.Hour)
	cookie, _ := iss.Issue(userID, testTokenVersion)
	cookieAuth := auth.RequireAuth(iss, testVersions)

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/me/mentorship/profile", cookieAuth, h.GetMyMentorProfile)
	app.Post("/api/v1/me/mentorship/profile", cookieAuth, h.SubmitMentorProfile)
	app.Delete("/api/v1/me/mentorship/profile", cookieAuth, h.WithdrawMentorProfile)
	app.Post("/api/v1/me/mentorship/profile/reactivate", cookieAuth, h.ReactivateMentorProfile)

	authed := func(method, path string) *http.Request {
		r := httptest.NewRequest(method, path, nil)
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
		return r
	}

	// Reactivating a profile that was never withdrawn (here: no profile at all yet) is
	// refused as not-found, not silently accepted.
	resp, err := app.Test(authed(fiber.MethodPost, "/api/v1/me/mentorship/profile/reactivate"))
	if err != nil {
		t.Fatalf("reactivate with no profile: %v", err)
	}
	if resp.StatusCode != fiber.StatusNotFound {
		t.Errorf("reactivate with no profile: status = %d, want 404", resp.StatusCode)
	}

	body := []byte(`{
		"company_slug": "httpco", "slug": "http-mentor", "name": "HTTP Mentor",
		"headline": "Staff Engineer", "topics": ["career"], "languages": ["en"],
		"timezone": "Europe/Berlin", "meeting_url": "https://meet.example.test/http-mentor",
		"session_minutes": 60, "notice_minutes": 120, "horizon_days": 30
	}`)
	createReq := httptest.NewRequest(fiber.MethodPost, "/api/v1/me/mentorship/profile", bytes.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
	createResp, err := app.Test(createReq)
	if err != nil {
		t.Fatalf("submit profile: %v", err)
	}
	if createResp.StatusCode != fiber.StatusCreated {
		b, _ := io.ReadAll(createResp.Body)
		t.Fatalf("submit profile: status = %d, body = %s", createResp.StatusCode, b)
	}

	// Reactivating a pending profile (never withdrawn) is a conflict, not a no-op.
	resp, err = app.Test(authed(fiber.MethodPost, "/api/v1/me/mentorship/profile/reactivate"))
	if err != nil {
		t.Fatalf("reactivate a pending profile: %v", err)
	}
	if resp.StatusCode != fiber.StatusConflict {
		t.Errorf("reactivate a pending profile: status = %d, want 409", resp.StatusCode)
	}

	// Withdraw, then withdraw again — the second must succeed, not 404.
	if resp, err = app.Test(authed(fiber.MethodDelete, "/api/v1/me/mentorship/profile")); err != nil {
		t.Fatalf("first withdraw: %v", err)
	} else if resp.StatusCode != fiber.StatusNoContent {
		t.Errorf("first withdraw: status = %d, want 204", resp.StatusCode)
	}
	if resp, err = app.Test(authed(fiber.MethodDelete, "/api/v1/me/mentorship/profile")); err != nil {
		t.Fatalf("second withdraw: %v", err)
	} else if resp.StatusCode != fiber.StatusNoContent {
		t.Errorf("second withdraw: status = %d, want 204, not the old 404 \"mentor not found\"", resp.StatusCode)
	}

	// The withdrawn profile does not read back as "paused".
	profResp, err := app.Test(authed(fiber.MethodGet, "/api/v1/me/mentorship/profile"))
	if err != nil {
		t.Fatalf("get profile: %v", err)
	}
	var profOut struct {
		Data struct {
			Status string `json:"status"`
			Paused bool   `json:"paused"`
		} `json:"data"`
	}
	if err := json.NewDecoder(profResp.Body).Decode(&profOut); err != nil {
		t.Fatalf("decode profile: %v", err)
	}
	if profOut.Data.Status != "withdrawn" || profOut.Data.Paused {
		t.Errorf("status=%q paused=%v, want withdrawn and not paused",
			profOut.Data.Status, profOut.Data.Paused)
	}

	// Reactivate: back to pending, no longer paused.
	reactivateResp, err := app.Test(authed(fiber.MethodPost, "/api/v1/me/mentorship/profile/reactivate"))
	if err != nil {
		t.Fatalf("reactivate: %v", err)
	}
	if reactivateResp.StatusCode != fiber.StatusOK {
		b, _ := io.ReadAll(reactivateResp.Body)
		t.Fatalf("reactivate: status = %d, body = %s", reactivateResp.StatusCode, b)
	}
	var reactivateOut struct {
		Data struct {
			Status string `json:"status"`
			Paused bool   `json:"paused"`
		} `json:"data"`
	}
	if err := json.NewDecoder(reactivateResp.Body).Decode(&reactivateOut); err != nil {
		t.Fatalf("decode reactivate response: %v", err)
	}
	if reactivateOut.Data.Status != "pending" || reactivateOut.Data.Paused {
		t.Errorf("status=%q paused=%v, want pending and not paused",
			reactivateOut.Data.Status, reactivateOut.Data.Paused)
	}
}
