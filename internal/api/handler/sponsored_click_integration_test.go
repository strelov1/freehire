//go:build integration

// Run with: mise exec -- go test -tags=integration ./internal/api/handler/ -run SponsoredClickEndToEnd
package handler

import (
	"context"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/engage/sponsored"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

func TestSponsoredClickEndToEnd(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	var user, jobID, campaign int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('buyer@example.test') RETURNING id`).Scan(&user); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO jobs (source, external_id, url, title, company, company_slug, public_slug)
		 VALUES ('test', 'j1', 'https://example.test/j1', 'Engineer', 'co', 'co', 'j1') RETURNING id`).Scan(&jobID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO sponsored_campaigns (job_id, owner_user_id, status, cpc_cents, budget_cents)
		 VALUES ($1, $2, 'active', 50, 500) RETURNING id`, jobID, user).Scan(&campaign); err != nil {
		t.Fatal(err)
	}

	app := clickApp(sponsored.New(pool), db.New(pool))
	token := sponsored.SignToken(testClickKey, sponsored.Claims{CampaignID: campaign, JobID: jobID, Placement: sponsored.PlacementSearch}, time.Now())
	want := "https://example.test/j1?utm_source=freehire.me"
	for range 2 {
		if status, loc := getClick(t, app, token, clickUA); status != 302 || loc != want {
			t.Fatalf("got %d %q", status, loc)
		}
	}

	rows, err := pool.Query(ctx, `SELECT valid, reason, cost_cents FROM sponsored_clicks WHERE campaign_id = $1 ORDER BY id`, campaign)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	type click struct {
		valid  bool
		reason string
		cost   int64
	}
	var got []click
	for rows.Next() {
		var c click
		if err := rows.Scan(&c.valid, &c.reason, &c.cost); err != nil {
			t.Fatal(err)
		}
		got = append(got, c)
	}
	if len(got) != 2 || got[0] != (click{true, "", 50}) || got[1] != (click{false, "duplicate", 0}) {
		t.Errorf("clicks = %+v", got)
	}
}
