//go:build integration

// auto-apply's daily allowance, against a real Postgres. What is asserted here is not the
// arithmetic — decide's own tests cover that — but the two things only the route can get
// wrong: WHEN a unit is spent, and what a refusal says.
//
// Run with: go test -tags=integration ./internal/api/handler/
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/identity/auth"

	"github.com/strelov1/freehire/internal/platform/db"
)

// makeUltra puts an account on the ultra plan, through the granted source for the reason
// makePro uses it: this is about what a plan ALLOWS, and no payment provider is involved.
func makeUltra(t *testing.T, pool *pgxpool.Pool, userID int64) {
	t.Helper()
	q := db.New(pool)
	if err := q.SetUltraUntilGranted(context.Background(), db.SetUltraUntilGrantedParams{
		ID: userID, Until: pgtype.Timestamptz{Time: time.Now().Add(24 * time.Hour), Valid: true},
	}); err != nil {
		t.Fatalf("make ultra: %v", err)
	}
}

// autoApplyUsedToday is how many units the ledger says this account has spent.
func autoApplyUsedToday(t *testing.T, pool *pgxpool.Pool, userID int64) int {
	t.Helper()
	var used int
	err := pool.QueryRow(context.Background(),
		`SELECT coalesce(sum(used), 0) FROM usage_daily
		  WHERE user_id = $1 AND feature = 'auto-apply' AND day = CURRENT_DATE`,
		userID).Scan(&used)
	if err != nil {
		t.Fatalf("reading usage: %v", err)
	}
	return used
}

func TestAutoApply_ProIsRefusedPastItsDailyAllowance(t *testing.T) {
	pool := startPostgres(t)
	truncateAutoApplyEnqueueTables(t, pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAutoApplyEnqueueApp(pool, iss, nil)

	user, cookie := autoApplyTailorUser(t, pool, iss, "pro-capped@example.test")
	makePro(t, pool, user)
	insertBaseCV(t, pool, user)

	// Four DIFFERENT postings, so nothing is absorbed by the per-posting idempotency.
	allowed := 0
	for i := range 4 {
		slug := "capped-" + string(rune('a'+i))
		seedEnqueueJob(t, pool, "greenhouse", slug)
		resp := enqueueRequest(t, app, slug, cookie)
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			allowed++
		}
	}
	if allowed != 3 {
		t.Fatalf("%d of four auto-applies succeeded, want 3 — pro's ceiling is what the tier "+
			"above is sold on, so a ceiling that does not refuse leaves Ultra selling nothing",
			allowed)
	}
	if used := autoApplyUsedToday(t, pool, user); used != 3 {
		t.Fatalf("spent %d, want 3 — a refused request must not be charged", used)
	}
}

