//go:build integration

package handler

import (
	"context"
	"fmt"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/contribution"
	"github.com/strelov1/freehire/internal/ingest/linkimport"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
)

type intakeLinkedInSource struct{ posting sources.Job }

func (intakeLinkedInSource) Provider() string { return "linkedin" }
func (intakeLinkedInSource) Fetch(context.Context, sources.CompanyEntry) ([]sources.Job, error) {
	return nil, nil
}
func (s intakeLinkedInSource) Posting(context.Context, string) (sources.Job, error) {
	return s.posting, nil
}

type intakeNativeSource struct{}

func (intakeNativeSource) Provider() string { return "hrsmart" }
func (intakeNativeSource) Fetch(_ context.Context, entry sources.CompanyEntry) ([]sources.Job, error) {
	return []sources.Job{{ExternalID: "3735", URL: "https://gistworld.mua.hrdepartment.com/hrsmart/ats/Posting/view/3735", Title: "Java Engineer", Company: entry.Company, Description: "Build Java services."}}, nil
}

type intakeEmployerProof struct{}

func (intakeEmployerProof) Resolve(context.Context, string) (string, string, string, bool) {
	return "", "", "", false
}
func (intakeEmployerProof) ResolveCompany(context.Context, string) (string, string, string, bool) {
	return "hrsmart", "gistworld.mua.hrdepartment.com", "https://gistworld.mua.hrdepartment.com/hrsmart/ats/JobSearch/viewAll", true
}

func TestLinkedInIntakeUsesApplicationOwnershipAndRecordsTheCaller(t *testing.T) {
	for _, tc := range []struct {
		name       string
		method     sources.ApplicationMethod
		target     string
		website    string
		wantSource string
		wantStatus string
		wantBoard  string
	}{
		{"internal apply", sources.ApplyOnSite, "", "", "linkedin", outcomeReview, ""},
		{"external apply", sources.ApplyExternal, "https://gistworld.mua.hrdepartment.com/hrsmart/ats/Posting/view/3735", "https://www.gistworld.com", "hrsmart", outcomeImported, "gistworld.mua.hrdepartment.com"},
		{"hidden external", sources.ApplyExternal, "", "https://www.gistworld.com", "", outcomeQueued, "gistworld.mua.hrdepartment.com"},
		{"unknown method", "", "", "", "", outcomeQueued, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pool := startPostgres(t)
			ctx := context.Background()
			q := db.New(pool)
			var userID int64
			if err := pool.QueryRow(ctx, "INSERT INTO users(email) VALUES('linkedin-contributor@example.test') RETURNING id").Scan(&userID); err != nil {
				t.Fatal(err)
			}
			posting := sources.Job{ExternalID: "123", URL: "https://www.linkedin.com/jobs/view/123/", Title: "Java Engineer", Company: "Gist Limited", Description: "Build Java services.", ApplyMethod: tc.method, ExternalApplyURL: tc.target, CompanyWebsite: tc.website}
			registry := map[string]sources.Source{"linkedin": intakeLinkedInSource{posting: posting}, "hrsmart": intakeNativeSource{}}
			svc := contribution.New(contribution.NewQueriesRepository(q), nil)
			intake := &intakeService{queries: q, contribution: svc, imports: linkimport.New(pool, q, nil, pagesClient{"linkedin.com": vacancyPage}, registry, intakeEmployerProof{})}
			out, err := intake.Resolve(ctx, userID, "https://nl.linkedin.com/jobs/view/java-engineer-123/?tracking=1", contribution.SurfaceWeb)
			if err != nil {
				t.Fatal(err)
			}
			if out.Status != tc.wantStatus || out.Board != tc.wantBoard {
				t.Fatalf("outcome=%+v", out)
			}
			var count int
			if err := pool.QueryRow(ctx, "SELECT count(*) FROM jobs").Scan(&count); err != nil {
				t.Fatal(err)
			}
			if tc.wantSource == "" {
				if count != 0 {
					t.Fatalf("unverified discovery published %d jobs", count)
				}
			} else {
				if count != 1 {
					t.Fatalf("jobs=%d", count)
				}
				row, err := q.GetJobBySlug(ctx, out.PublicSlug)
				if err != nil {
					t.Fatal(err)
				}
				if row.Source != tc.wantSource {
					t.Fatalf("source=%s", row.Source)
				}
				if tc.wantSource == "linkedin" && row.ExternalID != "123" {
					t.Fatalf("identity=%s", row.ExternalID)
				}
			}
			rows, err := contribution.NewQueriesRepository(q).ListByUser(ctx, userID)
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != 1 || rows[0].SubmittedBy != userID || rows[0].Surface != contribution.SurfaceWeb || rows[0].Board != tc.wantBoard {
				t.Fatalf("contributions=%+v", rows)
			}
			if tc.wantBoard != "" {
				schedule, err := q.GetIngestSchedule(ctx, "hrsmart")
				if err != nil || !schedule.Enabled || !schedule.Managed {
					t.Fatalf("contributed native board has no polling schedule: %+v, %v", schedule, err)
				}
				var company string
				if err := pool.QueryRow(ctx, "SELECT company FROM boards WHERE provider='hrsmart'").Scan(&company); err != nil || company != "Gist Limited" {
					t.Fatalf("native employer label = %q, %v", company, err)
				}
			}
			if tc.wantBoard != "" && !out.NovelBoard {
				t.Fatalf("destination was inspected too late: %+v", out)
			}
			if tc.wantSource == "linkedin" {
				again, err := intake.Resolve(ctx, userID, "https://www.linkedin.com/jobs/view/123/", contribution.SurfaceWeb)
				if err != nil || again.Status != outcomeFound || again.PublicSlug != out.PublicSlug {
					t.Fatalf("repeat=%+v err=%v", again, err)
				}
			}
		})
	}
}

