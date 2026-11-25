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

	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
)

func TestEmployerVacancyAnalytics(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	user := func(email string) int64 {
		t.Helper()
		var id int64
		if err := pool.QueryRow(ctx, "INSERT INTO users (email) VALUES ($1) RETURNING id", email).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	owner := user("owner@analytics.test")
	recruiter := user("recruiter@analytics.test")
	stranger := user("stranger@other.test")
	exec(`INSERT INTO company_accounts (user_id, company_slug, company_name, work_email, status)
		VALUES ($1, 'analytics', 'Analytics', 'owner@analytics.test', 'active')`, owner)
	exec(`INSERT INTO company_account_members (company_slug, user_id, role, work_email)
		VALUES ('analytics', $1, 'recruiter', 'recruiter@analytics.test')`, recruiter)
	exec(`INSERT INTO jobs (source, external_id, url, title, company, company_slug, public_slug, created_by)
		VALUES ('employer', 'analytics-role', '', 'Engineer', 'Analytics', 'analytics', 'analytics-role', $1),
		       ('employer', 'another-role', '', 'Another role', 'Analytics', 'analytics', 'another-role', $1)`, owner)
	exec(`INSERT INTO job_daily_views (day, job_id, page_uniques, uniques)
		SELECT (now() AT TIME ZONE 'UTC')::date - sample.age, j.id, sample.views, sample.total
		FROM jobs j CROSS JOIN (VALUES (1, 2, 900), (0, 40, 40), (31, 50, 50)) AS sample(age, views, total)
		WHERE j.public_slug = 'analytics-role'`)
	exec(`INSERT INTO direct_applications (job_id, user_id, sent_at)
		SELECT id, $1, now() - interval '1 day' FROM jobs WHERE public_slug = 'analytics-role'`, stranger)
	exec(`INSERT INTO direct_applications (job_id, user_id)
		SELECT id, $1 FROM jobs WHERE public_slug = 'analytics-role'`, user("pending@other.test"))
	exec(`INSERT INTO direct_applications (job_id, user_id, sent_at)
		SELECT id, $1, now() - interval '31 days' FROM jobs WHERE public_slug = 'analytics-role'`, user("old@other.test"))
	exec(`INSERT INTO direct_applications (job_id, user_id, sent_at)
		SELECT id, $1, now() FROM jobs WHERE public_slug = 'analytics-role'`, user("today@other.test"))

	h := newEmployerHandlers(db.New(pool), nil)
	issuer := auth.NewIssuer("test-secret", time.Hour)
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	h.register(app.Group("/api/v1"), middleware{cookie: auth.RequireAuth(issuer, testVersions)},
		newContract(contractHandlers{employerHandlers: h}))

	request := func(method, path string, userID int64, want int) (http.Header, []byte) {
		t.Helper()
		req := httptest.NewRequestWithContext(ctx, method, "/api/v1"+path, nil)
		if userID != 0 {
			token, err := issuer.Issue(userID, testTokenVersion)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Cookie", auth.CookieName+"="+token)
		}
		res, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()
		body, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != want {
			t.Fatalf("%s %s returned %d, want %d", method, path, res.StatusCode, want)
		}
		return res.Header, body
	}

	const click = "/jobs/analytics-role/apply-click"
	const report = "/employer/jobs/analytics-role/analytics"
	request(http.MethodPost, click, 0, 204)
	request(http.MethodPost, "/jobs/another-role/apply-click", 0, 204)
	// Move a genuine click to yesterday so the report's completed-day window can see it.
	exec(`UPDATE employer_vacancy_apply_clicks SET day = day - 1
		WHERE job_id = (SELECT id FROM jobs WHERE public_slug = 'analytics-role')`)
	request(http.MethodPost, click, 0, 204)
	exec("UPDATE jobs SET closed_at = now() WHERE public_slug = 'analytics-role'")
	request(http.MethodPost, click, 0, 204)
	var todayClicks int64
	if err := pool.QueryRow(ctx, `SELECT clicks FROM employer_vacancy_apply_clicks
		WHERE day = (now() AT TIME ZONE 'UTC')::date
		AND job_id = (SELECT id FROM jobs WHERE public_slug = 'analytics-role')`).Scan(&todayClicks); err != nil {
		t.Fatal(err)
	}
	if todayClicks != 1 {
		t.Errorf("today's clicks = %d, want 1 before the vacancy closed", todayClicks)
	}

	for _, id := range []int64{owner, recruiter} {
		headers, response := request(http.MethodGet, report, id, 200)
		if got := headers.Get("Cache-Control"); got != "private, no-store" {
			t.Errorf("Cache-Control = %q", got)
		}
		var body struct {
			Data struct {
				Views        int64 `json:"views"`
				ApplyClicks  int64 `json:"apply_clicks"`
				Applications int64 `json:"applications"`
			} `json:"data"`
		}
		if err := json.Unmarshal(response, &body); err != nil {
			t.Fatal(err)
		}
		if body.Data.Views != 2 || body.Data.ApplyClicks != 1 || body.Data.Applications != 1 {
			t.Errorf("analytics for %d = %+v, want 2 views, 1 click, 1 delivered application", id, body.Data)
		}
	}
	request(http.MethodGet, report, 0, 401)
	request(http.MethodGet, report, stranger, 404)
	exec("UPDATE company_accounts SET status = 'revoked' WHERE user_id = $1", owner)
	request(http.MethodGet, report, recruiter, 404)
}

func TestEmployerVacancyOutboundClicks(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	var owner, stranger int64
	for email, id := range map[string]*int64{"owner@clicks.test": &owner, "stranger@other.test": &stranger} {
		if err := pool.QueryRow(ctx, "INSERT INTO users (email) VALUES ($1) RETURNING id", email).Scan(id); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO company_accounts (user_id, company_slug, company_name, work_email, status)
		VALUES ($1, 'clicks', 'Clicks', 'owner@clicks.test', 'active')`, owner)
	exec(`INSERT INTO jobs (source, external_id, url, title, company, company_slug, public_slug, created_by)
		VALUES ('employer', 'clicks-role', 'https://clicks.test/job', 'Engineer', 'Clicks', 'clicks', 'clicks-role', $1)`, owner)
	// Two days: day -1 has 3 human clicks from 2 visitors (one empty hash) plus a bot; day -2 is outside
	// the requested window; another job's click must not leak in.
	exec(`INSERT INTO job_outbound_clicks (job_id, clicked_at, src, is_likely_bot, country, visitor_hash)
		SELECT j.id, (date_trunc('day', now() AT TIME ZONE 'UTC') - interval '1 day')::timestamptz + c.at, c.src, c.bot, c.country, c.hash
		FROM jobs j, (VALUES
			(interval '1 hour', 'web', false, 'DE', 'a'),
			(interval '2 hours', 'web', false, 'DE', 'a'),
			(interval '3 hours', 'mcp', false, '', 'b'),
			(interval '4 hours', 'web', true, 'US', 'c'),
			(interval '-1 day', 'web', false, 'FR', 'd')
		) AS c(at, src, bot, country, hash)
		WHERE j.public_slug = 'clicks-role'`)
	exec(`INSERT INTO job_outbound_clicks (job_id, src, country) VALUES (-1, 'web', 'DE')`)

	h := newEmployerHandlers(db.New(pool), nil)
	issuer := auth.NewIssuer("test-secret", time.Hour)
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	h.register(app.Group("/api/v1"), middleware{cookie: auth.RequireAuth(issuer, testVersions)},
		newContract(contractHandlers{employerHandlers: h}))
	get := func(path string, userID int64, want int) []byte {
		t.Helper()
		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/v1"+path, nil)
		token, err := issuer.Issue(userID, testTokenVersion)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Cookie", auth.CookieName+"="+token)
		res, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()
		body, _ := io.ReadAll(res.Body)
		if res.StatusCode != want {
			t.Fatalf("GET %s returned %d, want %d: %s", path, res.StatusCode, want, body)
		}
		return body
	}

	yesterday := time.Now().UTC().AddDate(0, 0, -1).Format(dateLayout)
	var body struct {
		Data struct {
			Days []struct {
				Day              string
				Clicks, Visitors int64
			} `json:"days"`
			Countries []struct {
				Key    string
				Clicks int64
			} `json:"countries"`
			Sources []struct {
				Key    string
				Clicks int64
			} `json:"sources"`
			Clicks   int64 `json:"clicks"`
			Visitors int64 `json:"visitors"`
		} `json:"data"`
	}
	if err := json.Unmarshal(get("/employer/jobs/clicks-role/outbound-clicks?from="+yesterday+"&to="+yesterday, owner, 200), &body); err != nil {
		t.Fatal(err)
	}
	d := body.Data
	if len(d.Days) != 1 || d.Days[0].Day != yesterday || d.Days[0].Clicks != 3 || d.Days[0].Visitors != 2 || d.Clicks != 3 || d.Visitors != 2 {
		t.Errorf("days = %+v clicks %d visitors %d, want one day with 3 clicks from 2 visitors", d.Days, d.Clicks, d.Visitors)
	}
	if len(d.Countries) != 2 || d.Countries[0].Key != "DE" || d.Countries[0].Clicks != 2 || d.Countries[1].Key != "" {
		t.Errorf("countries = %+v, want DE=2 then unknown=1, no bot country", d.Countries)
	}
	if len(d.Sources) != 2 || d.Sources[0].Key != "web" || d.Sources[0].Clicks != 2 || d.Sources[1].Key != "mcp" {
		t.Errorf("sources = %+v, want web=2 then mcp=1", d.Sources)
	}

	get("/employer/jobs/clicks-role/outbound-clicks", stranger, 404)
	get("/employer/jobs/clicks-role/outbound-clicks?from=2026-02-01&to=2026-01-01", owner, 400)
	get("/employer/jobs/clicks-role/outbound-clicks?from=2020-01-01&to=2026-01-01", owner, 400)
	get("/employer/jobs/clicks-role/outbound-clicks?from=yesterday", owner, 400)
}
