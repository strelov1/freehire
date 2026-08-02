package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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
	app.Post("/jobs/:slug/view", auth.RequireAuth(iss, testVersions), h.RecordView)
	app.Post("/jobs/:slug/apply", auth.RequireAuth(iss, testVersions), h.MarkApplied)
	app.Post("/jobs/:slug/save", auth.RequireAuth(iss, testVersions), h.SaveJob)
	app.Delete("/jobs/:slug/save", auth.RequireAuth(iss, testVersions), h.UnsaveJob)
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

// datedRepo captures what the dated apply path was handed, and which path ran at all: the
// stated-date and now() paths reach different repository methods, and a test that could not
// tell them apart would not notice the body being ignored.
type datedRepo struct {
	stubTrackingRepo
	on    time.Time
	plain bool
}

func (d *datedRepo) MarkApplied(context.Context, int64, int64, string) (jobtracking.Interaction, error) {
	d.plain = true
	return jobtracking.Interaction{JobID: 1}, nil
}

func (d *datedRepo) MarkAppliedOn(_ context.Context, _, _ int64, at time.Time, _ string) (jobtracking.Interaction, error) {
	d.on = at
	return jobtracking.Interaction{JobID: 1, AppliedAt: &at}, nil
}

func datedApplyApp(repo jobtracking.Repository) (*fiber.App, *auth.Issuer) {
	iss := auth.NewIssuer("test-secret", time.Hour)
	h := &trackingHandlers{tracking: jobtracking.New(repo)}
	app := fiber.New()
	app.Post("/jobs/:slug/apply", auth.RequireAuth(iss, testVersions), h.MarkApplied)
	return app, iss
}

func postApply(t *testing.T, app *fiber.App, token, body string) int {
	t.Helper()
	req := httptest.NewRequest(fiber.MethodPost, "/jobs/go-dev/apply", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	return resp.StatusCode
}

// The wire carries a day; storage takes an instant. Noon is that instant because midnight would
// render as the previous date for every reader west of Greenwich — the application would show a
// day earlier than the one the person typed.
func TestMarkApplied_StoresAStatedDayAtNoonUTC(t *testing.T) {
	repo := &datedRepo{}
	app, iss := datedApplyApp(repo)
	token, _ := iss.Issue(7, testTokenVersion)

	// A day relative to now, not a fixed one: the service measures the window against the real
	// clock here, so a hardcoded date would start failing a year after it was written.
	day := time.Now().UTC().AddDate(0, 0, -6)
	stated := day.Format("2006-01-02")
	if got := postApply(t, app, token, `{"applied_on":"`+stated+`"}`); got != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", got)
	}
	want := time.Date(day.Year(), day.Month(), day.Day(), 12, 0, 0, 0, time.UTC)
	if !repo.on.Equal(want) {
		t.Errorf("repository received %v, want %v", repo.on, want)
	}
}

// Without a body the endpoint is what it has always been. The dated path is an addition, not a
// replacement, so an existing caller sending nothing must not start taking a different route.
func TestMarkApplied_WithoutABodyKeepsTheUndatedPath(t *testing.T) {
	for _, body := range []string{"", "{}"} {
		repo := &datedRepo{}
		app, iss := datedApplyApp(repo)
		token, _ := iss.Issue(7, testTokenVersion)

		if got := postApply(t, app, token, body); got != fiber.StatusOK {
			t.Fatalf("body %q: status = %d, want 200", body, got)
		}
		if !repo.plain {
			t.Errorf("body %q: the dated path ran, want the plain one", body)
		}
		if !repo.on.IsZero() {
			t.Errorf("body %q: a date reached the repository", body)
		}
	}
}

// A date we cannot believe, or cannot read, is refused before anything is written. The window is
// the service's, so this also proves the handler does not hold a second copy of it.
func TestMarkApplied_RefusesAnUnusableDate(t *testing.T) {
	cases := map[string]string{
		"not a date":  `{"applied_on":"last tuesday"}`,
		"a timestamp": `{"applied_on":"2026-07-27T10:00:00Z"}`,
		// Not a string, and not JSON at all: both used to read as "no date given" and stamp
		// today, telling a caller who named a day that it worked.
		"a number":          `{"applied_on":20260727}`,
		"not json":          `applied_on=2026-07-27`,
		"in the future":     `{"applied_on":"` + time.Now().AddDate(0, 0, 2).Format("2006-01-02") + `"}`,
		"older than a year": `{"applied_on":"` + time.Now().AddDate(0, 0, -400).Format("2006-01-02") + `"}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			repo := &datedRepo{}
			app, iss := datedApplyApp(repo)
			token, _ := iss.Issue(7, testTokenVersion)

			if got := postApply(t, app, token, body); got != fiber.StatusBadRequest {
				t.Errorf("status = %d, want 400", got)
			}
			if !repo.on.IsZero() || repo.plain {
				t.Error("an unusable date reached the repository")
			}
		})
	}
}