func TestLinkedInIntakeFindsAnAlreadyCarriedNativePosting(t *testing.T) {
	pool, ctx := startPostgres(t), context.Background()
	q := db.New(pool)
	var userID int64
	if err := pool.QueryRow(ctx, "INSERT INTO users(email) VALUES('existing-native-contributor@example.test') RETURNING id").Scan(&userID); err != nil {
		t.Fatal(err)
	}
	posting := sources.Job{ExternalID: "123", Title: "Java Engineer", Company: "Gist Limited", Description: "Build Java services.", ApplyMethod: sources.ApplyExternal, ExternalApplyURL: "https://gistworld.mua.hrdepartment.com/hrsmart/ats/Posting/view/3735", CompanyWebsite: "https://www.gistworld.com"}
	registry := map[string]sources.Source{"linkedin": intakeLinkedInSource{posting: posting}, "hrsmart": intakeNativeSource{}}
	imports := linkimport.New(pool, q, nil, nil, registry, intakeEmployerProof{})
	nativeJob := posting
	nativeJob.ExternalID, nativeJob.URL = "gistworld.mua.hrdepartment.com:3735", posting.ExternalApplyURL
	original, err := imports.WriteNative(ctx, nil, linkimport.NativeDestination{Entry: sources.CompanyEntry{Provider: "hrsmart", Board: "gistworld.mua.hrdepartment.com", Company: posting.Company}, Job: nativeJob})
	if err != nil {
		t.Fatal(err)
	}
	svc := contribution.New(contribution.NewQueriesRepository(q), nil)
	intake := &intakeService{queries: q, contribution: svc, imports: imports}
	out, err := intake.Resolve(ctx, userID, "https://www.linkedin.com/jobs/view/123/", contribution.SurfaceWeb)
	if err != nil || out.Status != outcomeFound || out.PublicSlug != original.PublicSlug || out.NovelBoard {
		t.Fatalf("already-carried native outcome=%+v error=%v", out, err)
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM jobs").Scan(&count); err != nil || count != 1 {
		t.Fatalf("existing native posting was copied: rows=%d error=%v", count, err)
	}
}

type intakeEULeverEmployerProof struct{}

