//go:build integration

package main

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

type companyBoardFixture struct{}

func (companyBoardFixture) Resolve(context.Context, string) (string, string, string, bool) {
	return "", "", "", false
}
func (companyBoardFixture) ResolveCompany(context.Context, string) (string, string, string, bool) {
	return "hrsmart", "gistworld.mua.hrdepartment.com", "https://gistworld.mua.hrdepartment.com/hrsmart/ats/JobSearch/viewAll", true
}

func TestCompanyDiscoveryQueuesTheNativeBoardForOrdinaryIngestion(t *testing.T) {
	pool := testdb.Pool(t)
	ctx, q := context.Background(), db.New(pool)
	registry := sources.Taxonomy()
	registry["hrsmart"] = nativeDiscoveryFixture{}
	store := newDBStore(pool, 1, nil, newWriteTally(), defaultSeenPolicy())
	resolver := newNativeDiscovery(pool, registry, store)
	resolver.boards = companyBoardFixture{}
	err := resolver.Resolve(ctx, sources.CompanyEntry{Provider: "linkedin", Board: "software engineer"}, sources.Job{
		ExternalID: "4472812273", Title: "Senior Java Software Engineer", Company: "Gist Limited", URL: "https://www.linkedin.com/jobs/view/4472812273/",
		ApplyMethod: sources.ApplyExternal, CompanyWebsite: "https://www.gistworld.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	var pending bool
	if err := pool.QueryRow(ctx, "SELECT status='pending' FROM boards WHERE provider='hrsmart' AND board='gistworld.mua.hrdepartment.com'").Scan(&pending); err != nil || !pending {
		t.Fatalf("native board was not queued pending a real crawl: %v, %v", pending, err)
	}
	if cfg, err := q.GetIngestSchedule(ctx, "hrsmart"); err != nil || !cfg.Managed || !cfg.Enabled {
		t.Fatalf("native board cannot be polled by the ordinary scheduler: %+v, %v", cfg, err)
	}
	var copies int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM jobs").Scan(&copies); err != nil || copies != 0 {
		t.Fatalf("discovery invented a posting instead of scheduling the native crawl: %d, %v", copies, err)
	}
}

type nativeDiscoveryFixture struct{}

func (nativeDiscoveryFixture) Provider() string { return "hrsmart" }
func (nativeDiscoveryFixture) Fetch(_ context.Context, e sources.CompanyEntry) ([]sources.Job, error) {
	return []sources.Job{{ExternalID: "3735", URL: "https://gistworld.mua.hrdepartment.com/hrsmart/ats/Posting/view/3735", Title: "Senior Java Software Engineer", Company: e.Company, Description: "Build Java services from the employer's native ATS."}}, nil
}

func TestExternalLinkedInDiscoveryWritesOnlyTheNativeATSPosting(t *testing.T) {
	pool := testdb.Pool(t)
	ctx, q := context.Background(), db.New(pool)
	registry := sources.Taxonomy()
	registry["hrsmart"] = nativeDiscoveryFixture{}
	store := newDBStore(pool, 1, nil, newWriteTally(), defaultSeenPolicy())
	resolver := newNativeDiscovery(pool, registry, store)
	err := resolver.Resolve(ctx, sources.CompanyEntry{Provider: "linkedin", Board: "software engineer"}, sources.Job{
		ExternalID: "4472812273", Title: "Senior Java Software Engineer", Company: "Gist Limited", URL: "https://www.linkedin.com/jobs/view/4472812273/",
		ApplyMethod: sources.ApplyExternal, ExternalApplyURL: "https://gistworld.mua.hrdepartment.com/hrsmart/ats/Posting/view/3735?src=LinkedIn",
	})
	if err != nil {
		t.Fatal(err)
	}
	row, err := q.GetJobBySourceExternalID(ctx, db.GetJobBySourceExternalIDParams{Source: "hrsmart", ExternalID: "gistworld.mua.hrdepartment.com:3735"})
	if err != nil || row.URL != "https://gistworld.mua.hrdepartment.com/hrsmart/ats/Posting/view/3735" {
		t.Fatalf("native posting = %+v, %v", row, err)
	}
	var copies int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM jobs WHERE source='linkedin'").Scan(&copies); err != nil || copies != 0 {
		t.Fatalf("external discovery leaked into LinkedIn source: %d, %v", copies, err)
	}
}
