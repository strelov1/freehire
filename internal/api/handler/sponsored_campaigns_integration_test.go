//go:build integration

// Run with: mise exec -- go test -tags=integration ./internal/api/handler/ -run SponsoredCampaigns
package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/engage/sponsored"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/identity/billing"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

type fakeTopup struct{}

func (fakeTopup) SponsoredTopupURL(_ context.Context, _, _, cents int64) (string, error) {
	if cents < 2900 {
		return "", &billing.BelowMinimumError{MinCents: 2900, GotCents: cents}
	}
	return "https://pay.example.test/c", nil
}

func TestSponsoredCampaignsAuthorizationAndFlow(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `TRUNCATE sponsored_campaigns, search_outbox, jobs, users RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	var owner, other, mod int64
	for _, r := range []struct {
		email, role string
		id          *int64
	}{{"owner@example.test", "user", &owner}, {"other@example.test", "user", &other}, {"mod@example.test", "moderator", &mod}} {
		if err := pool.QueryRow(ctx, `INSERT INTO users (email, role) VALUES ($1, $2) RETURNING id`, r.email, r.role).Scan(r.id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO jobs (source, external_id, url, title, company, company_slug, public_slug)
		 VALUES ('test', 'j1', 'https://example.test/j1', 'Engineer', 'co', 'co', 'j1')`); err != nil {
		t.Fatal(err)
	}

	queries := db.New(pool)
	h := newSponsoredHandlers(sponsored.New(pool).WithMinCPC(10), queries, nil)
	h.billing = fakeTopup{}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	signedIn := func(c *fiber.Ctx) error {
		id, err := strconv.ParseInt(c.Get("X-User"), 10, 64)
		if err != nil {
			return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
		}
		c.Locals(auth.LocalsUserID, id)
		return c.Next()
	}
	h.register(app.Group("/api/v1"), middleware{cookie: signedIn, moderator: auth.RequireRole(queries, "moderator")},
		newContract(contractHandlers{sponsoredHandlers: h}))

	do := func(user int64, method, path, body string) (int, string) {
		req := httptest.NewRequestWithContext(ctx, method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if user != 0 {
			req.Header.Set("X-User", strconv.FormatInt(user, 10))
		}
		resp, err := app.Test(req, -1)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	want := func(got int, body string, status int) {
		t.Helper()
		if got != status {
			t.Fatalf("status %d, want %d (%s)", got, status, body)
		}
	}

	st, body := do(0, "GET", "/api/v1/sponsored/campaigns", "")
	want(st, body, 401)
	st, body = do(owner, "POST", "/api/v1/sponsored/campaigns", `{"job_slug":"j1","cpc_cents":5}`)
	want(st, body, 422)
	st, body = do(owner, "POST", "/api/v1/sponsored/campaigns", `{"job_slug":"j1","cpc_cents":50,"daily_cap_cents":500}`)
	want(st, body, 201)
	var created struct{ Data struct{ ID int64 } }
	if err := json.Unmarshal([]byte(body), &created); err != nil || created.Data.ID == 0 {
		t.Fatalf("create body %s: %v", body, err)
	}
	path := "/api/v1/sponsored/campaigns/" + strconv.FormatInt(created.Data.ID, 10)

	st, body = do(owner, "GET", path, "")
	want(st, body, 200)
	for _, p := range []struct{ m, suffix, b string }{{"GET", "", ""}, {"POST", "/pause", ""}, {"POST", "/end", ""}, {"POST", "/topup", `{"amount_cents":5000}`}} {
		st, body = do(other, p.m, path+p.suffix, p.b)
		want(st, body, 404)
	}
	st, body = do(owner, "POST", path+"/pause", "")
	want(st, body, 409) // a draft cannot pause
	st, body = do(owner, "POST", path+"/topup", `{"amount_cents":100}`)
	want(st, body, 422)
	st, body = do(owner, "POST", path+"/topup", `{"amount_cents":5000}`)
	want(st, body, 200)

	// Review endpoints: 401 anonymous, 403 for a plain user, even for the campaign's owner.
	for _, u := range []int64{0, owner} {
		st, body = do(u, "GET", "/api/v1/sponsored/review", "")
		want(st, body, map[bool]int{true: 401, false: 403}[u == 0])
		st, body = do(u, "POST", "/api/v1/sponsored/review/1/approve", "")
		want(st, body, map[bool]int{true: 401, false: 403}[u == 0])
	}

	// Moderator: approve needs pending_review, reject needs a reason.
	id := strconv.FormatInt(created.Data.ID, 10)
	st, body = do(mod, "POST", "/api/v1/sponsored/review/"+id+"/approve", "")
	want(st, body, 409)
	if _, err := pool.Exec(ctx, `UPDATE sponsored_campaigns SET status = 'pending_review', budget_cents = 5000`); err != nil {
		t.Fatal(err)
	}
	st, body = do(mod, "GET", "/api/v1/sponsored/review", "")
	want(st, body, 200)
	if !strings.Contains(body, `"owner_user_id"`) || strings.Contains(body, "owner@example.test") {
		t.Fatalf("queue body %s", body)
	}
	st, body = do(mod, "POST", "/api/v1/sponsored/review/"+id+"/reject", `{"reason":"  "}`)
	want(st, body, 422)
	st, body = do(mod, "POST", "/api/v1/sponsored/review/"+id+"/reject", `{"reason":"spam"}`)
	want(st, body, 200) // no refunder wired: the rejection stands, the refund is pending
	if !strings.Contains(body, `"refund_pending":true`) {
		t.Fatalf("reject body %s", body)
	}
	st, body = do(owner, "POST", path+"/topup", `{"amount_cents":5000}`)
	want(st, body, 409)
	st, body = do(owner, "GET", path, "")
	want(st, body, 200)
	if !strings.Contains(body, `"reject_reason":"spam"`) {
		t.Fatalf("detail body %s", body)
	}
}
