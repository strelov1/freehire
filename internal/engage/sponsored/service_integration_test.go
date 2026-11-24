//go:build integration

// Run with: mise exec -- go test -tags=integration ./internal/engage/sponsored/
package sponsored

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/platform/testdb"
)

func seedCampaign(t *testing.T, pool *pgxpool.Pool, cpc, budget int, status Status) (campaignID, jobID int64) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `TRUNCATE sponsored_campaigns, search_outbox, jobs, users RESTART IDENTITY CASCADE`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	var user int64
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
		 VALUES ($1, $2, $3, $4, $5) RETURNING id`, jobID, user, string(status), cpc, budget).Scan(&campaignID); err != nil {
		t.Fatal(err)
	}
	return campaignID, jobID
}

func outboxHas(t *testing.T, pool *pgxpool.Pool, jobID int64) bool {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM search_outbox WHERE job_id = $1`, jobID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n > 0
}

// Budget for exactly one charge, many concurrent valid clicks: one wins, spend equals budget.
func TestChargeConcurrentNeverOverspends(t *testing.T) {
	pool := testdb.Pool(t)
	id, jobID := seedCampaign(t, pool, 50, 50, StatusActive)
	svc := New(pool)
	const n = 20
	now := time.Now()

	var wg sync.WaitGroup
	results := make([]Decision, n)
	errs := make([]error, n)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], errs[i] = svc.Charge(context.Background(), id, Click{Placement: "search", VisitorHash: fmt.Sprintf("v%d", i)}, now)
		}()
	}
	wg.Wait()

	valid := 0
	for i := range n {
		if errs[i] != nil {
			t.Fatalf("click %d: %v", i, errs[i])
		}
		if results[i].Valid {
			valid++
		}
	}
	if valid != 1 {
		t.Errorf("valid clicks = %d, want 1", valid)
	}
	var spent, budget int64
	var status string
	if err := pool.QueryRow(context.Background(),
		`SELECT spent_cents, budget_cents, status FROM sponsored_campaigns WHERE id = $1`, id).Scan(&spent, &budget, &status); err != nil {
		t.Fatal(err)
	}
	if spent != budget || status != string(StatusExhausted) {
		t.Errorf("spent=%d budget=%d status=%s, want spent==budget and exhausted", spent, budget, status)
	}
	var rows int
	_ = pool.QueryRow(context.Background(), `SELECT count(*) FROM sponsored_clicks WHERE campaign_id = $1`, id).Scan(&rows)
	if rows != n {
		t.Errorf("click rows = %d, want %d (every click is recorded)", rows, n)
	}
	if !outboxHas(t, pool, jobID) {
		t.Error("exhaustion did not enqueue the job for re-indexing")
	}
}

func TestChargeDuplicateVisitorWithin24h(t *testing.T) {
	pool := testdb.Pool(t)
	id, _ := seedCampaign(t, pool, 50, 1000, StatusActive)
	svc := New(pool)
	now := time.Now()
	click := Click{Placement: "search", VisitorHash: "same"}
	first, err := svc.Charge(context.Background(), id, click, now)
	if err != nil || !first.Valid {
		t.Fatalf("first: %+v %v", first, err)
	}
	second, err := svc.Charge(context.Background(), id, click, now.Add(time.Hour))
	if err != nil || second.Valid || second.Reason != ReasonDuplicate {
		t.Fatalf("second: %+v %v", second, err)
	}
}

func TestTransitionAndEndForJob(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	id, jobID := seedCampaign(t, pool, 50, 1000, StatusPendingReview)
	svc := New(pool)

	if err := svc.Transition(ctx, id, StatusPaused); err == nil {
		t.Fatal("pending_review -> paused must be refused")
	}
	if outboxHas(t, pool, jobID) {
		t.Fatal("a refused transition must not enqueue")
	}
	if err := svc.Transition(ctx, id, StatusActive); err != nil {
		t.Fatal(err)
	}
	if !outboxHas(t, pool, jobID) {
		t.Error("transition did not enqueue the job")
	}
	n, err := svc.EndForJob(ctx, jobID)
	if err != nil || n != 1 {
		t.Fatalf("EndForJob = %d, %v", n, err)
	}
	var status string
	_ = pool.QueryRow(ctx, `SELECT status FROM sponsored_campaigns WHERE id = $1`, id).Scan(&status)
	if status != string(StatusEnded) {
		t.Errorf("status = %s, want ended", status)
	}
	if n, _ := svc.EndForJob(ctx, jobID); n != 0 {
		t.Errorf("second EndForJob ended %d, want 0", n)
	}
}
