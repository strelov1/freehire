//go:build integration

// Integration tests for the company-feedback HTTP flow against a real Postgres:
// create/edit/delete and the recomputed company summary each returns, the two
// Mine bugs an earlier review caught (a bad slug silently reading as "no
// feedback yet", and the caller's own live review reporting itself as
// "[deleted]"), the per-user new-review rate limit (an edit never counts
// against it), the body-length cap, and the report/hide moderation lever (a
// hidden review drops out of List/Count/the company average immediately).
// Run with: go test -tags=integration ./internal/handler/
package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/community"
	"github.com/strelov1/freehire/internal/companyfeedback"
	"github.com/strelov1/freehire/internal/db"
)

func newCompanyFeedbackApp(queries *db.Queries, pool *pgxpool.Pool, iss *auth.Issuer, cfg companyfeedback.Config) *fiber.App {
	communityRepo := community.NewQueriesRepository(queries)
	communitySvc := community.New(communityRepo, communityRepo, community.Config{})
	svc := companyfeedback.New(queries, pool, communityPersonas{svc: communitySvc}, cfg)
	h := newCompanyFeedbackHandlers(svc)
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	mw := middleware{
		cookie:    auth.RequireAuth(iss, testVersions),
		moderator: auth.RequireRole(queries, "moderator"),
	}
	h.register(app.Group("/api/v1"), mw)
	return app
}

type feedbackResp struct {
	ID           int64  `json:"id"`
	Author       string `json:"author"`
	Rating       int16  `json:"rating"`
	FeedbackType string `json:"feedback_type"`
	Body         string `json:"body"`
	Company      *struct {
		Count     int32    `json:"feedback_count"`
		RatingAvg *float32 `json:"feedback_rating_avg"`
	} `json:"company"`
}

func doFeedbackRequest(t *testing.T, app *fiber.App, method, path, cookie, body string) *http.Response {
	t.Helper()
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	if cookie != "" {
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
	}
	resp, err := app.Test(r, 10_000)
	if err != nil {
		t.Fatalf("app.Test %s %s: %v", method, path, err)
	}
	return resp
}

func decodeFeedback(t *testing.T, resp *http.Response) feedbackResp {
	t.Helper()
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var env struct {
		Data feedbackResp `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	return env.Data
}

func TestCompanyFeedbackEndpoints(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	queries := db.New(pool)

	var userID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('reviewer@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO companies (slug, name) VALUES ('acme', 'Acme')`); err != nil {
		t.Fatalf("seed company: %v", err)
	}

	iss := auth.NewIssuer("test-secret", time.Hour)
	cookie, err := iss.Issue(userID, testTokenVersion)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	app := newCompanyFeedbackApp(queries, pool, iss, companyfeedback.Config{})

	// Unauthenticated write is rejected.
	if resp := doFeedbackRequest(t, app, fiber.MethodPost, "/api/v1/companies/acme/feedback", "",
		`{"rating":5,"feedback_type":"culture","body":"great place"}`); resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("anon upsert: want 401, got %d", resp.StatusCode)
	}

	// Create: 201, the row, and the company's recomputed summary in the same response.
	resp := doFeedbackRequest(t, app, fiber.MethodPost, "/api/v1/companies/acme/feedback", cookie,
		`{"rating":5,"feedback_type":"culture","body":"great place"}`)
	if resp.StatusCode != fiber.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("create: want 201, got %d (%s)", resp.StatusCode, body)
	}
	created := decodeFeedback(t, resp)
	if created.Author == "" || strings.EqualFold(created.Author, "[deleted]") {
		t.Fatalf("create: author reported as deleted for a live reviewer: %+v", created)
	}
	if created.Company == nil || created.Company.Count != 1 || created.Company.RatingAvg == nil || *created.Company.RatingAvg != 5 {
		t.Fatalf("create: want company summary {1, 5}, got %+v", created.Company)
	}

	// Mine: real handle, not "[deleted]" — the bug an earlier review caught.
	mine := decodeFeedback(t, doFeedbackRequest(t, app, fiber.MethodGet, "/api/v1/companies/acme/feedback/mine", cookie, ""))
	if mine.Author == "" || strings.EqualFold(mine.Author, "[deleted]") {
		t.Fatalf("mine: caller's own live review reported as deleted: %+v", mine)
	}

	// Mine on an unknown company: 404, not a silent {"data":null} — the other bug.
	if resp := doFeedbackRequest(t, app, fiber.MethodGet, "/api/v1/companies/ghost/feedback/mine", cookie, ""); resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("mine unknown company: want 404, got %d", resp.StatusCode)
	}

	// Edit-by-resubmit: still one row, updated rating, company summary reflects it.
	edited := decodeFeedback(t, doFeedbackRequest(t, app, fiber.MethodPost, "/api/v1/companies/acme/feedback", cookie,
		`{"rating":3,"feedback_type":"culture","body":"actually mixed"}`))
	if edited.Rating != 3 || edited.Company.Count != 1 || *edited.Company.RatingAvg != 3 {
		t.Fatalf("edit: want rating 3, company {1,3}, got %+v", edited)
	}

	// Empty body -> 422; over-long body -> 422.
	if resp := doFeedbackRequest(t, app, fiber.MethodPost, "/api/v1/companies/acme/feedback", cookie,
		`{"rating":4,"feedback_type":"culture","body":"   "}`); resp.StatusCode != fiber.StatusUnprocessableEntity {
		t.Fatalf("empty body: want 422, got %d", resp.StatusCode)
	}
	huge, _ := json.Marshal(map[string]any{"rating": 4, "feedback_type": "culture", "body": strings.Repeat("a", 5001)})
	if resp := doFeedbackRequest(t, app, fiber.MethodPost, "/api/v1/companies/acme/feedback", cookie, string(huge)); resp.StatusCode != fiber.StatusUnprocessableEntity {
		t.Fatalf("over-long body: want 422, got %d", resp.StatusCode)
	}

	// Delete: 200 with the recomputed (now empty) summary, not a bare 204.
	delResp := doFeedbackRequest(t, app, fiber.MethodDelete, "/api/v1/companies/acme/feedback", cookie, "")
	if delResp.StatusCode != fiber.StatusOK {
		t.Fatalf("delete: want 200, got %d", delResp.StatusCode)
	}
	var delEnv struct {
		Data struct {
			Count     int32    `json:"feedback_count"`
			RatingAvg *float32 `json:"feedback_rating_avg"`
		} `json:"data"`
	}
	raw, _ := io.ReadAll(delResp.Body)
	if err := json.Unmarshal(raw, &delEnv); err != nil {
		t.Fatalf("decode delete response %s: %v", raw, err)
	}
	if delEnv.Data.Count != 0 || delEnv.Data.RatingAvg != nil {
		t.Fatalf("delete: want empty summary, got %+v", delEnv.Data)
	}
}

