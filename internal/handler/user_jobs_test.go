package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/jobtracking"
)

// userJobsApp mounts the view/apply routes behind RequireAuth on a handler whose
// tracking service is backed by a stub repository (no DB). The auth-gate cases
// below reject before the service is reached. Slug resolution and the DB-backed
// happy path / idempotency are covered by the db-package integration tests
// (GetJobBySlug, TestUserJobs).
func userJobsApp() (*fiber.App, *auth.Issuer) {
	iss := auth.NewIssuer("test-secret", time.Hour)
	h := &trackingHandlers{tracking: jobtracking.New(stubTrackingRepo{})}
	app := fiber.New()
	app.Post("/jobs/:slug/view", auth.RequireAuth(iss), h.RecordView)
	app.Post("/jobs/:slug/apply", auth.RequireAuth(iss), h.MarkApplied)
	app.Post("/jobs/:slug/save", auth.RequireAuth(iss), h.SaveJob)
	app.Delete("/jobs/:slug/save", auth.RequireAuth(iss), h.UnsaveJob)
	return app, iss
}

func postUserJob(t *testing.T, app *fiber.App, path, token string) int {
	t.Helper()
	return requestUserJob(t, app, fiber.MethodPost, path, token)
}

func requestUserJob(t *testing.T, app *fiber.App, method, path, token string) int {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if token != "" {
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	return resp.StatusCode
}

func TestRecordView_RequiresAuth(t *testing.T) {
	app, _ := userJobsApp()
	if got := postUserJob(t, app, "/jobs/go-dev-acme-t35nijto/view", ""); got != fiber.StatusUnauthorized {
		t.Errorf("status = %d, want 401", got)
	}
}

func TestMarkApplied_RequiresAuth(t *testing.T) {
	app, _ := userJobsApp()
	if got := postUserJob(t, app, "/jobs/go-dev-acme-t35nijto/apply", ""); got != fiber.StatusUnauthorized {
		t.Errorf("status = %d, want 401", got)
	}
}

func TestSaveJob_RequiresAuth(t *testing.T) {
	app, _ := userJobsApp()
	if got := postUserJob(t, app, "/jobs/go-dev-acme-t35nijto/save", ""); got != fiber.StatusUnauthorized {
		t.Errorf("status = %d, want 401", got)
	}
}

func TestUnsaveJob_RequiresAuth(t *testing.T) {
	app, _ := userJobsApp()
	if got := requestUserJob(t, app, fiber.MethodDelete, "/jobs/go-dev-acme-t35nijto/save", ""); got != fiber.StatusUnauthorized {
		t.Errorf("status = %d, want 401", got)
	}
}

// interactionResponse is the only interaction shape that reaches a response. This
// locks the contract: it omits user_id and carries job_id + the three timestamps.
func TestInteractionResponse_Shape(t *testing.T) {
	raw, err := json.Marshal(interactionResponse{JobID: 7})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, leaked := fields["user_id"]; leaked {
		t.Error("interactionResponse must not include user_id")
	}
	for _, want := range []string{"job_id", "viewed_at", "saved_at", "applied_at"} {
		if _, ok := fields[want]; !ok {
			t.Errorf("interactionResponse missing %q", want)
		}
	}
}

// TestToResponse_JSONShape pins the wire shape produced by toResponse: the JSON
// field names, a set *time.Time as a quoted RFC3339 string, a nil pointer as
// null, and a set *string as a quoted string. DB-free.
func TestToResponse_JSONShape(t *testing.T) {
	viewedAt := time.Date(2026, 6, 14, 9, 30, 0, 0, time.UTC)
	stage := "interview"

	resp := toResponse(jobtracking.Interaction{
		JobID:    7,
		ViewedAt: &viewedAt,
		Stage:    &stage,
		// SavedAt, AppliedAt, Notes left nil → expect null.
	})

	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	for _, want := range []string{"job_id", "viewed_at", "saved_at", "applied_at", "stage", "notes"} {
		if _, ok := fields[want]; !ok {
			t.Errorf("response missing field %q", want)
		}
	}
	if got := string(fields["viewed_at"]); got != `"2026-06-14T09:30:00Z"` {
		t.Errorf("viewed_at = %s, want quoted RFC3339", got)
	}
	if got := string(fields["stage"]); got != `"interview"` {
		t.Errorf("stage = %s, want %q", got, "interview")
	}
	for _, nullField := range []string{"saved_at", "applied_at", "notes"} {
		if got := string(fields[nullField]); got != "null" {
			t.Errorf("%s = %s, want null", nullField, got)
		}
	}
}
