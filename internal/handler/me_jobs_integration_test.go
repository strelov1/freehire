//go:build integration

// Integration test for the my-jobs listing wire contract: GET /api/v1/me/jobs
// must return the user's interactions with jobs in the shared jobview shape (no
// internal id), honor the filter param in data and meta.total, carry the per-tab
// counts, and keep closed jobs in the history. Run with:
// go test -tags=integration ./internal/handler/
package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/db"
)

func TestListMyJobsEndpoint(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	queries := db.New(pool)

	var userID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email) VALUES ('lister@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	seedJob := func(t *testing.T, ext string, closed bool) int64 {
		t.Helper()
		var id int64
		if err := pool.QueryRow(ctx,
			`INSERT INTO jobs (source, external_id, url, title, public_slug, closed_at)
			 VALUES ('test', $1, 'http://example.test', 'Job ' || $1, 'job-' || $1,
			         CASE WHEN $2 THEN now() ELSE NULL END)
			 RETURNING id`, ext, closed).Scan(&id); err != nil {
			t.Fatalf("seed job %q: %v", ext, err)
		}
		return id
	}
	viewedJob := seedJob(t, "viewed", false)
	appliedClosedJob := seedJob(t, "applied-closed", true)

	if _, err := queries.RecordJobView(ctx, db.RecordJobViewParams{UserID: userID, JobID: viewedJob}); err != nil {
		t.Fatalf("view: %v", err)
	}
	if _, err := queries.MarkJobApplied(ctx, db.MarkJobAppliedParams{UserID: userID, JobID: appliedClosedJob}); err != nil {
		t.Fatalf("apply: %v", err)
	}

	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(userID)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	h := &Handler{pool: pool, queries: queries, issuer: iss}
	app := fiber.New(fiber.Config{ErrorHandler: ErrorHandler})
	app.Get("/api/v1/me/jobs", auth.RequireAuth(iss), h.ListMyJobs)

	type item struct {
		Job       map[string]json.RawMessage `json:"job"`
		ViewedAt  *string                    `json:"viewed_at"`
		SavedAt   *string                    `json:"saved_at"`
		AppliedAt *string                    `json:"applied_at"`
	}
	type listBody struct {
		Data []item `json:"data"`
		Meta struct {
			Total  int `json:"total"`
			Limit  int `json:"limit"`
			Offset int `json:"offset"`
			Counts struct {
				All     int `json:"all"`
				Viewed  int `json:"viewed"`
				Saved   int `json:"saved"`
				Applied int `json:"applied"`
			} `json:"counts"`
		} `json:"meta"`
	}
	doList := func(t *testing.T, path string) listBody {
		t.Helper()
		req := httptest.NewRequest(fiber.MethodGet, path, nil)
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("GET %s: status = %d, want 200 (body %s)", path, resp.StatusCode, body)
		}
		var body listBody
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return body
	}

	t.Run("all interactions with jobview-shaped jobs and counts", func(t *testing.T) {
		body := doList(t, "/api/v1/me/jobs")
		if len(body.Data) != 2 || body.Meta.Total != 2 {
			t.Fatalf("got %d items, total %d, want 2/2", len(body.Data), body.Meta.Total)
		}
		job := body.Data[0].Job
		if _, leaked := job["id"]; leaked {
			t.Error("job must not expose the internal id")
		}
		if _, ok := job["public_slug"]; !ok {
			t.Error("job missing public_slug (not the jobview shape?)")
		}
		c := body.Meta.Counts
		if c.All != 2 || c.Viewed != 1 || c.Saved != 0 || c.Applied != 1 {
			t.Errorf("counts = %+v, want all=2 viewed=1 saved=0 applied=1", c)
		}
	})

	t.Run("filter=viewed returns only view-only interactions", func(t *testing.T) {
		body := doList(t, "/api/v1/me/jobs?filter=viewed")
		if len(body.Data) != 1 || body.Meta.Total != 1 {
			t.Fatalf("got %d items, total %d, want 1/1", len(body.Data), body.Meta.Total)
		}
		if body.Data[0].SavedAt != nil || body.Data[0].AppliedAt != nil {
			t.Error("viewed filter returned an interaction that was saved or applied")
		}
	})

	t.Run("filter=applied narrows data and total, keeps closed jobs", func(t *testing.T) {
		body := doList(t, "/api/v1/me/jobs?filter=applied")
		if len(body.Data) != 1 || body.Meta.Total != 1 {
			t.Fatalf("got %d items, total %d, want 1/1", len(body.Data), body.Meta.Total)
		}
		if body.Data[0].AppliedAt == nil {
			t.Error("applied filter returned a non-applied interaction")
		}
		var closedAt *string
		if err := json.Unmarshal(body.Data[0].Job["closed_at"], &closedAt); err != nil || closedAt == nil {
			t.Errorf("closed job must stay in the history with closed_at set (err %v)", err)
		}
	})
}