// TestCompanyFeedbackRateLimit mirrors community's over-the-cap test: a tiny
// Config.Cap trips after that many NEW companies, but editing an
// already-reviewed one never counts against it.
func TestCompanyFeedbackRateLimit(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	queries := db.New(pool)

	var userID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('ratelimited@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	for _, slug := range []string{"acme", "globex", "initech"} {
		if _, err := pool.Exec(ctx, `INSERT INTO companies (slug, name) VALUES ($1, $1)`, slug); err != nil {
			t.Fatalf("seed company %q: %v", slug, err)
		}
	}

	iss := auth.NewIssuer("test-secret", time.Hour)
	cookie, err := iss.Issue(userID, testTokenVersion)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	app := newCompanyFeedbackApp(queries, pool, iss, companyfeedback.Config{Cap: 2})

	post := func(slug string) *http.Response {
		return doFeedbackRequest(t, app, fiber.MethodPost, "/api/v1/companies/"+slug+"/feedback", cookie,
			`{"rating":4,"feedback_type":"other","body":"fine"}`)
	}
	if resp := post("acme"); resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("1st new review: want 201, got %d", resp.StatusCode)
	}
	if resp := post("globex"); resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("2nd new review: want 201, got %d", resp.StatusCode)
	}
	if resp := post("initech"); resp.StatusCode != fiber.StatusTooManyRequests {
		t.Fatalf("3rd new review (over cap): want 429, got %d", resp.StatusCode)
	}
	// Editing one of the first two never counts against the cap.
	if resp := post("acme"); resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("edit of an existing review after hitting cap: want 201, got %d", resp.StatusCode)
	}
}

