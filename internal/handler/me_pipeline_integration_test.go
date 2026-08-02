//go:build integration

// Integration test for the Pipeline aggregate endpoint: GET /api/v1/me/tracking/pipeline
// must count the caller's applications per stage server-side, carrying every stage of the
// vocabulary (zero included), counting an applied-with-no-stage row as `applied`, excluding
// saved-only and viewed-only rows, and requiring authentication. Run with:
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
	"github.com/strelov1/freehire/internal/userjob"
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
	token, err := iss.Issue(userID, testTokenVersion)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	h := &trackingHandlers{tracking: jobtracking.New(jobtracking.NewQueriesRepository(queries, pool))}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/me/tracking/pipeline", auth.RequireAuth(iss, testVersions), h.TrackingPipeline)

	t.Run("counts applications by stage", func(t *testing.T) {
		req := httptest.NewRequest(fiber.MethodGet, "/api/v1/me/tracking/pipeline", nil)
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
				Applications int64            `json:"applications"`
				Stages       map[string]int64 `json:"stages"`
				// Buckets is asserted absent: the seven-bucket vocabulary was the third name
				// for one state, and a response still carrying it would keep it alive for
				// every reader that has not migrated.
				Buckets json.RawMessage `json:"buckets"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}

		if body.Data.Applications != 9 {
			t.Errorf("applications = %d, want 9", body.Data.Applications)
		}
		if body.Data.Buckets != nil {
			t.Errorf("data.buckets is still served (%s); it is replaced by data.stages", body.Data.Buckets)
		}

		// The applied-with-no-stage row counts as `applied` — it is an application waiting on a
		// first reply, which is exactly what the stage says.
		want := map[string]int64{
			"applied": 2, "screening": 1, "responded": 1, "interview": 1,
			"offer": 1, "accepted": 1, "rejected": 1, "withdrawn": 1,
		}
		for stage, n := range want {
			if body.Data.Stages[stage] != n {
				t.Errorf("stages[%q] = %d, want %d", stage, body.Data.Stages[stage], n)
			}
		}

		// Every stage of the vocabulary is present, zero included: a caller must be able to
		// read a count of nothing without telling it apart from a key the server forgot.
		var sum int64
		for _, s := range userjob.Stages {
			n, ok := body.Data.Stages[s]
			if !ok {
				t.Errorf("stages is missing %q; every stage key is always present", s)
			}
			sum += n
		}
		if sum != body.Data.Applications {
			t.Errorf("stage counts sum = %d, want applications = %d", sum, body.Data.Applications)
		}
		if len(body.Data.Stages) != len(userjob.Stages) {
			t.Errorf("stages has %d keys, want %d — a key outside the vocabulary reached the wire",
				len(body.Data.Stages), len(userjob.Stages))
		}
	})

	t.Run("unauthenticated is 401", func(t *testing.T) {
		req := httptest.NewRequest(fiber.MethodGet, "/api/v1/me/tracking/pipeline", nil)
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