func TestListMyJobsBoardFilter(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	queries := db.New(pool)

	var userID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email) VALUES ('boardfilter@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	seedJob := func(t *testing.T, ext string) int64 {
		t.Helper()
		var id int64
		if err := pool.QueryRow(ctx,
			`INSERT INTO jobs (source, external_id, url, title, public_slug)
			 VALUES ('test', $1, 'http://example.test', 'Job ' || $1, 'board-' || $1)
			 RETURNING id`, ext).Scan(&id); err != nil {
			t.Fatalf("seed job %q: %v", ext, err)
		}
		return id
	}

	viewedOnlyJob := seedJob(t, "brd-viewed")
	savedJob := seedJob(t, "brd-saved")
	appliedJob := seedJob(t, "brd-applied")
	stageOnlyJob := seedJob(t, "brd-stage")

	// viewed-only: just a view, no save/apply/stage
	if _, err := queries.RecordJobView(ctx, db.RecordJobViewParams{UserID: userID, JobID: viewedOnlyJob}); err != nil {
		t.Fatalf("view: %v", err)
	}
	// saved
	if _, err := queries.SaveJob(ctx, db.SaveJobParams{UserID: userID, JobID: savedJob}); err != nil {
		t.Fatalf("save: %v", err)
	}
	// applied
	if _, err := queries.MarkJobApplied(ctx, db.MarkJobAppliedParams{UserID: userID, JobID: appliedJob}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	// stage-only: TrackJob sets stage without touching applied_at
	if _, err := queries.TrackJob(ctx, db.TrackJobParams{
		UserID: userID,
		JobID:  stageOnlyJob,
		Stage:  pgtype.Text{String: "interview", Valid: true},
	}); err != nil {
		t.Fatalf("track stage: %v", err)
	}

	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(userID)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	h := &Handler{pool: pool, queries: queries, issuer: iss}
	app := fiber.New(fiber.Config{ErrorHandler: ErrorHandler})
	app.Get("/api/v1/me/jobs", auth.RequireAuth(iss), h.ListMyJobs)

	req := httptest.NewRequest(fiber.MethodGet, "/api/v1/me/jobs?filter=board", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("GET /api/v1/me/jobs?filter=board: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200 (body %s)", resp.StatusCode, body)
	}

	var body struct {
		Data []struct {
			Job map[string]json.RawMessage `json:"job"`
		} `json:"data"`
		Meta struct {
			Total  int `json:"total"`
			Counts struct {
				All     int `json:"all"`
				Viewed  int `json:"viewed"`
				Saved   int `json:"saved"`
				Applied int `json:"applied"`
				Board   int `json:"board"`
			} `json:"counts"`
		} `json:"meta"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Collect the public_slugs returned
	got := make(map[string]bool)
	for _, item := range body.Data {
		var slug string
		if err := json.Unmarshal(item.Job["public_slug"], &slug); err != nil {
			t.Fatalf("unmarshal public_slug: %v", err)
		}
		got[slug] = true
	}

	if got["board-brd-viewed"] {
		t.Error("board filter must NOT include the viewed-only job")
	}
	if !got["board-brd-saved"] {
		t.Error("board filter must include the saved job")
	}
	if !got["board-brd-applied"] {
		t.Error("board filter must include the applied job")
	}
	if !got["board-brd-stage"] {
		t.Error("board filter must include the stage-only job (stage set, applied_at NULL)")
	}
	if len(body.Data) != 3 {
		t.Errorf("board filter returned %d items, want 3 (saved, applied, stage-only)", len(body.Data))
	}
	// meta.total must equal the board count (3), not the all count (4).
	if body.Meta.Total != 3 {
		t.Errorf("meta.total = %d, want 3 for board filter", body.Meta.Total)
	}
	if body.Meta.Counts.Board != 3 {
		t.Errorf("meta.counts.board = %d, want 3", body.Meta.Counts.Board)
	}
}