// TestCompanyFeedbackReportAndHide covers the light report/moderation lever: a
// reader flags a review, a moderator hides it, and a hidden review drops out
// of the public list/count/company average in the same step.
func TestCompanyFeedbackReportAndHide(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	queries := db.New(pool)

	var authorID, reporterID, modID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('author@example.test') RETURNING id`).Scan(&authorID); err != nil {
		t.Fatalf("seed author: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('reporter@example.test') RETURNING id`).Scan(&reporterID); err != nil {
		t.Fatalf("seed reporter: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email, role) VALUES ('mod2@example.test', 'moderator') RETURNING id`).Scan(&modID); err != nil {
		t.Fatalf("seed moderator: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO companies (slug, name) VALUES ('acme', 'Acme')`); err != nil {
		t.Fatalf("seed company: %v", err)
	}

	iss := auth.NewIssuer("test-secret", time.Hour)
	authorCookie, _ := iss.Issue(authorID, testTokenVersion)
	reporterCookie, _ := iss.Issue(reporterID, testTokenVersion)
	modCookie, _ := iss.Issue(modID, testTokenVersion)

	app := newCompanyFeedbackApp(queries, pool, iss, companyfeedback.Config{})

	created := decodeFeedback(t, doFeedbackRequest(t, app, fiber.MethodPost, "/api/v1/companies/acme/feedback", authorCookie,
		`{"rating":1,"feedback_type":"other","body":"unfounded claim"}`))

	// Bad reason -> 400. Unknown id -> 404.
	if resp := doFeedbackRequest(t, app, fiber.MethodPost, "/api/v1/company-feedback/999999/report", reporterCookie, `{"reason":"spam"}`); resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("report unknown id: want 404, got %d", resp.StatusCode)
	}
	reportPath := "/api/v1/company-feedback/" + strconv.FormatInt(created.ID, 10) + "/report"
	if resp := doFeedbackRequest(t, app, fiber.MethodPost, reportPath, reporterCookie, `{"reason":"sideways"}`); resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("bad reason: want 400, got %d", resp.StatusCode)
	}
	if resp := doFeedbackRequest(t, app, fiber.MethodPost, reportPath, reporterCookie, `{"reason":"false_information"}`); resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("report: want 204, got %d", resp.StatusCode)
	}
	// A second report from the same user is a silent no-op, not an error.
	if resp := doFeedbackRequest(t, app, fiber.MethodPost, reportPath, reporterCookie, `{"reason":"false_information"}`); resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("duplicate report: want 204, got %d", resp.StatusCode)
	}

	// A non-moderator cannot hide.
	hidePath := "/api/v1/company-feedback/" + strconv.FormatInt(created.ID, 10) + "/hide"
	if resp := doFeedbackRequest(t, app, fiber.MethodPost, hidePath, reporterCookie, ""); resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("non-moderator hide: want 403, got %d", resp.StatusCode)
	}

	// The moderator queue lists it, with the report reflected.
	queueResp := doFeedbackRequest(t, app, fiber.MethodGet, "/api/v1/company-feedback/reported", modCookie, "")
	var queueEnv struct {
		Data []struct {
			ID          int64 `json:"id"`
			ReportCount int64 `json:"report_count"`
		} `json:"data"`
	}
	raw, _ := io.ReadAll(queueResp.Body)
	if err := json.Unmarshal(raw, &queueEnv); err != nil {
		t.Fatalf("decode queue %s: %v", raw, err)
	}
	if len(queueEnv.Data) != 1 || queueEnv.Data[0].ID != created.ID || queueEnv.Data[0].ReportCount != 1 {
		t.Fatalf("moderator queue: want one entry with report_count 1, got %+v", queueEnv.Data)
	}

	// The moderator hides it.
	if resp := doFeedbackRequest(t, app, fiber.MethodPost, hidePath, modCookie, ""); resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("hide: want 204, got %d", resp.StatusCode)
	}
	// Idempotent.
	if resp := doFeedbackRequest(t, app, fiber.MethodPost, hidePath, modCookie, ""); resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("re-hide: want 204, got %d", resp.StatusCode)
	}

	// The hidden review drops out of the public list, the count, and the
	// company's materialized average in the same step.
	var listEnv struct {
		Data []feedbackResp `json:"data"`
		Meta struct {
			Total int64 `json:"total"`
		} `json:"meta"`
	}
	listResp := doFeedbackRequest(t, app, fiber.MethodGet, "/api/v1/companies/acme/feedback", "", "")
	raw, _ = io.ReadAll(listResp.Body)
	if err := json.Unmarshal(raw, &listEnv); err != nil {
		t.Fatalf("decode list %s: %v", raw, err)
	}
	if len(listEnv.Data) != 0 || listEnv.Meta.Total != 0 {
		t.Fatalf("list after hide: want empty, got %+v (total %d)", listEnv.Data, listEnv.Meta.Total)
	}
	var count int32
	var avg *float32
	if err := pool.QueryRow(ctx, `SELECT feedback_count, feedback_rating_avg FROM companies WHERE slug = 'acme'`).Scan(&count, &avg); err != nil {
		t.Fatalf("read company counters: %v", err)
	}
	if count != 0 || avg != nil {
		t.Fatalf("company counters after hide: want {0, nil}, got {%d, %v}", count, avg)
	}

	// The owner can still see (and would still be able to edit) their own hidden review.
	mine := decodeFeedback(t, doFeedbackRequest(t, app, fiber.MethodGet, "/api/v1/companies/acme/feedback/mine", authorCookie, ""))
	if mine.ID != created.ID {
		t.Fatalf("owner's mine after hide: want their review still visible to them, got %+v", mine)
	}
}
