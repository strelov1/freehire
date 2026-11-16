//go:build integration

package handler

import (
	"context"
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
func (intakeNativeSource) Fetch(context.Context, sources.CompanyEntry) ([]sources.Job, error) {
	return []sources.Job{{ExternalID: "3735", URL: "https://gistworld.mua.hrdepartment.com/hrsmart/ats/Posting/view/3735", Title: "Java Engineer", Company: "Gist Limited", Description: "Build Java services."}}, nil
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