func (intakeEULeverEmployerProof) Resolve(context.Context, string) (string, string, string, bool) {
	return "", "", "", false
}
func (intakeEULeverEmployerProof) ResolveCompany(context.Context, string) (string, string, string, bool) {
	return "lever", "acme-eu", "https://jobs.eu.lever.co/acme-eu", true
}

type intakeEULeverSource struct{}

func (intakeEULeverSource) Provider() string { return "lever" }
func (intakeEULeverSource) Fetch(_ context.Context, entry sources.CompanyEntry) ([]sources.Job, error) {
	if entry.Region != "eu" {
		return nil, fmt.Errorf("unexpected Lever region %q", entry.Region)
	}
	return []sources.Job{{ExternalID: "11111111-1111-4111-8111-111111111111", URL: "https://jobs.eu.lever.co/acme-eu/11111111-1111-4111-8111-111111111111", Title: "Java Engineer", Company: "Acme EU", Description: "Build Java services."}}, nil
}

func TestLinkedInIntakePreservesDiscoveredEuropeanBoardRegion(t *testing.T) {
	for _, readable := range []bool{false, true} {
		t.Run(fmt.Sprint(readable), func(t *testing.T) {
			pool, ctx := startPostgres(t), context.Background()
			q := db.New(pool)
			var userID int64
			if err := pool.QueryRow(ctx, "INSERT INTO users(email) VALUES('eu-board-contributor@example.test') RETURNING id").Scan(&userID); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, "INSERT INTO boards(provider,board,region,company,status) VALUES('lever','acme-eu','','Acme US','active')"); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `INSERT INTO jobs(source,external_id,url,title,company,company_slug,public_slug)
				VALUES('lever','acme-eu:22222222-2222-4222-8222-222222222222','https://jobs.lever.co/acme-eu/22222222-2222-4222-8222-222222222222','Engineer','Acme US','acme-us','lever-us-control')`); err != nil {
				t.Fatal(err)
			}
			posting := sources.Job{ExternalID: "123", Title: "Java Engineer", Company: "Acme EU", Description: "Build Java services.", ApplyMethod: sources.ApplyExternal, CompanyWebsite: "https://acme.example"}
			wantStatus := outcomeQueued
			if readable {
				posting.ExternalApplyURL = "https://jobs.eu.lever.co/acme-eu/11111111-1111-4111-8111-111111111111"
				wantStatus = outcomeImported
			}
			registry := sources.Taxonomy()
			registry["linkedin"] = intakeLinkedInSource{posting: posting}
			registry["lever"] = intakeEULeverSource{}
			svc := contribution.New(contribution.NewQueriesRepository(q), nil)
			intake := &intakeService{queries: q, contribution: svc, imports: linkimport.New(pool, q, nil, nil, registry, intakeEULeverEmployerProof{})}
			out, err := intake.Resolve(ctx, userID, "https://www.linkedin.com/jobs/view/123/", contribution.SurfaceWeb)
			if err != nil || out.Status != wantStatus || out.Board != "acme-eu" {
				t.Fatalf("EU board outcome=%+v error=%v", out, err)
			}
			var region string
			var owner int64
			if err := pool.QueryRow(ctx, "SELECT region,submitted_by FROM boards WHERE provider='lever' AND board='acme-eu' AND region='eu'").Scan(&region, &owner); err != nil || region != "eu" || owner != userID {
				t.Fatalf("contributed board region=%q owner=%d error=%v", region, owner, err)
			}
			var count int
			if err := pool.QueryRow(ctx, "SELECT count(*) FROM boards WHERE provider='lever' AND board='acme-eu'").Scan(&count); err != nil || count != 2 {
				t.Fatalf("regional board not separated: count=%d error=%v", count, err)
			}
			if readable {
				if _, err := pool.Exec(ctx, `UPDATE boards SET hub=true,tenants='{"keep":"stored"}'::jsonb WHERE provider='lever' AND board='acme-eu' AND region='eu'`); err != nil {
					t.Fatal(err)
				}
				verified, err := intake.imports.PrepareLinkedIn(ctx, "https://www.linkedin.com/jobs/view/123/")
				if err != nil || verified.Native == nil || !verified.Native.Entry.Hub || verified.Native.Entry.Tenants["keep"] != "stored" {
					t.Fatalf("stored EU configuration was replaced: %+v error=%v", verified, err)
				}
			}
			var controlCompany string
			if err := pool.QueryRow(ctx, "SELECT company FROM jobs WHERE public_slug='lever-us-control'").Scan(&controlCompany); err != nil || controlCompany != "Acme US" {
				t.Fatalf("other residency was changed: company=%q error=%v", controlCompany, err)
			}
		})
	}
}

