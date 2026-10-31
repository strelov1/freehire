//go:build integration

package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/strelov1/freehire/internal/application/jobtracking"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
)

func TestListTrackedJobs_BoardDatesReflectRecordedActions(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	var userID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('board-dates@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	fixtures := []struct{ slug, saved, created, applied, stage, want string }{
		{"saved-first", "2026-10-01T12:00:00Z", "2026-10-03T12:00:00Z", "2026-09-20T12:00:00Z", "applied", "2026-10-01T12:00:00Z"},
		{"application-first", "2026-10-10T12:00:00Z", "2026-10-03T12:00:00Z", "2026-09-20T12:00:00Z", "applied", "2026-10-03T12:00:00Z"},
		{"preparing", "", "2026-10-04T12:00:00Z", "", "preparing", "2026-10-04T12:00:00Z"},
		{"saved-only", "2026-10-05T12:00:00Z", "", "", "", "2026-10-05T12:00:00Z"},
		{"viewed-only", "", "", "", "", ""},
	}
	for _, f := range fixtures {
		var jobID int64
		if err := pool.QueryRow(ctx, `INSERT INTO jobs (source, external_id, url, title, company, public_slug, application_deadline)
		 VALUES ('test', $1, 'https://example.test', 'Engineer', 'Acme', $1, '2026-11-12') RETURNING id`, f.slug).Scan(&jobID); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO user_jobs (user_id, job_id, viewed_at, saved_at)
		 VALUES ($1, $2, '2026-10-30T12:00:00Z', NULLIF($3, '')::timestamptz)`, userID, jobID, f.saved); err != nil {
			t.Fatal(err)
		}
		if f.created != "" {
			if _, err := pool.Exec(ctx, `INSERT INTO applications (user_id, job_id, company_slug, role_title, created_at, applied_at, stage)
			 VALUES ($1, $2, 'acme', 'Engineer', $3::timestamptz, NULLIF($4, '')::timestamptz, $5)`, userID, jobID, f.created, f.applied, f.stage); err != nil {
				t.Fatal(err)
			}
		}
	}
	var orphanID int64
	if err := pool.QueryRow(ctx, `INSERT INTO applications (user_id, company_slug, role_title, created_at, stage)
	 VALUES ($1, 'orphan', 'Engineer', '2026-10-06T12:00:00Z', 'preparing') RETURNING id`, userID).Scan(&orphanID); err != nil {
		t.Fatal(err)
	}
	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(userID, testTokenVersion)
	if err != nil {
		t.Fatal(err)
	}
	h := &trackingHandlers{tracking: jobtracking.New(jobtracking.NewQueriesRepository(db.New(pool), pool))}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/me/tracking", auth.RequireAuth(iss, testVersions), h.ListTrackedJobs)
	req := httptest.NewRequestWithContext(ctx, fiber.MethodGet, "/me/tracking?filter=all&compact=1", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	resp, err := app.Test(req, 30_000)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var body struct {
		Data []struct {
			ID             string `json:"id"`
			AddedToBoardAt string `json:"added_to_board_at"`
			Job            *struct {
				ApplicationDeadline string `json:"application_deadline"`
			} `json:"job"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{fmt.Sprintf("a%d", orphanID): "2026-10-06T12:00:00Z"}
	for _, f := range fixtures {
		want[f.slug] = f.want
	}
	if len(body.Data) != len(want) {
		t.Fatalf("rows = %d, want %d", len(body.Data), len(want))
	}
	for _, row := range body.Data {
		expected, ok := want[row.ID]
		if !ok {
			t.Errorf("unexpected tracked row %q", row.ID)
			continue
		}
		if expected == "" {
			if row.AddedToBoardAt != "" {
				t.Errorf("%s added = %q, want no board date", row.ID, row.AddedToBoardAt)
			}
		} else {
			added, err := time.Parse(time.RFC3339, row.AddedToBoardAt)
			if err != nil {
				t.Fatalf("%s added date: %v", row.ID, err)
			}
			expectedTime, err := time.Parse(time.RFC3339, expected)
			if err != nil {
				t.Fatal(err)
			}
			if !added.Equal(expectedTime) {
				t.Errorf("%s added = %q, want %q", row.ID, row.AddedToBoardAt, expected)
			}
		}
		if row.Job != nil && row.Job.ApplicationDeadline != "2026-11-12" {
			t.Errorf("%s deadline = %q", row.ID, row.Job.ApplicationDeadline)
		}
	}
}
