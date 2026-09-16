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
