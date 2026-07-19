//go:build integration

// Integration test for GET /api/v1/me/tracking/analyses: lists the caller's analysed
// jobs (newest first, including closed ones) with the fit-analysis quota in meta.
// Run with: go test -tags=integration ./internal/handler/
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/credits"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/matchanalysis"
	"github.com/strelov1/freehire/internal/resume"
	"github.com/strelov1/freehire/internal/userprofile"
)

func TestListMyAnalysesEndpoint(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	queries := db.New(pool)

	var userID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email) VALUES ('analyses@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	seedJob := func(slug string, closed bool) int64 {
		var id int64
		closedAt := "NULL"
		if closed {
			closedAt = "now()"
		}
		if err := pool.QueryRow(ctx,
			`INSERT INTO jobs (source, external_id, url, title, description, company, public_slug, content_hash, closed_at)
			 VALUES ('test',$1,'http://e.test','Role '||$1,'desc','Acme',$1,'h', `+closedAt+`) RETURNING id`,
			slug).Scan(&id); err != nil {
			t.Fatalf("seed job %s: %v", slug, err)
		}
		return id
	}
	seedAnalysis := func(jobID int64, score int, verdict string, age time.Duration) {
		blob, _ := json.Marshal(&matchanalysis.Analysis{OverallScore: score, Verdict: verdict})
		if _, err := pool.Exec(ctx,
			`INSERT INTO user_job_analysis (user_id, job_id, analysis, model, created_at)
			 VALUES ($1,$2,$3,'model-x', now() - $4::interval)`,
			userID, jobID, blob, age.String()); err != nil {
			t.Fatalf("seed analysis: %v", err)
		}
	}

	openID := seedJob("open-role", false)
	closedID := seedJob("closed-role", true)
	seedAnalysis(openID, 82, "Strong Fit", time.Hour)    // newer
	seedAnalysis(closedID, 44, "Weak Fit", 48*time.Hour) // older

	iss := auth.NewIssuer("test-secret", time.Hour)
	token, _ := iss.Issue(userID)

	store := resume.New(newFakeResumeBlobs(), &fakeResumeRepo{})
	if _, err := store.Put(ctx, userID, "text/plain", []byte("5y Go.")); err != nil {
		t.Fatalf("seed CV: %v", err)
	}

	h := &API{
		pool: pool, queries: queries, issuer: iss,
		userProfile: userprofile.New(ownedProfile()),
		resume:      store, matchAnalysis: matchanalysis.NewAnalyzer(nil), matchAnalysisCache: queries,
		credits: credits.NewStore(queries, pool, credits.Config{MonthlyGrant: 20, CostMatch: 1, CostTailor: 3}),
	}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/me/tracking/analyses", auth.RequireAuth(iss), h.ListMyAnalyses)

	req := httptest.NewRequest(fiber.MethodGet, "/api/v1/me/tracking/analyses", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	var body struct {
		Data []myAnalysisItem `json:"data"`
		Meta struct {
			Credits *credits.Balance `json:"credits"`
		} `json:"meta"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if len(body.Data) != 2 {
		t.Fatalf("got %d items, want 2", len(body.Data))
	}
	// Newest first: the open role (analysed 1h ago) precedes the closed one (48h ago).
	if body.Data[0].Slug != "open-role" || body.Data[0].OverallScore != 82 || body.Data[0].Verdict != "Strong Fit" || body.Data[0].Closed {
		t.Errorf("item0 = %+v, want open-role/82/Strong Fit/open", body.Data[0])
	}
	if body.Data[1].Slug != "closed-role" || !body.Data[1].Closed {
		t.Errorf("item1 = %+v, want closed-role with Closed=true", body.Data[1])
	}
	// The two seeded analyses were inserted directly (no debit), so the fresh monthly
	// grant is intact in meta.
	if body.Meta.Credits == nil || body.Meta.Credits.Remaining != 20 {
		t.Errorf("credits = %+v, want remaining=20", body.Meta.Credits)
	}
}
