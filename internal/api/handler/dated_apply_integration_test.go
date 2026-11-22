//go:build integration

// Integration tests for the dated apply, exercised through the endpoint against a real
// Postgres. The pieces are unit-tested apart — the window in internal/application/userjob, the routing in
// user_jobs_test.go, the re-dating statement in internal/platform/db — but the transaction that runs
// MarkJobApplied and RedateApplication together is only itself here: whether the second
// statement can undo what the first wrote, and whether the counter survives both, are questions
// about their interaction and cannot be asked of either alone.
//
// Run with: go test -tags=integration ./internal/api/handler/
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

	"github.com/strelov1/freehire/internal/application/jobtracking"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
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
	app.Post("/api/v1/jobs/:slug/apply", auth.RequireAuth(iss, testVersions), newContract(contractHandlers{trackingHandlers: h}))
	app.Delete("/api/v1/me/applications/:id", auth.RequireAuth(iss, testVersions), newContract(contractHandlers{trackingHandlers: h}))
	token, err := iss.Issue(userID, testTokenVersion)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	return app, token, userID, pool, ctx
}

func applyOn(t *testing.T, app *fiber.App, token, slug, body string) (int, string) {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/api/v1/jobs/"+slug+"/apply", strings.NewReader(body))
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

// The apply response carries the id of the application it wrote, on the undated and the dated
// path alike, and deleting by it removes that application but leaves the saved mark: the
// match card's Undo must not unsave a job the candidate saved earlier.
func TestApply_ReturnsTheApplicationIDAndUndoKeepsTheSavedMark(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"undated", ``},
		{"dated", fmt.Sprintf(`{"applied_on":%q}`, time.Now().UTC().AddDate(0, 0, -3).Format("2006-01-02"))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			slug := "apply-id-" + tc.name + "-t35nijto"
			app, token, userID, pool, ctx := datedApplyFixture(t, tc.name+"-id@example.test", "id-"+tc.name, slug)
			if _, err := pool.Exec(ctx,
				`INSERT INTO user_jobs (user_id, job_id, saved_at)
				 SELECT $1, id, now() FROM jobs WHERE public_slug = $2`, userID, slug); err != nil {
				t.Fatalf("seed saved mark: %v", err)
			}
			status, body := applyOn(t, app, token, slug, tc.body)
			if status != fiber.StatusOK {
				t.Fatalf("status = %d, body = %s", status, body)
			}
			var out struct {
				Data struct {
					ApplicationID string `json:"application_id"`
				} `json:"data"`
			}
			if err := json.Unmarshal([]byte(body), &out); err != nil {
				t.Fatalf("decode: %v", err)
			}
			var id int64
			if err := pool.QueryRow(ctx, `SELECT id FROM applications WHERE user_id = $1`, userID).Scan(&id); err != nil {
				t.Fatalf("read application: %v", err)
			}
			if want := fmt.Sprintf("a%d", id); out.Data.ApplicationID != want {
				t.Fatalf("application_id = %q, want %q", out.Data.ApplicationID, want)
			}

			req := httptest.NewRequestWithContext(context.Background(), fiber.MethodDelete,
				"/api/v1/me/applications/"+out.Data.ApplicationID, nil)
			req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token}) //nolint:gosec // a request cookie: the Set-Cookie attributes do not apply
			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("Test: %v", err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode != fiber.StatusOK {
				t.Fatalf("undo status = %d", resp.StatusCode)
			}
			var applications, saved int
			_ = pool.QueryRow(ctx, `SELECT count(*) FROM applications WHERE user_id = $1`, userID).Scan(&applications)
			_ = pool.QueryRow(ctx, `SELECT count(*) FROM user_jobs WHERE user_id = $1 AND saved_at IS NOT NULL`, userID).Scan(&saved)
			if applications != 0 || saved != 1 {
				t.Errorf("applications = %d (want 0), saved marks = %d (want 1)", applications, saved)
			}
		})
	}
}

// An application the candidate was only preparing (stage and notes, no applied_at) already exists:
// applying promotes it but returns no application_id, so the card's Undo cannot delete that history.
func TestApply_OmitsTheApplicationIDWhenItExisted(t *testing.T) {
	slug := "apply-existed-t35nijto"
	app, token, userID, pool, ctx := datedApplyFixture(t, "existed-id@example.test", "existed-1", slug)
	if _, err := pool.Exec(ctx,
		`INSERT INTO applications (user_id, job_id, company_slug, role_title, stage, notes)
		 SELECT $1, id, company_slug, title, 'interview', 'call booked' FROM jobs WHERE public_slug = $2`, userID, slug); err != nil {
		t.Fatalf("seed application: %v", err)
	}
	status, body := applyOn(t, app, token, slug, ``)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, body = %s", status, body)
	}
	if strings.Contains(body, "application_id") {
		t.Errorf("response carries application_id for a pre-existing application: %s", body)
	}
	var stage, notes string
	if err := pool.QueryRow(ctx, `SELECT stage, notes FROM applications WHERE user_id = $1`, userID).Scan(&stage, &notes); err != nil {
		t.Fatalf("read application: %v", err)
	}
	if stage != "interview" || notes != "call booked" {
		t.Errorf("stage/notes = %q/%q, want interview/call booked", stage, notes)
	}
}
