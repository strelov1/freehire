//go:build integration

// Integration test for the per-job profile-match endpoint:
// GET /api/v1/jobs/:slug/match must classify the job's skills against the
// caller's profile skills (exact/adjacent/missing) and return the coverage
// percent. Run with: go test -tags=integration ./internal/handler/
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

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/userprofile"
)

func TestJobMatchEndpoint(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	queries := db.New(pool)

	var userID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email) VALUES ('matcher@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO user_profiles (user_id, specializations, skills)
		 VALUES ($1, ARRAY['frontend'], ARRAY['react','typescript','gcp'])`, userID); err != nil {
		t.Fatalf("seed profile: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO jobs (source, external_id, url, title, public_slug, skills)
		 VALUES ('test', 'm1', 'http://example.test', 'Job', 'match-job',
		         ARRAY['react','typescript','graphql','nodejs','aws'])`); err != nil {
		t.Fatalf("seed job: %v", err)
	}

	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(userID)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	h := &API{
		pool: pool, queries: queries, issuer: iss,
		userProfile: userprofile.New(userprofile.NewQueriesRepository(queries)),
	}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/jobs/:slug/match", auth.RequireAuth(iss), h.JobMatch)

	type adj struct {
		Name string `json:"name"`
		Via  string `json:"via"`
	}
	type matchBody struct {
		Data struct {
			Total           int      `json:"total"`
			ExactCount      int      `json:"exact_count"`
			AdjacentCount   int      `json:"adjacent_count"`
			CoveragePercent int      `json:"coverage_percent"`
			Matched         []string `json:"matched"`
			Adjacent        []adj    `json:"adjacent"`
			Missing         []string `json:"missing"`
		} `json:"data"`
	}

	get := func(t *testing.T, slug string) (*http.Response, matchBody) {
		t.Helper()
		req := httptest.NewRequest(fiber.MethodGet, "/api/v1/jobs/"+slug+"/match", nil)
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("GET match: %v", err)
		}
		var body matchBody
		_ = json.NewDecoder(resp.Body).Decode(&body)
		return resp, body
	}

	t.Run("classifies job skills against the profile", func(t *testing.T) {
		resp, body := get(t, "match-job")
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("status = %d, want 200 (body %s)", resp.StatusCode, b)
		}
		d := body.Data
		if d.Total != 5 || d.ExactCount != 2 || d.AdjacentCount != 1 || d.CoveragePercent != 50 {
			t.Errorf("got total=%d exact=%d adjacent=%d percent=%d, want 5/2/1/50",
				d.Total, d.ExactCount, d.AdjacentCount, d.CoveragePercent)
		}
		if len(d.Adjacent) != 1 || d.Adjacent[0].Name != "aws" || d.Adjacent[0].Via != "gcp" {
			t.Errorf("adjacent = %+v, want [{aws gcp}]", d.Adjacent)
		}
		if len(d.Missing) != 2 {
			t.Errorf("missing = %v, want 2 entries", d.Missing)
		}
	})

	t.Run("unknown slug is 404", func(t *testing.T) {
		resp, _ := get(t, "no-such-job")
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusNotFound {
			t.Errorf("status = %d, want 404", resp.StatusCode)
		}
	})

	t.Run("caller without a profile is 404", func(t *testing.T) {
		var otherID int64
		if err := pool.QueryRow(ctx,
			`INSERT INTO users (email) VALUES ('noprofile@example.test') RETURNING id`).Scan(&otherID); err != nil {
			t.Fatalf("seed user: %v", err)
		}
		otherToken, err := iss.Issue(otherID)
		if err != nil {
			t.Fatalf("issue token: %v", err)
		}
		req := httptest.NewRequest(fiber.MethodGet, "/api/v1/jobs/match-job/match", nil)
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: otherToken})
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("GET match: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusNotFound {
			t.Errorf("status = %d, want 404 (no profile)", resp.StatusCode)
		}
	})
}
