//go:build integration

// Integration tests for GetTrackedApplication's new auto_apply field
// (openspec/changes/auto-apply-review-tracking): the tracker drawer's own read of a job's
// live auto-apply attempt.
// Run with: go test -tags=integration ./internal/api/handler/
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
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/application/jobtracking"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
)

func newAutoApplyReviewInfoApp(pool *pgxpool.Pool, iss *auth.Issuer) *fiber.App {
	queries := db.New(pool)
	h := newInboxHandlers(queries, pool, nil, nil, "", false, "")
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	api := app.Group("/api/v1")
	api.Get("/me/tracking/:slug", auth.RequireAuth(iss, testVersions), h.GetTrackedApplication)
	return app
}

func trackForReviewInfoTest(t *testing.T, pool *pgxpool.Pool, userID int64, slug string) {
	t.Helper()
	tracking := jobtracking.New(jobtracking.NewQueriesRepository(db.New(pool), pool))
	stage := "preparing"
	if _, err := tracking.Track(context.Background(), userID, slug, &stage, nil, "auto_apply"); err != nil {
		t.Fatalf("track: %v", err)
	}
}

func trackedApplicationRequest(t *testing.T, app *fiber.App, slug, cookie string) *http.Response {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/me/tracking/"+slug, nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
	resp, err := app.Test(req, 10_000)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	return resp
}

type trackedApplicationAutoApply struct {
	Status          string `json:"status"`
	ResolvedPreview *struct {
		Fields []struct {
			Label string `json:"label"`
			Value string `json:"value"`
		} `json:"fields"`
	} `json:"resolved_preview"`
	Unmapped []struct {
		ID string `json:"id"`
	} `json:"unmapped"`
}

func decodeTrackedApplicationAutoApply(t *testing.T, resp *http.Response) *trackedApplicationAutoApply {
	t.Helper()
	var out struct {
		Data struct {
			AutoApply *trackedApplicationAutoApply `json:"auto_apply"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return out.Data.AutoApply
}

func TestGetTrackedApplication_NoAutoApplyAttemptIsNull(t *testing.T) {
	pool := startPostgres(t)
	truncateAutoApplyTailorTables(t, pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app := newAutoApplyReviewInfoApp(pool, iss)

	userID, cookie := autoApplyTailorUser(t, pool, iss, "no-attempt@example.test")
	insertAutoApplyJob(t, pool, "no-attempt-job")
	trackForReviewInfoTest(t, pool, userID, "no-attempt-job")

	resp := trackedApplicationRequest(t, app, "no-attempt-job", cookie)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := decodeTrackedApplicationAutoApply(t, resp); got != nil {
		t.Errorf("auto_apply = %+v, want nil", got)
	}
}

func TestGetTrackedApplication_PendingReviewCarriesTheResolvedPreview(t *testing.T) {
	pool := startPostgres(t)
	truncateAutoApplyTailorTables(t, pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app := newAutoApplyReviewInfoApp(pool, iss)
	q := db.New(pool)

	userID, cookie := autoApplyTailorUser(t, pool, iss, "pending@example.test")
	job := insertAutoApplyJob(t, pool, "pending-job")
	trackForReviewInfoTest(t, pool, userID, "pending-job")
	queueID := insertAutoApplyQueueRow(t, pool, userID, job)

	var cvID uuid.UUID
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO cvs (user_id, title, data) VALUES ($1, 'CV', '{}') RETURNING id`, userID).Scan(&cvID); err != nil {
		t.Fatal(err)
	}
	if _, err := q.SetAutoApplyTailoredCV(context.Background(), db.SetAutoApplyTailoredCVParams{ID: queueID, TailoredCvID: &cvID}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.SetAutoApplyResolvedPreview(context.Background(), db.SetAutoApplyResolvedPreviewParams{
		ID: queueID, ResolvedPreview: []byte(`{"fields":[{"label":"First name","value":"Ada"}]}`),
	}); err != nil {
		t.Fatal(err)
	}

	resp := trackedApplicationRequest(t, app, "pending-job", cookie)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	got := decodeTrackedApplicationAutoApply(t, resp)
	if got == nil || got.Status != "pending_review" {
		t.Fatalf("auto_apply = %+v, want pending_review", got)
	}
	if got.ResolvedPreview == nil || len(got.ResolvedPreview.Fields) != 1 || got.ResolvedPreview.Fields[0].Value != "Ada" {
		t.Errorf("resolved_preview = %+v, want the persisted field", got.ResolvedPreview)
	}
}

func TestGetTrackedApplication_BlockedCarriesUnmappedNeverLastError(t *testing.T) {
	pool := startPostgres(t)
	truncateAutoApplyTailorTables(t, pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app := newAutoApplyReviewInfoApp(pool, iss)
	q := db.New(pool)

	userID, cookie := autoApplyTailorUser(t, pool, iss, "blocked@example.test")
	job := insertAutoApplyJob(t, pool, "blocked-job")
	trackForReviewInfoTest(t, pool, userID, "blocked-job")
	queueID := insertAutoApplyQueueRow(t, pool, userID, job)

	var cvID uuid.UUID
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO cvs (user_id, title, data) VALUES ($1, 'CV', '{}') RETURNING id`, userID).Scan(&cvID); err != nil {
		t.Fatal(err)
	}
	if _, err := q.SetAutoApplyTailoredCV(context.Background(), db.SetAutoApplyTailoredCVParams{ID: queueID, TailoredCvID: &cvID}); err != nil {
		t.Fatal(err)
	}
	if err := q.MarkAutoApplyBlocked(context.Background(), db.MarkAutoApplyBlockedParams{
		ID: queueID, LastError: "an internal diagnostic that must never reach the wire",
		Unmapped: []byte(`[{"id":"why_us","label":"Why us?","required":true,"reason":"no known answer"}]`),
	}); err != nil {
		t.Fatal(err)
	}

	resp := trackedApplicationRequest(t, app, "blocked-job", cookie)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(body, []byte("an internal diagnostic")) {
		t.Fatalf("response leaked internal error text: %s", body)
	}

	var out struct {
		Data struct {
			AutoApply *trackedApplicationAutoApply `json:"auto_apply"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	got := out.Data.AutoApply
	if got == nil || got.Status != "blocked" {
		t.Fatalf("auto_apply = %+v, want blocked", got)
	}
	if len(got.Unmapped) != 1 || got.Unmapped[0].ID != "why_us" {
		t.Errorf("unmapped = %+v, want the one blocking field named", got.Unmapped)
	}
	if got.ResolvedPreview != nil {
		t.Errorf("resolved_preview = %+v, want nil for a blocked attempt", got.ResolvedPreview)
	}
}
