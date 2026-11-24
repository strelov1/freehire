//go:build integration

// Run with: mise exec -- go test -tags=integration ./internal/api/handler/ -run SponsoredFlowEndToEnd
// Needs Docker (a throwaway Meilisearch) and the local test Postgres.
package handler

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/strelov1/freehire/internal/engage/sponsored"
	"github.com/strelov1/freehire/internal/identity/billing"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
	"github.com/strelov1/freehire/internal/search/search"
)

func startFlowMeili(t *testing.T) *search.Client {
	t.Helper()
	ctx := context.Background()
	const key = "test-master-key"
	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "getmeili/meilisearch:v1.49.0", // the version prod runs
			ExposedPorts: []string{"7700/tcp"},
			Env:          map[string]string{"MEILI_MASTER_KEY": key, "MEILI_ENV": "development"},
			WaitingFor:   wait.ForHTTP("/health").WithPort("7700/tcp").WithStartupTimeout(60 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("start meilisearch: %v", err)
	}
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(c) })
	host, err := c.Host(ctx)
	if err != nil {
		t.Fatal(err)
	}
	port, err := c.MappedPort(ctx, "7700")
	if err != nil {
		t.Fatal(err)
	}
	return search.NewClient("http://"+host+":"+port.Port(), key)
}

// reindexFlowJobs builds documents the way cmd/search-drain and cmd/reindex do (FromJob, then
// ServingCampaigns + AttachSponsored from the database) and pushes them to Meilisearch.
func reindexFlowJobs(t *testing.T, pool *pgxpool.Pool, mc *search.Client, ids []int64) {
	t.Helper()
	ctx := context.Background()
	q := db.New(pool)
	serving, err := search.ServingCampaigns(ctx, q, ids, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	docs := make([]search.JobDocument, 0, len(ids))
	for _, id := range ids {
		job, err := q.GetJob(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		doc, err := search.FromJob(job, "ats")
		if err != nil {
			t.Fatal(err)
		}
		doc.AttachSponsored(serving)
		docs = append(docs, doc)
	}
	if err := mc.IndexJobs(ctx, docs); err != nil {
		t.Fatalf("IndexJobs: %v", err)
	}
}

func TestSponsoredFlowEndToEnd(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	mc := startFlowMeili(t)
	if err := mc.EnsureIndex(ctx); err != nil { // the real filterable/sortable settings
		t.Fatalf("EnsureIndex: %v", err)
	}

	var buyer, moderator int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('buyer@example.test') RETURNING id`).Scan(&buyer); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email, role) VALUES ('mod@example.test', 'moderator') RETURNING id`).Scan(&moderator); err != nil {
		t.Fatal(err)
	}
	// Three jobs match "go engineer" in the title; the promoted one only mentions Go in the body, so it ranks last.
	var ids []int64
	for i, title := range []string{"Go engineer", "Senior Go engineer", "Staff Go engineer", "Backend developer"} {
		var id int64
		if err := pool.QueryRow(ctx,
			`INSERT INTO jobs (source, external_id, url, title, company, company_slug, public_slug, description)
			 VALUES ('test', $1, $2, $3, 'co', 'co', $4, 'We build services in Go.') RETURNING id`,
			fmt.Sprint("j", i), fmt.Sprintf("https://example.test/j%d", i), title, fmt.Sprint("job-", i)).Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	promoted := ids[3]

	svc := sponsored.New(pool)
	camp, err := svc.Create(ctx, buyer, "job-3", 50, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := svc.CreditTopup(ctx, billing.Topup{SessionID: "cs_flow", PaymentIntentID: "pi_flow", CampaignID: camp.ID, UserID: buyer, AmountCents: 100}); err != nil {
		t.Fatalf("CreditTopup: %v", err)
	}
	campaignState := func() (status string, spent, budget int64) {
		t.Helper()
		if err := pool.QueryRow(ctx, `SELECT status, spent_cents, budget_cents FROM sponsored_campaigns WHERE id = $1`, camp.ID).Scan(&status, &spent, &budget); err != nil {
			t.Fatal(err)
		}
		return
	}
	if s, _, b := campaignState(); s != "pending_review" || b != 100 {
		t.Fatalf("after top-up: %s budget %d", s, b)
	}

	// Pending review: indexed, but not promoted.
	reindexFlowJobs(t, pool, mc, ids)
	app := adApp(mc, db.New(pool), testClickKey, nil)
	_, body := doGet(t, app, "/api/v1/jobs/search?q=go+engineer")
	if got := adSlugs(body); len(got) != 4 || got[0] == "job-3" || body["data"].([]any)[0].(map[string]any)["sponsored"] != nil {
		t.Fatalf("before approval: %v", got)
	}

	if err := svc.Approve(ctx, camp.ID, moderator); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if s, _, _ := campaignState(); s != "active" {
		t.Fatalf("after approval: %s", s)
	}
	reindexFlowJobs(t, pool, mc, ids)

	_, body = doGet(t, app, "/api/v1/jobs/search?q=go+engineer")
	got := adSlugs(body)
	if len(got) != 4 || got[0] != "job-3" {
		t.Fatalf("sponsored job not first, once: %v", got)
	}
	for _, s := range got[1:] {
		if s == "job-3" {
			t.Fatalf("promoted job repeated in the organic part: %v", got)
		}
	}
	first := body["data"].([]any)[0].(map[string]any)
	url, _ := first["sponsored_url"].(string)
	if first["sponsored"] != true || !strings.HasPrefix(url, sponsored.ClickPath) {
		t.Fatalf("first hit = %v", first)
	}
	for _, u := range []string{"/api/v1/jobs/search?q=go+engineer&offset=1", "/api/v1/jobs/search"} {
		_, b := doGet(t, app, u)
		for _, d := range b["data"].([]any) {
			if d.(map[string]any)["sponsored"] != nil {
				t.Fatalf("%s carries a sponsored entry: %v", u, d)
			}
		}
	}

	// Two different visitors click through the real redirect route.
	clicks := clickApp(svc, db.New(pool))
	token := strings.TrimPrefix(url, sponsored.ClickPath)
	for _, ua := range []string{clickUA, clickUA + " Second"} {
		if status, loc := getClick(t, clicks, token, ua); status != 302 || loc != "https://example.test/j3?utm_source=freehire.me" {
			t.Fatalf("click: %d %q", status, loc)
		}
	}
	if s, spent, b := campaignState(); s != "exhausted" || spent != 100 || spent != b {
		t.Fatalf("after clicks: %s spent %d budget %d", s, spent, b)
	}
	var queued int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM search_outbox WHERE job_id = $1`, promoted).Scan(&queued); err != nil || queued != 1 {
		t.Fatalf("outbox rows for the job = %d, %v", queued, err)
	}

	// The drain re-indexes it: no longer sponsored.
	reindexFlowJobs(t, pool, mc, []int64{promoted})
	_, body = doGet(t, app, "/api/v1/jobs/search?q=go+engineer")
	for _, d := range body["data"].([]any) {
		if d.(map[string]any)["sponsored"] != nil {
			t.Fatalf("exhausted campaign still promoted: %v", d)
		}
	}
	if got := adSlugs(body); len(got) != 4 {
		t.Fatalf("after exhaustion: %v", got)
	}
}
