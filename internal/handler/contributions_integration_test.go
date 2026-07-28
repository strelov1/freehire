//go:build integration

// Integration test for GET /api/v1/me/contributions, the caller's own list.
//
// Submitting no longer has an endpoint of its own: every surface enters through the shared
// intake (POST /jobs/resolve), so the recording, dedup, reward and review-queue behaviour is
// exercised by TestResolveJobEndpoint and the contribution package's own tests rather than
// duplicated here. Run with: go test -tags=integration ./internal/handler/
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
	"github.com/strelov1/freehire/internal/contribution"
	"github.com/strelov1/freehire/internal/db"
)

func TestMyContributionsList(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	var mine, theirs int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('contrib@example.test') RETURNING id`).Scan(&mine); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('other@example.test') RETURNING id`).Scan(&theirs); err != nil {
		t.Fatalf("seed other user: %v", err)
	}

	queries := db.New(pool)
	repo := contribution.NewQueriesRepository(pool, queries)
	if _, _, err := repo.Record(ctx, contribution.RecordInput{
		SubmittedBy: mine, URL: "https://jobs.ashbyhq.com/acme", Source: "ashby", Board: "acme",
		Surface: contribution.SurfaceCLI,
	}); err != nil {
		t.Fatalf("record own: %v", err)
	}
	if _, err := repo.RecordReview(ctx, mine, "https://example.com/careers/1", contribution.SurfaceWeb); err != nil {
		t.Fatalf("record review: %v", err)
	}
	if _, _, err := repo.Record(ctx, contribution.RecordInput{
		SubmittedBy: theirs, URL: "https://jobs.lever.co/globex", Source: "lever", Board: "globex",
		Surface: contribution.SurfaceWeb,
	}); err != nil {
		t.Fatalf("record other user's: %v", err)
	}

	iss := auth.NewIssuer("test-secret", time.Hour)
	cookie, _ := iss.Issue(mine, testTokenVersion)
	ch := &contributionHandlers{contribution: contribution.New(repo, nil)}

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/me/contributions", auth.RequireAuthOrKey(iss, testVersions, apiKeys{queries}), ch.ListMyContributions)

	r := httptest.NewRequest(fiber.MethodGet, "/api/v1/me/contributions", nil)
	r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
	resp, err := app.Test(r)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var out struct {
		Data []struct {
			URL     string `json:"url"`
			Source  string `json:"source"`
			Board   string `json:"board"`
			Status  string `json:"status"`
			Surface string `json:"surface"`
		} `json:"data"`
	}
	raw, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(out.Data) != 2 {
		t.Fatalf("list has %d rows, want the caller's 2 and nobody else's: %+v", len(out.Data), out.Data)
	}
	// Newest first: the review row was recorded after the board row.
	if out.Data[0].Status != contribution.StatusReview || out.Data[0].Board != "" {
		t.Errorf("first row = %+v, want the review row with no board", out.Data[0])
	}
	if out.Data[1].Board != "acme" || out.Data[1].Surface != contribution.SurfaceCLI {
		t.Errorf("second row = %+v, want the ashby/acme board submitted from the CLI", out.Data[1])
	}
	for _, row := range out.Data {
		if row.Board == "globex" {
			t.Error("another user's contribution leaked into the caller's list")
		}
	}
}
