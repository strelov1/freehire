//go:build integration

package main

import (
	"bytes"
	"context"
	"log"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/linkimport"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

type unrecognizedLinkedInBoard struct{}

func (unrecognizedLinkedInBoard) Resolve(context.Context, string) (string, string, string, bool) {
	return "", "", "", false
}

func (unrecognizedLinkedInBoard) ResolveCompany(context.Context, string) (string, string, string, bool) {
	return "", "", "", false
}

func (unrecognizedLinkedInBoard) ResolveCompanyWithCareerHosts(context.Context, string) (string, string, string, []string, bool) {
	return "", "", "", []string{"apply.vendor.example"}, false
}

func TestLinkedInUnresolvedATSLogsObservedHost(t *testing.T) {
	pool := testdb.Pool(t)
	resolver := newNativeDiscovery(pool, sources.Taxonomy(), nil)
	resolver.importer = linkimport.New(pool, resolver.q, nil, nil, resolver.registry, unrecognizedLinkedInBoard{})
	var logs bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(previous) })

	entry := sources.CompanyEntry{Provider: "linkedin", Board: "engineer"}
	for _, tc := range []struct {
		id, company, apply, website, want string
		wantError                         bool
	}{
		{"100001", "Direct", "https://apply.vendor.example/opening?token=private-fixture-value", "", `reason=apply_destination_unresolved apply_host="apply.vendor.example"`, true},
		{"100002", "Careers", "", "https://employer.example/careers?token=private-fixture-value", `reason=career_site_unresolved employer_host="employer.example" career_hosts="apply.vendor.example"`, false},
		{"100003", "Missing", "", "", `reason=no_employer_website`, false},
	} {
		t.Run(tc.company, func(t *testing.T) {
			logs.Reset()
			err := resolver.Resolve(context.Background(), entry, sources.Job{
				ExternalID: tc.id, URL: sources.LinkedInPostingURL(tc.id), Company: tc.company,
				ApplyMethod: sources.ApplyExternal, ExternalApplyURL: tc.apply, CompanyWebsite: tc.website,
			})
			if (err != nil) != tc.wantError {
				t.Fatalf("Resolve error = %v, want error %t", err, tc.wantError)
			}
			got := logs.String()
			if !strings.Contains(got, `linkedin_ats_unresolved posting_id="`+tc.id+`"`) ||
				!strings.Contains(got, tc.want) || strings.Contains(got, "private-fixture-value") {
				t.Fatalf("unexpected ATS log: %q", got)
			}
		})
	}
}

type companyBoardFixture struct{}

func (companyBoardFixture) Resolve(context.Context, string) (string, string, string, bool) {
	return "", "", "", false
}
func (companyBoardFixture) ResolveCompany(context.Context, string) (string, string, string, bool) {
	return "hrsmart", "gistworld.mua.hrdepartment.com", "https://gistworld.mua.hrdepartment.com/hrsmart/ats/JobSearch/viewAll", true
}

func withEmployerProof(d *nativeDiscovery) *nativeDiscovery {
	d.boards = companyBoardFixture{}
	d.importer = linkimport.New(d.pool, d.q, nil, nil, d.registry, d.boards)
	return d
}

func TestCompanyDiscoveryQueuesTheNativeBoardForOrdinaryIngestion(t *testing.T) {
	pool := testdb.Pool(t)
	ctx, q := context.Background(), db.New(pool)
	registry := sources.Taxonomy()
	registry["hrsmart"] = nativeDiscoveryFixture{}
	store := newDBStore(pool, 1, nil, newWriteTally(), defaultSeenPolicy())
	resolver := withEmployerProof(newNativeDiscovery(pool, registry, store))
	if err := resolver.Resolve(ctx, sources.CompanyEntry{Provider: "linkedin", Board: "software engineer"}, sources.Job{ExternalID: "earlier", Title: "Senior Java Software Engineer", Company: "Gist Limited", ApplyMethod: sources.ApplyExternal}); err != nil {
		t.Fatal(err)
	}
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
	resolver := withEmployerProof(newNativeDiscovery(pool, registry, store))
	err := resolver.Resolve(ctx, sources.CompanyEntry{Provider: "linkedin", Board: "software engineer"}, sources.Job{
		ExternalID: "4472812273", Title: "Senior Java Software Engineer", Company: "Gist Limited", URL: "https://www.linkedin.com/jobs/view/4472812273/",
		ApplyMethod: sources.ApplyExternal, CompanyWebsite: "https://www.gistworld.com", ExternalApplyURL: "https://gistworld.mua.hrdepartment.com/hrsmart/ats/Posting/view/3735?src=LinkedIn",
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