type intakeVanityICIMSSource struct{ unavailable bool }

func (intakeVanityICIMSSource) Provider() string { return "icims" }
func (s intakeVanityICIMSSource) Fetch(context.Context, sources.CompanyEntry) ([]sources.Job, error) {
	if s.unavailable {
		return nil, fmt.Errorf("native posting unavailable")
	}
	return []sources.Job{{ExternalID: "1234", URL: "https://careers.peraton.com/jobs/1234/job", Title: "Java Engineer", Company: "Peraton", Description: "Build Java services."}}, nil
}

type intakeVanityProof struct{}

func (intakeVanityProof) Resolve(context.Context, string) (string, string, string, bool) {
	return "icims", "careers-peraton.icims.com", "https://careers-peraton.icims.com", true
}
func (intakeVanityProof) ResolveCompany(context.Context, string) (string, string, string, bool) {
	return "icims", "careers-peraton.icims.com", "https://careers-peraton.icims.com", true
}

func TestLinkedInIntakeAttributesVerifiedVanityNativeBoard(t *testing.T) {
	for _, unavailable := range []bool{false, true} {
		t.Run(fmt.Sprint(unavailable), func(t *testing.T) {
			pool, ctx := startPostgres(t), context.Background()
			q := db.New(pool)
			var userID int64
			if err := pool.QueryRow(ctx, "INSERT INTO users(email) VALUES('vanity-contributor@example.test') RETURNING id").Scan(&userID); err != nil {
				t.Fatal(err)
			}
			posting := sources.Job{ExternalID: "123", Title: "Java Engineer", Company: "Peraton", Description: "Build Java services.", ApplyMethod: sources.ApplyExternal, ExternalApplyURL: "https://careers.peraton.com/jobs/1234/job", CompanyWebsite: "https://www.peraton.com"}
			registry := map[string]sources.Source{"linkedin": intakeLinkedInSource{posting: posting}, "icims": intakeVanityICIMSSource{unavailable: unavailable}}
			svc := contribution.New(contribution.NewQueriesRepository(q), nil)
			intake := &intakeService{queries: q, contribution: svc, imports: linkimport.New(pool, q, nil, nil, registry, intakeVanityProof{})}
			out, err := intake.Resolve(ctx, userID, "https://www.linkedin.com/jobs/view/123/", contribution.SurfaceExtension)
			wantStatus := outcomeImported
			if unavailable {
				wantStatus = outcomeQueued
			}
			if err != nil || out.Status != wantStatus || out.Board != "careers-peraton.icims.com" {
				t.Fatalf("vanity outcome=%+v error=%v", out, err)
			}
			var owner int64
			var surface string
			if err := pool.QueryRow(ctx, "SELECT submitted_by,surface FROM boards WHERE provider='icims' AND board='careers-peraton.icims.com'").Scan(&owner, &surface); err != nil || owner != userID || surface != contribution.SurfaceExtension {
				t.Fatalf("vanity board owner=%d surface=%q error=%v", owner, surface, err)
			}
			cfg, err := q.GetIngestSchedule(ctx, "icims")
			if err != nil || !cfg.Enabled || !cfg.Managed {
				t.Fatalf("verified vanity board not scheduled: %+v error=%v", cfg, err)
			}
			if unavailable {
				var company string
				if err := pool.QueryRow(ctx, "SELECT company FROM boards WHERE provider='icims'").Scan(&company); err != nil || company != "Peraton" {
					t.Fatalf("verified vanity employer=%q error=%v", company, err)
				}
				var jobs int
				if err := pool.QueryRow(ctx, "SELECT count(*) FROM jobs").Scan(&jobs); err != nil || jobs != 0 {
					t.Fatalf("unreadable vanity posting published: rows=%d error=%v", jobs, err)
				}
			}
		})
	}
}

