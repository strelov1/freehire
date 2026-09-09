//go:build integration

// Integration tests for the company process-report HTTP flow against a real Postgres:
// the status for each refusal (401 anonymous, 400 unknown kind, 404 unknown company,
// 409 a second live report, 429 over the cap), the count that comes back on every
// write so a caller can render the badge from the answer it already has, and that
// Mine opens the write surface in the right state instead of finding out by being
// refused. Run with: go test -tags=integration ./internal/api/handler/
package handler

import (
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/engage/processreport"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
)

func newProcessReportApp(queries *db.Queries, pool *pgxpool.Pool, iss *auth.Issuer, cfg processreport.Config) *fiber.App {
	h := newCompanyProcessReportHandlers(processreport.New(queries, pool, cfg))
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	mw := middleware{cookie: auth.RequireAuth(iss, testVersions)}
	h.register(app.Group("/api/v1"), mw)
	return app
}

// The three helpers below take the REQUEST, not the response. bodyclose reports at the
// call site of whatever returns an *http.Response, and it cannot see a close that
// happens across a function boundary — so handing the response to a closing helper
// still reads as a leak. Making the request inside the helper puts the close where the
// analyser looks for it, and the tests read as intent rather than plumbing.

// processStatus performs the request and returns its status alone.
func processStatus(t *testing.T, app *fiber.App, method, path, cookie, body string) int {
	t.Helper()
	resp := doFeedbackRequest(t, app, method, path, cookie, body)
	_, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp.StatusCode
}

// processResult performs the request and decodes the write response, failing the test
// when the status is not the expected one — so a wrong status names itself instead of
// surfacing as an unmarshal error.
func processResult(t *testing.T, app *fiber.App, method, path, cookie, body string, want int) processReportResponse {
	t.Helper()
	resp := doFeedbackRequest(t, app, method, path, cookie, body)
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != want {
		t.Fatalf("%s %s: want %d, got %d (%s)", method, path, want, resp.StatusCode, raw)
	}
	var env struct {
		Data processReportResponse `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	return env.Data
}

// processKinds performs the request and decodes the caller's held kinds.
func processKinds(t *testing.T, app *fiber.App, path, cookie string) []string {
	t.Helper()
	resp := doFeedbackRequest(t, app, fiber.MethodGet, path, cookie, "")
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var env struct {
		Data []string `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	return env.Data
}

func TestCompanyProcessReportEndpoints(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	queries := db.New(pool)

	var userID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email) VALUES ('reporter@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO companies (slug, name) VALUES ('acme-ai', 'Acme AI')`); err != nil {
		t.Fatalf("seed company: %v", err)
	}

	iss := auth.NewIssuer("test-secret", time.Hour)
	cookie, err := iss.Issue(userID, testTokenVersion)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	app := newProcessReportApp(queries, pool, iss, processreport.Config{})

	const path = "/api/v1/companies/acme-ai/process-reports"
	const filed = `{"kind":"ai_interview"}`

	// Anonymous filing is rejected: this is a claim attributed to a person.
	if got := processStatus(t, app, fiber.MethodPost, path, "", filed); got != fiber.StatusUnauthorized {
		t.Fatalf("anon file: want 401, got %d", got)
	}

	// Before filing, the caller holds nothing — and it is an empty array, never null,
	// so a client can render it without a nil check.
	if kinds := processKinds(t, app, path+"/mine", cookie); len(kinds) != 0 {
		t.Fatalf("mine before filing = %v, want empty", kinds)
	}

	// An unknown kind is refused before any write. The vocabulary decides what the
	// badge renders, so it cannot be open.
	if got := processStatus(t, app, fiber.MethodPost, path, cookie, `{"kind":"unpaid_test_task"}`); got != fiber.StatusBadRequest {
		t.Fatalf("unknown kind: want 400, got %d", got)
	}

	// An unknown company is a clean 404, not the FK's opaque constraint violation.
	if got := processStatus(t, app, fiber.MethodPost,
		"/api/v1/companies/no-such-co/process-reports", cookie, filed); got != fiber.StatusNotFound {
		t.Fatalf("unknown company: want 404, got %d", got)
	}

	// Filing answers 201 with the company's resulting count, so the caller renders the
	// badge from this response rather than re-reading the company.
	if got := processResult(t, app, fiber.MethodPost, path, cookie, filed, fiber.StatusCreated); got.Count != 1 || got.Kind != "ai_interview" {
		t.Fatalf("file response = %+v, want kind ai_interview count 1", got)
	}

	// One report is enough — there is no contributor gate to clear.
	kinds := processKinds(t, app, path+"/mine", cookie)
	if len(kinds) != 1 || kinds[0] != "ai_interview" {
		t.Fatalf("mine after filing = %v, want [ai_interview]", kinds)
	}

	// A second live report is 409, not a silent no-op: the caller asked to file and
	// deserves to know their report already stands.
	if got := processStatus(t, app, fiber.MethodPost, path, cookie, filed); got != fiber.StatusConflict {
		t.Fatalf("duplicate file: want 409, got %d", got)
	}

	// Withdrawing returns the lowered count; the kind rides as a query param.
	if got := processResult(t, app, fiber.MethodDelete, path+"?kind=ai_interview", cookie, "", fiber.StatusOK); got.Count != 0 {
		t.Fatalf("count after retraction = %d, want 0", got.Count)
	}

	// Withdrawing again has nothing to withdraw.
	if got := processStatus(t, app, fiber.MethodDelete, path+"?kind=ai_interview", cookie, ""); got != fiber.StatusNotFound {
		t.Fatalf("second retract: want 404, got %d", got)
	}

	// Re-filing revives the same row, so the count returns to one rather than two —
	// a retraction is not a way to file repeatedly.
	if got := processResult(t, app, fiber.MethodPost, path, cookie, filed, fiber.StatusCreated); got.Count != 1 {
		t.Fatalf("count after revival = %d, want 1", got.Count)
	}
}

func TestCompanyProcessReportRateLimit(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	queries := db.New(pool)

	var userID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email) VALUES ('prolific@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	for _, slug := range []string{"rate-one", "rate-two"} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO companies (slug, name) VALUES ($1, $1)`, slug); err != nil {
			t.Fatalf("seed company %s: %v", slug, err)
		}
	}

	iss := auth.NewIssuer("test-secret", time.Hour)
	cookie, err := iss.Issue(userID, testTokenVersion)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	// Cap of one, so the second company is refused. The shipped default is far looser —
	// the cap exists so one account cannot label the catalogue, not to ration honest
	// reporting.
	app := newProcessReportApp(queries, pool, iss, processreport.Config{Window: time.Hour, Cap: 1})

	const filed = `{"kind":"ai_interview"}`
	if got := processStatus(t, app, fiber.MethodPost,
		"/api/v1/companies/rate-one/process-reports", cookie, filed); got != fiber.StatusCreated {
		t.Fatalf("first: want 201, got %d", got)
	}
	if got := processStatus(t, app, fiber.MethodPost,
		"/api/v1/companies/rate-two/process-reports", cookie, filed); got != fiber.StatusTooManyRequests {
		t.Fatalf("over the cap: want 429, got %d", got)
	}
}
