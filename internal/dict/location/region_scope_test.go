package location

import (
	"slices"
	"testing"
)

func TestRegionScopeFromDescription(t *testing.T) {
	tests := []struct {
		name          string
		desc          string
		wantCountries []string
		wantRegions   []string
	}{
		{
			// The verbatim creative-fabrica report (freehire job_reports 31/33): a clean
			// dictionary token ("New Zealand") resolves; the noisy ones ("Australia East
			// Coast", "nearby time zones") resolve nothing rather than a guess.
			"remote role within, one clean token and noise",
			"This is a fully remote role within New Zealand, Australia East Coast or nearby time zones.",
			[]string{"nz"}, []string{"apac"},
		},
		{
			"based in, a clean multi-country list",
			"This role is open to candidates based in the United States, Canada, Argentina, or Brazil.",
			[]string{"ar", "br", "ca", "us"}, []string{"latam", "north_america"},
		},
		{
			// The verbatim kard-financial report (freehire job_reports #30).
			"hiring in, a trailing-only qualifier",
			"We are a fully remote company hiring in the US, Canada, Argentina, or Brazil only.",
			[]string{"ar", "br", "ca", "us"}, []string{"latam", "north_america"},
		},
		{
			"incidental region mention is not a restriction",
			"We serve customers across Europe and have offices worldwide.",
			nil, nil,
		},
		{
			// Code-review finding: a company-HQ "About Us" mention must not be read as a
			// role restriction — the role statement later in the same description says
			// the opposite (open worldwide), and the bare "based in" anchor was matching
			// the HQ sentence first regardless of which one the role actually asserts.
			"a company-HQ mention is not a role restriction (worldwide role stated too)",
			"Our company is based in Berlin, Germany, but this role is fully remote and open worldwide.",
			nil, nil,
		},
		{
			"a company-HQ mention is not a role restriction (no role statement at all)",
			"We are based in San Francisco. This position is fully remote, open to candidates anywhere in the world.",
			nil, nil,
		},
		{
			"a negated restriction statement yields nothing",
			"This role is not restricted to the US, we welcome applicants everywhere.",
			nil, nil,
		},
		{
			"HTML paragraph boundary",
			"<p>We are hiring in Canada</p><p>Benefits</p>",
			[]string{"ca"}, []string{"north_america"},
		},
		{
			"HTML list boundary",
			"<li>Candidates based in Canada</li><li>5 years Go</li>",
			[]string{"ca"}, []string{"north_america"},
		},
		{
			"HTML inline emphasis",
			"<p>We are hiring in <strong>Canada</strong>.</p>",
			[]string{"ca"}, []string{"north_america"},
		},
		{
			"dotted US abbreviation",
			"We are hiring in the U.S. only.",
			[]string{"us"}, []string{"north_america"},
		},
		{
			"dotted US in list",
			"Candidates based in the U.S. or Canada.",
			[]string{"ca", "us"}, []string{"north_america"},
		},
		{
			"dotted USA abbreviation",
			"We are hiring in the U.S.A. only.",
			[]string{"us"}, []string{"north_america"},
		},
		{
			"dotted UK abbreviation",
			"Candidates based in the U.K. only.",
			[]string{"gb"}, []string{"uk"},
		},
		{
			"unresolved anchor before hiring restriction",
			"Access to production data is restricted to the security team. We are hiring in Canada.",
			[]string{"ca"}, []string{"north_america"},
		},
		{
			"unresolved repeated anchor",
			"We are hiring in our growing team. We are hiring in Canada.",
			[]string{"ca"}, []string{"north_america"},
		},
		{
			"first resolving clause wins",
			"Candidates based in Canada. We are hiring in the US.",
			[]string{"ca"}, []string{"north_america"},
		},
		{
			"and joined countries",
			"We are hiring in Canada and the US.",
			[]string{"ca", "us"}, []string{"north_america"},
		},
		{
			"ampersand joined countries",
			"We are hiring in Canada & the US.",
			[]string{"ca", "us"}, []string{"north_america"},
		},
		{
			"and within country name",
			"Candidates based in Bosnia and Herzegovina.",
			[]string{"ba"}, []string{"eu"},
		},
		{
			"empty description",
			"",
			nil, nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCountries, gotRegions := RegionScopeFromDescription(tt.desc)
			if !slices.Equal(gotCountries, tt.wantCountries) {
				t.Errorf("countries = %v, want %v", gotCountries, tt.wantCountries)
			}
			if !slices.Equal(gotRegions, tt.wantRegions) {
				t.Errorf("regions = %v, want %v", gotRegions, tt.wantRegions)
			}
		})
	}
}
