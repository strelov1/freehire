package search

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
)

func TestFromCompany_MapsAllFieldsAndFacets(t *testing.T) {
	c := db.Company{
		Slug:          "acme",
		Name:          "Acme Inc",
		Tagline:       pgtype.Text{String: "We build things", Valid: true},
		Industries:    []string{"Fintech"},
		HqCountry:     pgtype.Text{String: "de", Valid: true},
		JobCount:      7,
		Collections:   []string{"yc"},
		Regions:       []string{"europe"},
		Countries:     []string{"de"},
		Domains:       []string{"fintech"},
		CompanyTypes:  []string{"startup"},
		CompanySizes:  []string{"11-50"},
		RemoteRegions: []string{"eu"},
		YcBatch:       []string{"W21"},
		YcStatus:      []string{"Active"},
		YcStage:       []string{"Growth"},
		YcFlags:       []string{"top_company"},
		Maturity:      pgtype.Text{String: "startup", Valid: true},
		Subindustry:   pgtype.Text{String: "Payments", Valid: true},
	}
	doc := FromCompany(c)

	if doc.Slug != "acme" || doc.Name != "Acme Inc" || doc.JobCount != 7 {
		t.Fatalf("core fields wrong: %+v", doc)
	}
	if doc.Tagline != "We build things" || doc.HqCountry != "de" {
		t.Errorf("pgtype scalars not unwrapped: tagline=%q hq=%q", doc.Tagline, doc.HqCountry)
	}
	if doc.Maturity != "startup" || doc.Subindustry != "Payments" {
		t.Errorf("scalar facets wrong: maturity=%q sub=%q", doc.Maturity, doc.Subindustry)
	}

	for _, tc := range []struct {
		name      string
		got, want []string
	}{
		{"industries", doc.Industries, []string{"Fintech"}},
		{"collections", doc.Collections, []string{"yc"}},
		{"regions", doc.Regions, []string{"europe"}},
		{"countries", doc.Countries, []string{"de"}},
		{"domains", doc.Domains, []string{"fintech"}},
		{"company_types", doc.CompanyTypes, []string{"startup"}},
		{"company_sizes", doc.CompanySizes, []string{"11-50"}},
		{"remote_regions", doc.RemoteRegions, []string{"eu"}},
		{"yc_batch", doc.YcBatch, []string{"W21"}},
		{"yc_status", doc.YcStatus, []string{"Active"}},
		{"yc_stage", doc.YcStage, []string{"Growth"}},
		{"yc_flags", doc.YcFlags, []string{"top_company"}},
	} {
		if !slices.Equal(tc.got, tc.want) {
			t.Errorf("%s = %v, want %v", tc.name, tc.got, tc.want)
		}
	}
}

func TestFromCompany_NullScalarsBecomeEmpty(t *testing.T) {
	// A NULL tagline/hq_country/maturity/subindustry (pgtype.Text zero value) must
	// map to an empty string, not leak an invalid pgtype into the document JSON.
	doc := FromCompany(db.Company{Slug: "x", Name: "X"})
	if doc.Tagline != "" || doc.HqCountry != "" || doc.Maturity != "" || doc.Subindustry != "" {
		t.Errorf("NULL pgtype.Text should map to empty string, got %+v", doc)
	}
}

func TestFromCompany_SlugIsTopLevelPrimaryKey(t *testing.T) {
	// Meilisearch reads the primary key "slug" from the top level of the document,
	// and job_count must be present so the sortable ranking tiebreaker can read it.
	doc := FromCompany(db.Company{Slug: "acme", Name: "Acme", JobCount: 3})
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got := string(m["slug"]); got != `"acme"` {
		t.Errorf("top-level slug = %s, want \"acme\" in %s", got, raw)
	}
	if got := string(m["job_count"]); got != "3" {
		t.Errorf("top-level job_count = %s, want 3 in %s", got, raw)
	}
}

func TestFromCompany_SiteFromCompanyInfo(t *testing.T) {
	for _, tc := range []struct {
		name, info, website, linkedin string
	}{
		{"website wins", `{"website":" https://gomotive.com ","homepage":"wearemotive.com","linkedin":"https://linkedin.com/company/motive-inc"}`, "https://gomotive.com", "https://linkedin.com/company/motive-inc"},
		{"homepage fallback", `{"website":"  ","homepage":"acme.example"}`, "acme.example", ""},
		// The SQL path trims ASCII whitespace only; a no-break space is kept on both paths.
		{"no-break space kept", "{\"website\":\"\u00a0\",\"homepage\":\"acme.example\",\"linkedin\":\" \u00a0 \"}", "\u00a0", "\u00a0"},
		{"nothing recorded", `{}`, "", ""},
		{"malformed blob", `not json`, "", ""},
		{"no blob", ``, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := FromCompany(db.Company{Slug: "x", Name: "X", CompanyInfo: json.RawMessage(tc.info)})
			if doc.Website != tc.website || doc.LinkedIn != tc.linkedin {
				t.Errorf("site = (%q, %q), want (%q, %q)", doc.Website, doc.LinkedIn, tc.website, tc.linkedin)
			}
		})
	}
}
