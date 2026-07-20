//go:build integration

// Integration tests for the employee-referral HTTP contract: the seeker request flow
// (eligibility-gated, 201), the referrer inbox + resolve, and the moderator-gated offer
// queue/decision. A real Postgres exercises the DB-backed paths; the ping is a no-op
// (nil channels). Run with: go test -tags=integration ./internal/handler/
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/referral"
)

func TestReferralEndpoints(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	queries := db.New(pool)

	seedUser := func(email, role string) int64 {
		var id int64
		if err := pool.QueryRow(ctx,
			`INSERT INTO users (email, role) VALUES ($1, $2) RETURNING id`, email, role).Scan(&id); err != nil {
			t.Fatalf("seed user %s: %v", email, err)
		}
		return id
	}
	seeker := seedUser("seeker@example.test", "user")
	refUser := seedUser("ref@example.test", "user")
	mod := seedUser("mod@example.test", "moderator")
	// The seeker has a stored résumé so an 'original' request is attachable.
	if _, err := pool.Exec(ctx, `UPDATE users SET resume_object_key = 'resume/x.pdf' WHERE id = $1`, seeker); err != nil {
		t.Fatalf("seed resume: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO companies (slug, name, job_count) VALUES ('acme','Acme',1)`); err != nil {
		t.Fatalf("seed company: %v", err)
	}

	iss := auth.NewIssuer("test-secret", time.Hour)
	token := func(uid int64) string {
		tok, err := iss.Issue(uid)
		if err != nil {
			t.Fatalf("issue: %v", err)
		}
		return tok
	}

	h := &API{
		pool: pool, queries: queries, issuer: iss,
		referral: referral.New(referral.NewQueriesRepository(queries),
			referral.NewChannelPinger(nil, "", nil), referral.Config{}),
	}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	requireMod := auth.RequireRole(queries, "moderator")
	app.Post("/api/v1/me/referrals/requests", auth.RequireAuth(iss), h.CreateReferralRequest)
	app.Get("/api/v1/me/referrals/requests", auth.RequireAuth(iss), h.ListMyReferralRequests)
	app.Get("/api/v1/me/referrals/incoming", auth.RequireAuth(iss), h.ListIncomingReferralRequests)
	app.Get("/api/v1/me/referrals/incoming/:id/cv", auth.RequireAuth(iss), h.ViewReferralRequestCV)
	app.Post("/api/v1/me/referrals/incoming/:id/resolve", auth.RequireAuth(iss), h.ResolveReferralRequest)
	app.Get("/api/v1/referrals/offers", auth.RequireAuth(iss), requireMod, h.ListPendingReferralOffers)
	app.Get("/api/v1/referrals/offers/:id/proof", auth.RequireAuth(iss), requireMod, h.ViewReferralOfferProof)
	app.Post("/api/v1/referrals/offers/:id/decide", auth.RequireAuth(iss), requireMod, h.DecideReferralOffer)

	do := func(method, path, tok string, body any) (int, map[string]any) {
		var buf io.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			buf = bytes.NewReader(b)
		}
		req, _ := http.NewRequest(method, path, buf)
		req.Host = "localhost"
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if tok != "" {
			req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: tok})
		}
		resp, err := app.Test(req, -1)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		defer resp.Body.Close()
		var out map[string]any
		raw, _ := io.ReadAll(resp.Body)
		_ = json.Unmarshal(raw, &out)
		return resp.StatusCode, out
	}

	// A request into a company with no approved referrer is a 409.
	reqBody := map[string]any{"company_slug": "acme", "cv_kind": "original", "contact_email": "seeker@example.test", "linkedin_url": "https://www.linkedin.com/in/seeker", "note": "hi"}
	if code, _ := do(http.MethodPost, "/api/v1/me/referrals/requests", token(seeker), reqBody); code != http.StatusConflict {
		t.Fatalf("request without referrer: status %d, want 409", code)
	}

	// Approve an offer directly so the company becomes referral-eligible.
	if _, err := pool.Exec(ctx,
		`INSERT INTO referral_offers (user_id, company_slug, proof_object_key, status) VALUES ($1,'acme','k','approved')`,
		refUser); err != nil {
		t.Fatalf("seed approved offer: %v", err)
	}

	t.Run("seeker creates a request, sees it, referrer resolves it", func(t *testing.T) {
		code, body := do(http.MethodPost, "/api/v1/me/referrals/requests", token(seeker), reqBody)
		if code != http.StatusCreated {
			t.Fatalf("create request: status %d body %v, want 201", code, body)
		}
		reqID := int64(body["data"].(map[string]any)["id"].(float64))

		// Duplicate active request → 409.
		if code, _ := do(http.MethodPost, "/api/v1/me/referrals/requests", token(seeker), reqBody); code != http.StatusConflict {
			t.Errorf("duplicate request: status %d, want 409", code)
		}

		// Seeker sees exactly one request.
		_, mine := do(http.MethodGet, "/api/v1/me/referrals/requests", token(seeker), nil)
		if got := len(mine["data"].([]any)); got != 1 {
			t.Errorf("my requests = %d, want 1", got)
		}

		// Referrer sees it in the inbox with the seeker's contact.
		_, inbox := do(http.MethodGet, "/api/v1/me/referrals/incoming", token(refUser), nil)
		rows := inbox["data"].([]any)
		if len(rows) != 1 {
			t.Fatalf("referrer inbox = %d, want 1", len(rows))
		}
		if rows[0].(map[string]any)["contact_email"] != "seeker@example.test" {
			t.Errorf("inbox row = %v, want seeker contact", rows[0])
		}
		if rows[0].(map[string]any)["linkedin_url"] != "https://www.linkedin.com/in/seeker" {
			t.Errorf("inbox row = %v, want seeker LinkedIn", rows[0])
		}

		// CV access is cabinet-only: the seeker (not an approved referrer) is refused.
		if code, _ := do(http.MethodGet, "/api/v1/me/referrals/incoming/"+itoa(reqID)+"/cv", token(seeker), nil); code != http.StatusForbidden {
			t.Errorf("seeker viewing CV: status %d, want 403", code)
		}
		// The referrer is authorized; with no blob store wired the stream reports 503,
		// which proves the request reached the storage path past the gate.
		if code, _ := do(http.MethodGet, "/api/v1/me/referrals/incoming/"+itoa(reqID)+"/cv", token(refUser), nil); code != http.StatusServiceUnavailable {
			t.Errorf("referrer viewing original CV (no blob): status %d, want 503", code)
		}

		// Referrer marks it contacted.
		code, marked := do(http.MethodPost, "/api/v1/me/referrals/incoming/"+itoa(reqID)+"/resolve",
			token(refUser), map[string]any{"status": "contacted"})
		if code != http.StatusOK {
			t.Fatalf("resolve: status %d body %v, want 200", code, marked)
		}
		if marked["data"].(map[string]any)["status"] != "contacted" {
			t.Errorf("resolved status = %v, want contacted", marked["data"])
		}
	})

	t.Run("offer queue is moderator-gated", func(t *testing.T) {
		if code, _ := do(http.MethodGet, "/api/v1/referrals/offers", token(seeker), nil); code != http.StatusForbidden {
			t.Errorf("non-moderator offer queue: status %d, want 403", code)
		}
		if code, _ := do(http.MethodGet, "/api/v1/referrals/offers", token(mod), nil); code != http.StatusOK {
			t.Errorf("moderator offer queue: status %d, want 200", code)
		}
	})

	t.Run("company read exposes referral availability", func(t *testing.T) {
		pub := fiber.New(fiber.Config{ErrorHandler: RenderError})
		pub.Get("/api/v1/companies/:slug", h.GetCompany)
		if _, err := pool.Exec(ctx, `INSERT INTO companies (slug, name, job_count) VALUES ('globex','Globex',0)`); err != nil {
			t.Fatalf("seed company: %v", err)
		}
		avail := func() bool {
			req, _ := http.NewRequest(http.MethodGet, "/api/v1/companies/globex", nil)
			req.Host = "localhost"
			resp, err := pub.Test(req, -1)
			if err != nil {
				t.Fatalf("get company: %v", err)
			}
			defer resp.Body.Close()
			var out map[string]any
			raw, _ := io.ReadAll(resp.Body)
			_ = json.Unmarshal(raw, &out)
			return out["data"].(map[string]any)["referral_available"].(bool)
		}
		if avail() {
			t.Error("referral_available = true with no approved offer, want false")
		}
		if _, err := pool.Exec(ctx,
			`INSERT INTO referral_offers (user_id, company_slug, proof_object_key, status) VALUES ($1,'globex','k','approved')`,
			refUser); err != nil {
			t.Fatalf("seed approved offer: %v", err)
		}
		if !avail() {
			t.Error("referral_available = false after an approved offer, want true")
		}
	})

	t.Run("moderator decides a pending offer", func(t *testing.T) {
		var offerID int64
		if err := pool.QueryRow(ctx,
			`INSERT INTO referral_offers (user_id, company_slug, proof_object_key) VALUES ($1,'acme','k2') RETURNING id`,
			seeker).Scan(&offerID); err != nil {
			t.Fatalf("seed pending offer: %v", err)
		}
		code, body := do(http.MethodPost, "/api/v1/referrals/offers/"+itoa(offerID)+"/decide",
			token(mod), map[string]any{"approve": true})
		if code != http.StatusOK {
			t.Fatalf("decide: status %d body %v, want 200", code, body)
		}
		if body["data"].(map[string]any)["status"] != "approved" {
			t.Errorf("decided status = %v, want approved", body["data"])
		}
	})
}
