//go:build integration

// Integration tests for the dated apply, exercised through the endpoint against a real
// Postgres. The pieces are unit-tested apart — the window in internal/userjob, the routing in
// user_jobs_test.go, the re-dating statement in internal/db — but the transaction that runs
// MarkJobApplied and RedateApplication together is only itself here: whether the second
// statement can undo what the first wrote, and whether the counter survives both, are questions
// about their interaction and cannot be asked of either alone.
//
// Run with: go test -tags=integration ./internal/handler/
package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/jobtracking"
)

// datedApplyFixture seeds one user and one job, and mounts the apply route on the real
// repository.
func datedApplyFixture(t *testing.T, email, ext, slug string) (*fiber.App, string, int64, *pgxpool.Pool, context.Context) {
	t.Helper()
	pool := startPostgres(t)
	ctx := context.Background()
	queries := db.New(pool)

	var userID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email) VALUES ($1) RETURNING id`, email).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO jobs (source, external_id, url, title, public_slug)
		 VALUES ('test', $1, 'http://example.test', 'Go Dev', $2)`, ext, slug); err != nil {
		t.Fatalf("seed job: %v", err)
	}

	h := &trackingHandlers{tracking: jobtracking.New(jobtracking.NewQueriesRepository(queries, pool))}
	iss := auth.NewIssuer("test-secret", time.Hour)
	app := fiber.New()
	app.Post("/jobs/:slug/apply", auth.RequireAuth(iss, testVersions), h.MarkApplied)
	token, err := iss.Issue(userID, testTokenVersion)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	return app, token, userID, pool, ctx
}

func applyOn(t *testing.T, app *fiber.App, token, slug, body string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(fiber.MethodPost, "/jobs/"+slug+"/apply", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	out, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(out)
}

// applicationState reads back what the two records say, which is the whole point: a correction
// that moved one and not the other is the defect this path exists to avoid.
func applicationState(t *testing.T, pool *pgxpool.Pool, ctx context.Context, userID int64) (appliedAt time.Time, events int, occurred time.Time, appliedCount int) {
	t.Helper()
	row := pool.QueryRow(ctx, `SELECT applied_at FROM applications WHERE user_id = $1`, userID)
	if err := row.Scan(&appliedAt); err != nil {
		t.Fatalf("read application: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`SELECT count(*), min(occurred_at) FROM application_events
		  WHERE user_id = $1 AND kind = 'applied'`, userID).Scan(&events, &occurred); err != nil {
		t.Fatalf("read events: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`SELECT applied_count FROM jobs WHERE public_slug = (SELECT j.public_slug FROM jobs j
		   JOIN applications a ON a.job_id = j.id WHERE a.user_id = $1)`, userID).Scan(&appliedCount); err != nil {
		t.Fatalf("read applied_count: %v", err)
	}
	return appliedAt, events, occurred, appliedCount
}

// Applying to a job never applied to, with a day stated: the application is created already
// carrying that day, and so is its event. MarkJobApplied inserts the event with the date it was
// handed, and the re-date that follows must leave it alone rather than double-writing.
func TestDatedApply_CreatesTheApplicationOnTheStatedDay(t *testing.T) {
	app, token, userID, pool, ctx := datedApplyFixture(t, "dated-create@example.test", "dated-1", "dated-create-t35nijto")

	day := time.Now().UTC().AddDate(0, 0, -30)
	stated := day.Format("2006-01-02")
	status, body := applyOn(t, app, token, "dated-create-t35nijto", fmt.Sprintf(`{"applied_on":%q}`, stated))
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, body = %s", status, body)
	}

	appliedAt, events, occurred, count := applicationState(t, pool, ctx, userID)
	want := time.Date(day.Year(), day.Month(), day.Day(), 12, 0, 0, 0, time.UTC)
	if !appliedAt.UTC().Equal(want) {
		t.Errorf("applied_at = %v, want %v", appliedAt.UTC(), want)
	}
	if events != 1 {
		t.Errorf("applied events = %d, want 1", events)
	}
	if !occurred.UTC().Equal(want) {
		t.Errorf("event occurred_at = %v, want %v", occurred.UTC(), want)
	}
	if count != 1 {
		t.Errorf("applied_count = %d, want 1", count)
	}

	var got struct {
		Data struct {
			AppliedAt time.Time `json:"applied_at"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !got.Data.AppliedAt.UTC().Equal(want) {
		t.Errorf("response applied_at = %v, want %v", got.Data.AppliedAt.UTC(), want)
	}
}

// Correcting an application recorded today. MarkJobApplied's own rule keeps a date it already
// holds, so the correction rests entirely on the second statement — and the counter must not
// move for what is still one application.
func TestDatedApply_CorrectsAnApplicationAlreadyRecorded(t *testing.T) {
	app, token, userID, pool, ctx := datedApplyFixture(t, "dated-fix@example.test", "dated-2", "dated-fix-t35nijto")

	if status, body := applyOn(t, app, token, "dated-fix-t35nijto", ""); status != fiber.StatusOK {
		t.Fatalf("undated apply: status = %d, body = %s", status, body)
	}
	before, _, _, _ := applicationState(t, pool, ctx, userID)
	if before.UTC().Before(time.Now().UTC().AddDate(0, 0, -1)) {
		t.Fatalf("the undated apply did not stamp now: %v", before)
	}

	day := time.Now().UTC().AddDate(0, 0, -25)
	stated := day.Format("2006-01-02")
	if status, body := applyOn(t, app, token, "dated-fix-t35nijto", fmt.Sprintf(`{"applied_on":%q}`, stated)); status != fiber.StatusOK {
		t.Fatalf("dated apply: status = %d, body = %s", status, body)
	}

	appliedAt, events, occurred, count := applicationState(t, pool, ctx, userID)
	want := time.Date(day.Year(), day.Month(), day.Day(), 12, 0, 0, 0, time.UTC)
	if !appliedAt.UTC().Equal(want) {
		t.Errorf("applied_at = %v, want the corrected %v", appliedAt.UTC(), want)
	}
	if !occurred.UTC().Equal(want) {
		t.Errorf("event occurred_at = %v, want the corrected %v", occurred.UTC(), want)
	}
	if events != 1 {
		t.Errorf("applied events = %d, want 1: a correction repairs the event, it does not add one", events)
	}
	if count != 1 {
		t.Errorf("applied_count = %d, want 1: correcting a date is not a second application", count)
	}
}

// The window is the service's, and it holds through the endpoint against a real database:
// nothing is written when the date is refused.
func TestDatedApply_RefusesAnUnbelievableDateWithoutWriting(t *testing.T) {
	app, token, userID, pool, ctx := datedApplyFixture(t, "dated-bad@example.test", "dated-3", "dated-bad-t35nijto")

	future := time.Now().UTC().AddDate(0, 0, 3).Format("2006-01-02")
	if status, _ := applyOn(t, app, token, "dated-bad-t35nijto", fmt.Sprintf(`{"applied_on":%q}`, future)); status != fiber.StatusBadRequest {
		t.Errorf("status = %d, want 400", status)
	}
	var applications int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM applications WHERE user_id = $1`, userID).Scan(&applications); err != nil {
		t.Fatalf("count applications: %v", err)
	}
	if applications != 0 {
		t.Errorf("applications = %d, want 0: a refused date must write nothing", applications)
	}
}