type intakeUnavailableNativeSource struct{}

func (intakeUnavailableNativeSource) Provider() string { return "hrsmart" }
func (intakeUnavailableNativeSource) Fetch(context.Context, sources.CompanyEntry) ([]sources.Job, error) {
	return nil, fmt.Errorf("temporary ATS outage")
}

func TestLinkedInIntakeSchedulesRecognizedBoardAfterNativeReadFailure(t *testing.T) {
	for _, paused := range []bool{false, true} {
		t.Run(fmt.Sprint(paused), func(t *testing.T) {
			pool, ctx := startPostgres(t), context.Background()
			q := db.New(pool)
			var userID int64
			if err := pool.QueryRow(ctx, "INSERT INTO users(email) VALUES('failed-native-contributor@example.test') RETURNING id").Scan(&userID); err != nil {
				t.Fatal(err)
			}
			if paused {
				if _, err := pool.Exec(ctx, "INSERT INTO ingest_schedule(provider,enabled,managed,disabled_reason) VALUES('hrsmart',false,true,'operator pause')"); err != nil {
					t.Fatal(err)
				}
			}
			posting := sources.Job{ExternalID: "123", Title: "Java Engineer", Company: "Gist Limited", Description: "Build Java services.", ApplyMethod: sources.ApplyExternal, ExternalApplyURL: "https://gistworld.mua.hrdepartment.com/hrsmart/ats/Posting/view/3735", CompanyWebsite: "https://www.gistworld.com"}
			registry := map[string]sources.Source{"linkedin": intakeLinkedInSource{posting: posting}, "hrsmart": intakeUnavailableNativeSource{}}
			svc := contribution.New(contribution.NewQueriesRepository(q), nil)
			intake := &intakeService{queries: q, contribution: svc, imports: linkimport.New(pool, q, nil, nil, registry, intakeEmployerProof{})}
			out, err := intake.Resolve(ctx, userID, "https://www.linkedin.com/jobs/view/123/", contribution.SurfaceWeb)
			if err != nil || out.Status != outcomeQueued || out.PublicSlug != "" {
				t.Fatalf("unreadable native outcome=%+v error=%v", out, err)
			}
			cfg, err := q.GetIngestSchedule(ctx, "hrsmart")
			if err != nil || cfg.Enabled == paused || !cfg.Managed {
				t.Fatalf("recorded board schedule=%+v error=%v paused=%t", cfg, err, paused)
			}
			var jobs int
			if err := pool.QueryRow(ctx, "SELECT count(*) FROM jobs").Scan(&jobs); err != nil || jobs != 0 {
				t.Fatalf("unreadable native published jobs=%d error=%v", jobs, err)
			}
			if !paused {
				var company string
				if err := pool.QueryRow(ctx, "SELECT company FROM boards WHERE provider='hrsmart'").Scan(&company); err != nil || company != "Gist Limited" {
					t.Fatalf("verified employer was lost after fetch failure: company=%q error=%v", company, err)
				}
				// HRsmart labels jobs from the board entry. A placeholder recorded
				// during the outage would poison both the later crawl and this retry.
				registry["hrsmart"] = intakeNativeSource{}
				intake.imports = linkimport.New(pool, q, nil, nil, registry, intakeEmployerProof{})
				again, err := intake.Resolve(ctx, userID, "https://www.linkedin.com/jobs/view/123/", contribution.SurfaceWeb)
				if err != nil || again.Status != outcomeImported {
					t.Fatalf("recovered native outcome=%+v error=%v", again, err)
				}
				saved, err := q.GetJobBySlug(ctx, again.PublicSlug)
				if err != nil || saved.Source != "hrsmart" || saved.Company != "Gist Limited" {
					t.Fatalf("recovered native employer=%q source=%q error=%v", saved.Company, saved.Source, err)
				}
			}
		})
	}
}
