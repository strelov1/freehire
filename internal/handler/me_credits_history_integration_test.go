//go:build integration

// Integration test for GET /api/v1/me/credits/history: lists the caller's credit-ledger
// entries newest first, each labelled for display — grants, contribution rewards, and metered
// debits resolved to the job they named — scoped to the caller. Deleted subjects fall back to
// a generic label. Run with: go test -tags=integration ./internal/handler/
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/credits"
	"github.com/strelov1/freehire/internal/db"
)

func TestGetMyCreditsHistoryEndpoint(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	queries := db.New(pool)

	seedUser := func(email string) int64 {
		var id int64
		if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ($1) RETURNING id`, email).Scan(&id); err != nil {
			t.Fatalf("seed user %s: %v", email, err)
		}
		return id
	}
	userID := seedUser("credits-history@example.test")
	otherID := seedUser("other@example.test")

	var jobID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO jobs (source, external_id, url, title, company, public_slug, content_hash)
		 VALUES ('test','job:1','http://e.test','Senior Go Engineer','Acme','senior-go-engineer','h') RETURNING id`).Scan(&jobID); err != nil {
		t.Fatalf("seed job: %v", err)
	}
	var cvID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO cvs (user_id, title, template_id, data, job_id)
		 VALUES ($1, 'Tailored', 'default', '{}'::jsonb, $2) RETURNING id`, userID, jobID).Scan(&cvID); err != nil {
		t.Fatalf("seed tailored cv: %v", err)
	}

	// Seed the ledger oldest → newest so the endpoint's newest-first order is observable.
	// (period is cosmetic here; the history query does not filter by it.)
	seed := func(uid int64, kind, feature, ref string, delta int, ageMinutes int) {
		var f, r any
		if feature != "" {
			f = feature
		}
		if ref != "" {
			r = ref
		}
		if _, err := pool.Exec(ctx,
			`INSERT INTO credit_ledger (user_id, period, kind, feature, delta, ref, created_at)
			 VALUES ($1, '2026-07', $2, $3, $4, $5, now() - make_interval(mins => $6))`,
			uid, kind, f, delta, r, ageMinutes); err != nil {
			t.Fatalf("seed ledger (%s): %v", kind, err)
		}
	}
	seed(userID, "grant", "", "", 20, 50)                                // oldest
	seed(userID, "reward", "", "1", 5, 40)                               // contribution reward
	seed(userID, "debit", "match", strconv.FormatInt(jobID, 10), -1, 30) // match → job title
	seed(userID, "debit", "tailor", strconv.FormatInt(cvID, 10), -3, 20) // tailor → job title
	seed(userID, "debit", "match", "9999999", -1, 10)                    // deleted job → fallback (newest)
	seed(otherID, "grant", "", "", 20, 5)                                // another user's row — must not leak

	iss := auth.NewIssuer("test-secret", time.Hour)
	token, _ := iss.Issue(userID)

	h := &API{
		pool:    pool,
		queries: queries,
		issuer:  iss,
		credits: credits.NewStore(queries, pool, credits.Config{MonthlyGrant: 20, CostMatch: 1, CostTailor: 3, ContributionReward: 5}),
	}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/me/credits/history", auth.RequireAuthOrKey(iss, queries), h.GetMyCreditsHistory)

	req := httptest.NewRequest(fiber.MethodGet, "/api/v1/me/credits/history", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body struct {
		Data []struct {
			Kind     string `json:"kind"`
			Delta    int    `json:"delta"`
			Label    string `json:"label"`
			Subtitle string `json:"subtitle"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Only the caller's five rows, newest first.
	if len(body.Data) != 5 {
		t.Fatalf("entries = %d, want 5 (caller-scoped)", len(body.Data))
	}
	want := []struct {
		kind, label, subtitle string
		delta                 int
	}{
		{"debit", "Match analysis", "", -1},                   // deleted job → no subtitle
		{"debit", "CV tailoring", "Senior Go Engineer", -3},   // tailor → job title
		{"debit", "Match analysis", "Senior Go Engineer", -1}, // match → job title
		{"reward", "Board contribution", "", 5},               // contribution reward
		{"grant", "Monthly grant", "", 20},                    // oldest
	}
	for i, w := range want {
		got := body.Data[i]
		if got.Kind != w.kind || got.Label != w.label || got.Subtitle != w.subtitle || got.Delta != w.delta {
			t.Errorf("entry[%d] = %+v, want kind=%s label=%q subtitle=%q delta=%d", i, got, w.kind, w.label, w.subtitle, w.delta)
		}
	}
}
