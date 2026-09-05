package jobview_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/job/job"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/db"
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
			want: `{"public_slug":"senior-go-developer-acme-1","source":"greenhouse","manually_added":true,"external_id":"acme:1","url":"http://x.test?utm_source=freehire.me","title":"Senior Go Developer","company":"Acme","company_slug":"acme","location":"Berlin, Germany","description":"","countries":["de"],"regions":[],"work_mode":"onsite","skills":["go","postgresql"],"cities":["Berlin"],"collections":["bigtech","yc"],"posted_at":"2026-01-01T00:00:00Z","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-02T00:00:00Z","last_seen_at":null,"closed_at":null,"enrichment":{"summary":"Great role","employment_type":"full_time","salary_min":100000,"salary_currency":"EUR","seniority":"senior","experience_years_min":5,"english_level":"c1","education_level":"bachelor","category":"backend","posting_language":"en"},"enriched_at":"2026-01-03T00:00:00Z","enrichment_version":1,"view_count":4,"applied_count":2,"upvote_count":0,"downvote_count":0,"my_vote":0}`,
		},
		"unenriched, empty facets": {
			row: db.Job{
				ID: 2, Source: "lever", ExternalID: "beta:2", Title: "Engineer",
				Company: "Beta", CompanySlug: "beta", PublicSlug: "engineer-beta-2",
				CreatedAt: ts(2026, 1, 5),
			},
			want: `{"public_slug":"engineer-beta-2","source":"lever","manually_added":false,"external_id":"beta:2","url":"","title":"Engineer","company":"Beta","company_slug":"beta","location":"","description":"","countries":[],"regions":[],"skills":[],"cities":[],"collections":[],"posted_at":"2026-01-05T00:00:00Z","created_at":"2026-01-05T00:00:00Z","updated_at":null,"last_seen_at":null,"closed_at":null,"enrichment":{},"enriched_at":null,"enrichment_version":0,"view_count":0,"applied_count":0,"upvote_count":0,"downvote_count":0,"my_vote":0}`,
		},
		"closed posting": {
			row: db.Job{
				ID: 3, Source: "ashby", ExternalID: "g:3", Title: "Dev", Company: "Gamma",
				CompanySlug: "gamma", PublicSlug: "dev-gamma-3",
				CreatedAt: ts(2026, 1, 4), ClosedAt: ts(2026, 2, 1),
			},
			want: `{"public_slug":"dev-gamma-3","source":"ashby","manually_added":false,"external_id":"g:3","url":"","title":"Dev","company":"Gamma","company_slug":"gamma","location":"","description":"","countries":[],"regions":[],"skills":[],"cities":[],"collections":[],"posted_at":"2026-01-04T00:00:00Z","created_at":"2026-01-04T00:00:00Z","updated_at":null,"last_seen_at":null,"closed_at":"2026-02-01T00:00:00Z","enrichment":{},"enriched_at":null,"enrichment_version":0,"view_count":0,"applied_count":0,"upvote_count":0,"downvote_count":0,"my_vote":0}`,
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
			want: `{"public_slug":"remote-dev-delta-4","source":"manual","manually_added":false,"external_id":"d:4","url":"","title":"Remote Dev","company":"Delta","company_slug":"delta","location":"","description":"","countries":["es"],"regions":["europe"],"work_mode":"remote","skills":[],"cities":[],"collections":[],"posted_at":"2026-01-06T00:00:00Z","created_at":"2026-01-06T00:00:00Z","updated_at":null,"last_seen_at":null,"closed_at":null,"enrichment":{},"enriched_at":null,"enrichment_version":0,"view_count":0,"applied_count":0,"upvote_count":0,"downvote_count":0,"my_vote":0}`,
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

// Requirements is a served enrichment field like summary/salary/visa_sponsorship —
// FromDomain does not fold or zero it out, unlike the dict-covered facets
// (seniority, category, skills, etc.). This pins that design decision with a test
// rather than leaving it as unverified default behavior.
func TestFromDomain_RequirementsPassThroughUnchanged(t *testing.T) {
	row := db.Job{
		ID: 5, Source: "greenhouse", ExternalID: "acme:5", Title: "Backend Engineer",
		Company: "Acme", CompanySlug: "acme", PublicSlug: "backend-engineer-acme-5",
		CreatedAt: pgtype.Timestamptz{Time: time.Date(2026, 1, 7, 0, 0, 0, 0, time.UTC), Valid: true},
		Enrichment: json.RawMessage(
			`{"requirements":[{"text":"5+ years Go","priority":"required"},{"text":"Kubernetes","priority":"preferred"}]}`),
	}

	j, x, err := job.FromRow(row)
	if err != nil {
		t.Fatalf("job.FromRow: %v", err)
	}
	got, err := jobview.FromDomain(j, x)
	if err != nil {
		t.Fatalf("FromDomain: %v", err)
	}

	want := []struct {
		Text     string `json:"text"`
		Priority string `json:"priority"`
	}{
		{Text: "5+ years Go", Priority: "required"},
		{Text: "Kubernetes", Priority: "preferred"},
	}
	if len(got.Enrichment.Requirements) != len(want) {
		t.Fatalf("Requirements = %v, want %v", got.Enrichment.Requirements, want)
	}
	for i, r := range got.Enrichment.Requirements {
		if r.Text != want[i].Text || r.Priority != want[i].Priority {
			t.Errorf("Requirements[%d] = %+v, want %+v", i, r, want[i])
		}
	}
}

// The whole point of storing a deterministic derivation: a posting the model has never
// reached must still SERVE its requirements. The SQL overlay in SetJobEnrichment
// cannot deliver that — it runs only when the model runs — so the fold has to happen
// on the read path, and this is the test that says so.
func TestFromDomain_DerivedRequirementsServeAnUnenrichedJob(t *testing.T) {
	row := db.Job{
		ID: 6, Source: "greenhouse", ExternalID: "acme:6", Title: "Backend Engineer",
		Company: "Acme", CompanySlug: "acme", PublicSlug: "backend-engineer-acme-6",
		CreatedAt: pgtype.Timestamptz{Time: time.Date(2026, 1, 7, 0, 0, 0, 0, time.UTC), Valid: true},
		// Never enriched: no payload, version 0.
		RequirementsDerived: json.RawMessage(
			`[{"text":"5+ years of Go","priority":"required"},{"text":"Kubernetes","priority":"preferred"}]`),
	}

	j, x, err := job.FromRow(row)
	if err != nil {
		t.Fatalf("job.FromRow: %v", err)
	}
	got, err := jobview.FromDomain(j, x)
	if err != nil {
		t.Fatalf("FromDomain: %v", err)
	}

	if len(got.Enrichment.Requirements) != 2 {
		t.Fatalf("Requirements = %v, want the two derived entries", got.Enrichment.Requirements)
	}
	if got.Enrichment.Requirements[0].Text != "5+ years of Go" ||
		got.Enrichment.Requirements[1].Priority != "preferred" {
		t.Errorf("Requirements = %+v, want the derived list verbatim", got.Enrichment.Requirements)
	}
	// The derivation is not enrichment: it must not make an unenriched job look
	// enriched, because the enrichment queue is gated on exactly that stamp.
	if got.EnrichmentVersion != 0 || got.EnrichedAt != nil {
		t.Errorf("provenance = version %d / %v, want an unenriched job", got.EnrichmentVersion, got.EnrichedAt)
	}
}

// The model wins where it has a reading: it reaches the postings whose requirements are
// prose with no list markup, which the extractor cannot.
func TestFromDomain_ModelRequirementsWinOverTheDerivation(t *testing.T) {
	row := db.Job{
		ID: 7, Source: "greenhouse", ExternalID: "acme:7", Title: "Backend Engineer",
		Company: "Acme", CompanySlug: "acme", PublicSlug: "backend-engineer-acme-7",
		CreatedAt:  pgtype.Timestamptz{Time: time.Date(2026, 1, 7, 0, 0, 0, 0, time.UTC), Valid: true},
		Enrichment: json.RawMessage(`{"requirements":[{"text":"Rust","priority":"required"}]}`),
		RequirementsDerived: json.RawMessage(
			`[{"text":"5+ years of Go","priority":"required"}]`),
	}

	j, x, err := job.FromRow(row)
	if err != nil {
		t.Fatalf("job.FromRow: %v", err)
	}
	got, err := jobview.FromDomain(j, x)
	if err != nil {
		t.Fatalf("FromDomain: %v", err)
	}

	if len(got.Enrichment.Requirements) != 1 || got.Enrichment.Requirements[0].Text != "Rust" {
		t.Errorf("Requirements = %+v, want the model's own list", got.Enrichment.Requirements)
	}
}

// A row that predates migration 0139, or one whose payload is unreadable, must serve no
// requirements rather than fail the read: the column is a display convenience, and
// losing a whole posting over it is the worse trade.
func TestFromDomain_UnreadableDerivedRequirementsAreNotFatal(t *testing.T) {
	for name, payload := range map[string]json.RawMessage{
		"absent":        nil,
		"empty array":   json.RawMessage(`[]`),
		"not a list":    json.RawMessage(`{"text":"Go"}`),
		"not even json": json.RawMessage(`{`),
	} {
		t.Run(name, func(t *testing.T) {
			row := db.Job{
				ID: 8, Source: "greenhouse", ExternalID: "acme:8", Title: "Backend Engineer",
				Company: "Acme", CompanySlug: "acme", PublicSlug: "backend-engineer-acme-8",
				CreatedAt:           pgtype.Timestamptz{Time: time.Date(2026, 1, 7, 0, 0, 0, 0, time.UTC), Valid: true},
				RequirementsDerived: payload,
			}

			j, x, err := job.FromRow(row)
			if err != nil {
				t.Fatalf("job.FromRow: %v", err)
			}
			got, err := jobview.FromDomain(j, x)
			if err != nil {
				t.Fatalf("FromDomain: %v", err)
			}
			if len(got.Enrichment.Requirements) != 0 {
				t.Errorf("Requirements = %v, want none", got.Enrichment.Requirements)
			}
		})
	}
}
