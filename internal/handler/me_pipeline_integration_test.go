//go:build integration

// Integration test for the Pipeline aggregate endpoint: GET /api/v1/me/jobs/pipeline
// must aggregate the caller's applications into the seven buckets server-side,
// counting an applied-with-no-stage row as no_answer, excluding saved-only and
// viewed-only rows, and requiring authentication. Run with:
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
	"github.com/strelov1/freehire/internal/jobtracking"
)

func TestMyPipelineEndpoint(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	queries := db.New(pool)

	var userID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email) VALUES ('pipeline@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	seedJob := func(t *testing.T, ext string) int64 {
		t.Helper()
		var id int64
		if err := pool.QueryRow(ctx,
			`INSERT INTO jobs (source, external_id, url, title, public_slug)
			 VALUES ('test', $1, 'http://example.test', 'Job ' || $1, 'pipe-' || $1)
			 RETURNING id`, ext).Scan(&id); err != nil {
			t.Fatalf("seed job %q: %v", ext, err)
		}
		return id
	}
	stage := func(t *testing.T, jobID int64, s string) {
		t.Helper()
		if _, err := queries.TrackJob(ctx, db.TrackJobParams{
			UserID: userID, JobID: jobID, Stage: pgtype.Text{String: s, Valid: true},
		}); err != nil {
			t.Fatalf("track stage %q: %v", s, err)
		}
	}

	// One application per non-empty bucket, plus a second applied-no-stage row so
	// no_answer is distinguishable, plus a saved-only and a viewed-only row that
	// must NOT count as applications.
	stage(t, seedJob(t, "screening"), "screening")
	stage(t, seedJob(t, "responded"), "responded") // also in_progress
	stage(t, seedJob(t, "interview"), "interview")
	stage(t, seedJob(t, "offer"), "offer")
	stage(t, seedJob(t, "accepted"), "accepted")
	stage(t, seedJob(t, "rejected"), "rejected")
	stage(t, seedJob(t, "withdrawn"), "withdrawn")

	// applied with no explicit stage → no_answer (two of them)
	if _, err := queries.MarkJobApplied(ctx, db.MarkJobAppliedParams{UserID: userID, JobID: seedJob(t, "applied1")}); err != nil {
		t.Fatalf("apply1: %v", err)
	}
	if _, err := queries.MarkJobApplied(ctx, db.MarkJobAppliedParams{UserID: userID, JobID: seedJob(t, "applied2")}); err != nil {
		t.Fatalf("apply2: %v", err)
	}
	// saved-only and viewed-only — excluded from applications
	if _, err := queries.SaveJob(ctx, db.SaveJobParams{UserID: userID, JobID: seedJob(t, "saved")}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := queries.RecordJobView(ctx, db.RecordJobViewParams{UserID: userID, JobID: seedJob(t, "viewed")}); err != nil {
		t.Fatalf("view: %v", err)
	}

	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(userID)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	h := &API{pool: pool, queries: queries, issuer: iss, tracking: jobtracking.New(jobtracking.NewQueriesRepository(queries))}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/me/jobs/pipeline", auth.RequireAuth(iss), h.MyPipeline)

	t.Run("aggregates applications into buckets", func(t *testing.T) {
		req := httptest.NewRequest(fiber.MethodGet, "/api/v1/me/jobs/pipeline", nil)
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("GET pipeline: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("status = %d, want 200 (body %s)", resp.StatusCode, body)
		}

		var body struct {
			Data struct {
				Applications int64 `json:"applications"`
				Buckets      struct {
					NoAnswer     int64 `json:"no_answer"`
					InProgress   int64 `json:"in_progress"`
					Interviewing int64 `json:"interviewing"`
					Offer        int64 `json:"offer"`
					Accepted     int64 `json:"accepted"`
					Rejected     int64 `json:"rejected"`
					Declined     int64 `json:"declined"`
				} `json:"buckets"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}

		b := body.Data.Buckets
		if body.Data.Applications != 9 {
			t.Errorf("applications = %d, want 9", body.Data.Applications)
		}
		if b.NoAnswer != 2 {
			t.Errorf("no_answer = %d, want 2", b.NoAnswer)
		}
		if b.InProgress != 2 {
			t.Errorf("in_progress = %d, want 2 (screening+responded)", b.InProgress)
		}
		if b.Interviewing != 1 || b.Offer != 1 || b.Accepted != 1 || b.Rejected != 1 || b.Declined != 1 {
			t.Errorf("buckets = %+v, want each of interviewing/offer/accepted/rejected/declined = 1", b)
		}
		sum := b.NoAnswer + b.InProgress + b.Interviewing + b.Offer + b.Accepted + b.Rejected + b.Declined
		if sum != body.Data.Applications {
			t.Errorf("buckets sum = %d, want applications = %d", sum, body.Data.Applications)
		}
	})

	t.Run("unauthenticated is 401", func(t *testing.T) {
		req := httptest.NewRequest(fiber.MethodGet, "/api/v1/me/jobs/pipeline", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("GET pipeline: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusUnauthorized {
			t.Errorf("status = %d, want 401", resp.StatusCode)
		}
	})
}