func TestAutoApply_UltraIsNeverRefused(t *testing.T) {
	pool := startPostgres(t)
	truncateAutoApplyEnqueueTables(t, pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAutoApplyEnqueueApp(pool, iss, nil)

	user, cookie := autoApplyTailorUser(t, pool, iss, "ultra@example.test")
	makeUltra(t, pool, user)
	insertBaseCV(t, pool, user)

	for i := range 6 {
		slug := "unbounded-" + string(rune('a'+i))
		seedEnqueueJob(t, pool, "greenhouse", slug)
		resp := enqueueRequest(t, app, slug, cookie)
		status := resp.StatusCode
		_ = resp.Body.Close()
		if status != http.StatusOK {
			t.Fatalf("auto-apply %d of six was refused with %d — unbounded auto-apply is the "+
				"whole of what this tier is sold on", i+1, status)
		}
	}
}

func TestAutoApply_ARepeatForTheSamePostingSpendsNothing(t *testing.T) {
	pool := startPostgres(t)
	truncateAutoApplyEnqueueTables(t, pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAutoApplyEnqueueApp(pool, iss, nil)

	user, cookie := autoApplyTailorUser(t, pool, iss, "double-click@example.test")
	makePro(t, pool, user)
	insertBaseCV(t, pool, user)
	seedEnqueueJob(t, pool, "greenhouse", "same-posting")

	for range 3 {
		resp := enqueueRequest(t, app, "same-posting", cookie)
		_ = resp.Body.Close()
	}

	if used := autoApplyUsedToday(t, pool, user); used != 1 {
		t.Fatalf("spent %d for one posting, want 1 — the charge is keyed by the posting, so a "+
			"double-clicked button costs one attempt rather than three", used)
	}
}

func TestAutoApply_ARequestRefusedBeforeTheAllowanceSpendsNothing(t *testing.T) {
	pool := startPostgres(t)
	truncateAutoApplyEnqueueTables(t, pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAutoApplyEnqueueApp(pool, iss, nil)

	user, cookie := autoApplyTailorUser(t, pool, iss, "no-cv@example.test")
	makePro(t, pool, user)
	// Deliberately no base CV.
	seedEnqueueJob(t, pool, "greenhouse", "needs-a-cv")

	resp := enqueueRequest(t, app, "needs-a-cv", cookie)
	status := resp.StatusCode
	_ = resp.Body.Close()
	if status != http.StatusConflict {
		t.Fatalf("status = %d, want 409 for a missing CV", status)
	}
	if used := autoApplyUsedToday(t, pool, user); used != 0 {
		t.Fatalf("spent %d, want 0 — the allowance is the LAST gate, so everything a request "+
			"can be refused for on other grounds costs nothing", used)
	}
}

func TestAutoApply_TheRefusalNamesTheFeatureAndItsFigures(t *testing.T) {
	pool := startPostgres(t)
	truncateAutoApplyEnqueueTables(t, pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAutoApplyEnqueueApp(pool, iss, nil)

	user, cookie := autoApplyTailorUser(t, pool, iss, "capped-message@example.test")
	makePro(t, pool, user)
	insertBaseCV(t, pool, user)

	// Spend the day's allowance, then ask once more.
	var last *http.Response
	for i := range 4 {
		slug := "figures-" + string(rune('a'+i))
		seedEnqueueJob(t, pool, "greenhouse", slug)
		if last != nil {
			_ = last.Body.Close()
		}
		last = enqueueRequest(t, app, slug, cookie)
	}
	defer func() { _ = last.Body.Close() }()

	if last.StatusCode != http.StatusPaymentRequired {
		t.Fatalf("status = %d, want 402", last.StatusCode)
	}

	var body struct {
		Error     string `json:"error"`
		Allowance struct {
			Feature string `json:"feature"`
			Used    int    `json:"used"`
			Limit   int    `json:"limit"`
		} `json:"allowance"`
		UpgradeURL string `json:"upgrade_url"`
	}
	if err := json.NewDecoder(last.Body).Decode(&body); err != nil {
		t.Fatalf("decoding the refusal: %v", err)
	}
	if body.Allowance.Feature != "auto-apply" {
		t.Fatalf("refusal names %q, want auto-apply — spending another auto-apply is what "+
			"the caller has to do something about", body.Allowance.Feature)
	}
	if body.Allowance.Used != 3 || body.Allowance.Limit != 3 {
		t.Fatalf("refusal reports used=%d limit=%d, want 3 and 3 — a refusal has to carry "+
			"the same numbers a success reports", body.Allowance.Used, body.Allowance.Limit)
	}
	// A refused PRO caller now has something to buy, which was not true when this refusal
	// was written: the upgrade link was offered to the free tier alone, because there was
	// nothing above pro. Withholding it now would leave the one refusal a subscriber can
	// actually act on saying nothing about how.
	if body.UpgradeURL == "" {
		t.Fatal("a refused pro caller was offered no upgrade — Ultra exists now, and this is " +
			"the exact moment it is worth something to them")
	}
}
