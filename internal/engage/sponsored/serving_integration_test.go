//go:build integration

package sponsored

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
	"github.com/strelov1/freehire/internal/search/search"
)

func addCampaign(t *testing.T, pool *pgxpool.Pool, jobID int64, status Status, cpc, budget, spent int64, dailyCap *int64) int64 {
	t.Helper()
	var user, id int64
	if err := pool.QueryRow(context.Background(), `SELECT id FROM users LIMIT 1`).Scan(&user); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO sponsored_campaigns (job_id, owner_user_id, status, cpc_cents, budget_cents, spent_cents, daily_cap_cents)
		 VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`, jobID, user, string(status), cpc, budget, spent, dailyCap).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestServingCampaignsEligibility(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)
	_, job := seedCampaign(t, pool, 10, 100, StatusPaused) // paused: not serving
	cap50 := int64(50)

	q := db.New(pool)
	serving := func() map[int64]db.ListServingSponsoredCampaignsRow {
		m, err := search.ServingCampaigns(ctx, q, []int64{job}, now)
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	if len(serving()) != 0 {
		t.Fatal("paused campaign serves")
	}
	addCampaign(t, pool, job, StatusActive, 20, 100, 100, nil)      // budget gone
	addCampaign(t, pool, job, StatusExhausted, 20, 100, 100, nil)   // exhausted
	addCampaign(t, pool, job, StatusPendingReview, 90, 100, 0, nil) // not approved
	if len(serving()) != 0 {
		t.Fatal("ineligible campaigns serve")
	}
	low := addCampaign(t, pool, job, StatusActive, 30, 100, 0, nil)
	high := addCampaign(t, pool, job, StatusActive, 45, 100, 99, &cap50)
	if got := serving()[job]; got.CampaignID != high || got.CpcCents != 45 {
		t.Fatalf("serving = %+v, want the highest cpc campaign %d", got, high)
	}
	// Today's cap reached: the capped campaign drops out and the next one serves ...
	if _, err := pool.Exec(ctx, `INSERT INTO sponsored_daily (campaign_id, day, spent_cents) VALUES ($1, '2026-11-24', 50)`, high); err != nil {
		t.Fatal(err)
	}
	if got := serving()[job]; got.CampaignID != low {
		t.Fatalf("serving = %+v, want %d after the cap", got, low)
	}
	// ... and the cap resets on the next UTC day.
	m, err := search.ServingCampaigns(ctx, q, []int64{job}, now.Add(24*time.Hour))
	if err != nil || m[job].CampaignID != high {
		t.Fatalf("next day serving = %+v %v, want %d", m[job], err, high)
	}
	// A closed job never serves.
	if _, err := pool.Exec(ctx, `UPDATE jobs SET closed_at = now() WHERE id = $1`, job); err != nil {
		t.Fatal(err)
	}
	if len(serving()) != 0 {
		t.Fatal("closed job serves")
	}
}

func TestEndClosedJobsAndDailyCapRequeue(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	svc := New(pool)
	_, open := seedCampaign(t, pool, 10, 100, StatusActive)
	cap50 := int64(50)
	addCampaign(t, pool, open, StatusActive, 10, 100, 0, &cap50)
	addCampaign(t, pool, open, StatusActive, 10, 100, 0, &cap50) // two capped campaigns, one job
	var closed int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO jobs (source, external_id, url, title, company, company_slug, public_slug, closed_at)
		 VALUES ('test', 'j2', 'https://example.test/j2', 'Old', 'co', 'co', 'j2', now()) RETURNING id`).Scan(&closed); err != nil {
		t.Fatal(err)
	}
	dead := addCampaign(t, pool, closed, StatusActive, 10, 100, 0, nil)

	n, err := svc.RequeueDailyCapJobs(ctx)
	if err != nil || n != 1 || !outboxHas(t, pool, open) || outboxHas(t, pool, closed) {
		t.Fatalf("requeue n=%d err=%v (want exactly the open capped job)", n, err)
	}
	if n, err := svc.EndClosedJobs(ctx); err != nil || n != 1 {
		t.Fatalf("ended %d, %v; want 1", n, err)
	}
	var status string
	_ = pool.QueryRow(ctx, `SELECT status FROM sponsored_campaigns WHERE id = $1`, dead).Scan(&status)
	if status != string(StatusEnded) {
		t.Errorf("status = %s", status)
	}
	if n, _ := svc.EndClosedJobs(ctx); n != 0 {
		t.Errorf("second sweep ended %d", n)
	}
}
