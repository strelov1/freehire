//go:build integration

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

	"github.com/strelov1/freehire/internal/application/jobtracking"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
)

// The board asks for ?compact=1 because it renders a card's identity and its tag row and
// nothing else. Measured on prod 2026-10-30 for a 500-card board: the blurb and the skills
// were 270 KB of a 689 KB response that the board never drew, and the response is
// serialized again into the SSR payload. Without the flag the listing keeps its full card,
// which the saved/history lists render.
func TestListTrackedJobs_CompactDropsWhatTheBoardDoesNotDraw(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	queries := db.New(pool)

	var userID, jobID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email) VALUES ('compact@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO jobs (source, external_id, url, title, company, company_slug, public_slug, description,
		                   work_mode, seniority, skills, collections)
		 VALUES ('test', 'c-1', 'http://example.test', 'Go Engineer', 'Acme', 'acme', 'go-engineer-c1',
		         '<p>We build things.</p>', 'remote', 'senior', ARRAY['go','postgres'], ARRAY['yc'])
		 RETURNING id`).Scan(&jobID); err != nil {
		t.Fatalf("seed job: %v", err)
	}
	if _, err := queries.MarkJobApplied(ctx, db.MarkJobAppliedParams{UserID: userID, JobID: jobID}); err != nil {
		t.Fatalf("apply: %v", err)
	}

	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(userID, testTokenVersion)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	h := &trackingHandlers{tracking: jobtracking.New(jobtracking.NewQueriesRepository(queries, pool))}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/me/tracking", auth.RequireAuth(iss, testVersions), h.ListTrackedJobs)

	type card struct {
		PublicSlug  string   `json:"public_slug"`
		Title       string   `json:"title"`
		Company     string   `json:"company"`
		WorkMode    string   `json:"work_mode"`
		Seniority   string   `json:"seniority"`
		Blurb       string   `json:"blurb"`
		Skills      []string `json:"skills"`
		Collections []string `json:"collections"`
	}
	get := func(t *testing.T, path string) card {
		t.Helper()
		req := httptest.NewRequestWithContext(ctx, fiber.MethodGet, path, nil)
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		resp, err := app.Test(req, 30_000)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("status = %d: %s", resp.StatusCode, body)
		}
		var parsed struct {
			Data []struct {
				Job *card `json:"job"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(parsed.Data) != 1 || parsed.Data[0].Job == nil {
			t.Fatalf("want one row with a job, got %s", body)
		}
		return *parsed.Data[0].Job
	}

	full := get(t, "/me/tracking?filter=board&limit=500")
	if full.Blurb == "" || len(full.Skills) == 0 || len(full.Collections) == 0 {
		t.Errorf("full card lost its blurb/skills/collections: %+v", full)
	}

	compact := get(t, "/me/tracking?filter=board&limit=500&compact=1")
	if compact.Blurb != "" || compact.Skills != nil || compact.Collections != nil {
		t.Errorf("compact card still carries what the board never draws: %+v", compact)
	}
	// What the board and its drawer DO draw: the identity and the tag row.
	if compact.PublicSlug != "go-engineer-c1" || compact.Title != "Go Engineer" || compact.Company != "Acme" ||
		compact.WorkMode != "remote" || compact.Seniority != "senior" {
		t.Errorf("compact card lost a field the board draws: %+v", compact)
	}
}
