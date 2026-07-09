package jobview_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/job"
	"github.com/strelov1/freehire/internal/jobview"
)

// FromDomain must project a job to the exact wire shape the pre-aggregate FromRow
// produced from the persistence row. This golden test pins that output against a
// FROZEN oracle — expected JSON literals captured from the original FromRow while
// it still held the full implementation (they matched byte-for-byte at that point).
//
// The oracle is deliberately NOT `jobview.FromRow(row)`: FromRow is now a shim over
// job.FromRow → FromDomain, so comparing against it would be tautological and could
// never catch a mapping regression (e.g. jobFromRow dropping a column). A frozen
// literal is an independent oracle, so a future edit that silently blanks a field
// fails here.
func TestFromDomain_MatchesFrozenWireShape(t *testing.T) {
	ts := func(y int, mo time.Month, d int) pgtype.Timestamptz {
		return pgtype.Timestamptz{Time: time.Date(y, mo, d, 0, 0, 0, 0, time.UTC), Valid: true}
	}

	type fixture struct {
		row  db.Job
		want string
	}
	fixtures := map[string]fixture{
		"enriched, dict-pinned geo, counts, manual": {
			row: db.Job{
				ID: 1, Source: "greenhouse", ExternalID: "acme:1", URL: "http://x.test",
				Title: "Senior Go Developer", Company: "Acme", CompanySlug: "acme",
				PublicSlug: "senior-go-developer-acme-1", Location: "Berlin, Germany",
				Countries: []string{"de"}, Skills: []string{"go", "postgresql"},
				WorkMode: "onsite", Seniority: "senior", Category: "backend",
				PostingLanguage: "en", EmploymentType: "full_time", EducationLevel: "bachelor",
				EnglishLevel: "c1", ExperienceYearsMin: pgtype.Int4{Int32: 5, Valid: true},
				Cities:      []string{"Berlin"},
				Collections: []string{"yc", "bigtech"},
				Enrichment: json.RawMessage(
					`{"summary":"Great role","skills":["go","kubernetes"],"countries":["fr"],"cities":["Munich"],"salary_min":100000,"salary_currency":"EUR"}`),
				EnrichedAt: ts(2026, 1, 3), EnrichmentVersion: 1,
				CreatedBy: pgtype.Int8{Int64: 9, Valid: true},
				PostedAt:  ts(2026, 1, 1), CreatedAt: ts(2026, 1, 1), UpdatedAt: ts(2026, 1, 2),
				ViewCount: 4, AppliedCount: 2,
			},
			want: `{"public_slug":"senior-go-developer-acme-1","source":"greenhouse","manually_added":true,"external_id":"acme:1","url":"http://x.test","title":"Senior Go Developer","company":"Acme","company_slug":"acme","location":"Berlin, Germany","description":"","countries":["de"],"regions":[],"work_mode":"onsite","skills":["go","postgresql"],"cities":["Berlin"],"collections":["bigtech","yc"],"posted_at":"2026-01-01T00:00:00Z","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-02T00:00:00Z","closed_at":null,"enrichment":{"summary":"Great role","employment_type":"full_time","salary_min":100000,"salary_currency":"EUR","seniority":"senior","experience_years_min":5,"english_level":"c1","education_level":"bachelor","category":"backend","posting_language":"en"},"enriched_at":"2026-01-03T00:00:00Z","enrichment_version":1,"view_count":4,"applied_count":2}`,
		},
		"unenriched, empty facets": {
			row: db.Job{
				ID: 2, Source: "lever", ExternalID: "beta:2", Title: "Engineer",
				Company: "Beta", CompanySlug: "beta", PublicSlug: "engineer-beta-2",
				CreatedAt: ts(2026, 1, 5),
			},
			want: `{"public_slug":"engineer-beta-2","source":"lever","manually_added":false,"external_id":"beta:2","url":"","title":"Engineer","company":"Beta","company_slug":"beta","location":"","description":"","countries":[],"regions":[],"skills":[],"cities":[],"collections":[],"posted_at":"2026-01-05T00:00:00Z","created_at":"2026-01-05T00:00:00Z","updated_at":null,"closed_at":null,"enrichment":{},"enriched_at":null,"enrichment_version":0,"view_count":0,"applied_count":0}`,
		},
		"closed posting": {
			row: db.Job{
				ID: 3, Source: "ashby", ExternalID: "g:3", Title: "Dev", Company: "Gamma",
				CompanySlug: "gamma", PublicSlug: "dev-gamma-3",
				CreatedAt: ts(2026, 1, 4), ClosedAt: ts(2026, 2, 1),
			},
			want: `{"public_slug":"dev-gamma-3","source":"ashby","manually_added":false,"external_id":"g:3","url":"","title":"Dev","company":"Gamma","company_slug":"gamma","location":"","description":"","countries":[],"regions":[],"skills":[],"cities":[],"collections":[],"posted_at":"2026-01-04T00:00:00Z","created_at":"2026-01-04T00:00:00Z","updated_at":null,"closed_at":"2026-02-01T00:00:00Z","enrichment":{},"enriched_at":null,"enrichment_version":0,"view_count":0,"applied_count":0}`,
		},
		"geo hybrid: dict unpinned, LLM restricts": {
			row: db.Job{
				ID: 4, Source: "manual", ExternalID: "d:4", Title: "Remote Dev", Company: "Delta",
				CompanySlug: "delta", PublicSlug: "remote-dev-delta-4",
				Regions:    []string{"global"},
				WorkMode:   "remote",
				Enrichment: json.RawMessage(`{"countries":["es"],"regions":["europe"]}`),
				CreatedAt:  ts(2026, 1, 6),
			},
			want: `{"public_slug":"remote-dev-delta-4","source":"manual","manually_added":false,"external_id":"d:4","url":"","title":"Remote Dev","company":"Delta","company_slug":"delta","location":"","description":"","countries":["es"],"regions":["europe"],"work_mode":"remote","skills":[],"cities":[],"collections":[],"posted_at":"2026-01-06T00:00:00Z","created_at":"2026-01-06T00:00:00Z","updated_at":null,"closed_at":null,"enrichment":{},"enriched_at":null,"enrichment_version":0,"view_count":0,"applied_count":0}`,
		},
	}

	for name, fx := range fixtures {
		t.Run(name, func(t *testing.T) {
			j, x, err := job.FromRow(fx.row)
			if err != nil {
				t.Fatalf("job.FromRow: %v", err)
			}
			got, err := jobview.FromDomain(j, x)
			if err != nil {
				t.Fatalf("FromDomain: %v", err)
			}
			gotJSON, err := json.Marshal(got)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if string(gotJSON) != fx.want {
				t.Errorf("projection drifted from frozen wire shape:\n want = %s\n got  = %s", fx.want, gotJSON)
			}
		})
	}
}
