package location

import (
	"slices"
	"strings"
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
			"department access is not a geographic restriction",
			"Access to employee records is restricted to HR and IT.",
			nil, nil,
		},
		{
			"department access does not hide hiring geography",
			"Access to employee records is restricted to HR and IT. We are hiring in Canada.",
			[]string{"ca"}, []string{"north_america"},
		},
		{
			"role restriction with country codes",
			"This role is restricted to HR and IT.",
			[]string{"hr", "it"}, []string{"eu"},
		},
		{
			"candidate restriction with country names",
			"Candidates are restricted to Canada and the US.",
			[]string{"ca", "us"}, []string{"north_america"},
		},
		{
			"role restriction with country names",
			"This role is restricted to Canada and the US.",
			[]string{"ca", "us"}, []string{"north_america"},
		},
		{
			"compound subdivision after conjunction",
			"Candidates based in Canada and Tyne and Wear.",
			[]string{"ca", "gb"}, []string{"north_america", "uk"},
		},
		{
			"compound country before conjunction",
			"Candidates based in Bosnia and Herzegovina and Canada.",
			[]string{"ba", "ca"}, []string{"eu", "north_america"},
		},
		{
			"compound country after conjunction",
			"Candidates based in Canada and Bosnia and Herzegovina.",
			[]string{"ba", "ca"}, []string{"eu", "north_america"},
		},
		{
			"restriction after long plain text",
			strings.Repeat("x", 50_000) + ". We are hiring in Canada.",
			[]string{"ca"}, []string{"north_america"},
		},
		{
			"restriction after long HTML",
			"<p>" + strings.Repeat("x", 50_000) + "</p><p>We are hiring in <strong>Canada</strong>.</p>",
			[]string{"ca"}, []string{"north_america"},
		},
		{
			"restriction across Unicode truncation boundary",
			"<p>" + strings.Repeat("界", 49_990) + "</p><p>We are hiring in Canada.</p>",
			[]string{"ca"}, []string{"north_america"},
		},
		{
			"plain text newline ends hiring clause",
			"We are hiring in Canada\nBenefits include paid leave.",
			[]string{"ca"}, []string{"north_america"},
		},
		{
			"plain text comparison preserves newline boundary",
			"We are hiring in Canada\nBenefits include < 5 office visits a year.",
			[]string{"ca"}, []string{"north_america"},
		},
		{
			"HTML emphasis inside restriction anchor",
			"<p>Candidates <strong>based in</strong> Canada.</p>",
			[]string{"ca"}, []string{"north_america"},
		},
		{
			"blockquote does not wrap restriction anchor",
			"<blockquote><p>" + strings.Repeat("x", 54) + " Candidates <strong>based in</strong> Canada.</p></blockquote>",
			[]string{"ca"}, []string{"north_america"},
		},
		{
			"HTML wrapped and list",
			"<p>We are hiring in Canada\nand the US.</p>",
			[]string{"ca", "us"}, []string{"north_america"},
		},
		{
			"HTML CRLF wrapped list",
			"<p>We are hiring in Canada\r\n\tand\tthe US.</p>",
			[]string{"ca", "us"}, []string{"north_america"},
		},
		{
			"plain wrapped or list",
			"We are hiring in Canada\n or the US.",
			[]string{"ca", "us"}, []string{"north_america"},
		},
		{
			"CRLF and tab wrapped list",
			"We are hiring in Canada\r\n\tand\tthe US.",
			[]string{"ca", "us"}, []string{"north_america"},
		},
		{
			"wrapped ampersand list",
			"We are hiring in Canada\n & the US.",
			[]string{"ca", "us"}, []string{"north_america"},
		},
		{
			"blank line ends restriction before connector",
			"We are hiring in Canada\n\nand the US.",
			[]string{"ca"}, []string{"north_america"},
		},
		{
			"separate HTML paragraphs end restriction before connector",
			"<p>We are hiring in Canada</p><p>and the US.</p>",
			[]string{"ca"}, []string{"north_america"},
		},
		{
			"connector must be a whole word",
			"We are hiring in Canada\nAndroid teams work in the US.",
			[]string{"ca"}, []string{"north_america"},
		},
		{
			"nested HTML block ends restriction before connector",
			"<div>We are hiring in Canada<div>and the US.</div></div>",
			[]string{"ca"}, []string{"north_america"},
		},
		{
			"whitespace only blank line ends restriction",
			"We are hiring in Canada\r\n\t\r\nand the US.",
			[]string{"ca"}, []string{"north_america"},
		},
		{
			"uppercase wrapped connector",
			"We are hiring in Canada\nAND the US.",
			[]string{"ca", "us"}, []string{"north_america"},
		},
		{
			"plain wrap after and",
			"We are hiring in Canada and\nthe US.",
			[]string{"ca", "us"}, []string{"north_america"},
		},
		{
			"HTML wrap after and",
			"<p>We are hiring in Canada and\nthe US.</p>",
			[]string{"ca", "us"}, []string{"north_america"},
		},
		{
			"plain wrap after comma",
			"We are hiring in Canada,\nthe US.",
			[]string{"ca", "us"}, []string{"north_america"},
		},
		{
			"HTML wrap after comma",
			"<p>We are hiring in Canada,\nthe US.</p>",
			[]string{"ca", "us"}, []string{"north_america"},
		},
		{
			"plain wrap after or",
			"We are hiring in Canada or\nthe US.",
			[]string{"ca", "us"}, []string{"north_america"},
		},
		{
			"plain wrap after ampersand",
			"We are hiring in Canada &\nthe US.",
			[]string{"ca", "us"}, []string{"north_america"},
		},
		{
			"blank line after comma ends restriction",
			"We are hiring in Canada,\n\nthe US.",
			[]string{"ca"}, []string{"north_america"},
		},
		{
			"HTML block after comma ends restriction",
			"<p>We are hiring in Canada,\n<div>the US.</div></p>",
			[]string{"ca"}, []string{"north_america"},
		},
		{
			"inline emphasis after source newline",
			"<p>We are hiring in Canada\n<strong>and</strong> the US.</p>",
			[]string{"ca", "us"}, []string{"north_america"},
		},
		{
			"inline emphasis before source newline",
			"<p>We are hiring in Canada <strong>and</strong>\nthe US.</p>",
			[]string{"ca", "us"}, []string{"north_america"},
		},
		{
			"inline connector after paragraph boundary",
			"<p>We are hiring in Canada</p>\n<p><strong>and</strong> the US.</p>",
			[]string{"ca"}, []string{"north_america"},
		},
		{
			"inline connector after nested block boundary",
			"<div>We are hiring in Canada\n<div><strong>and</strong> the US.</div></div>",
			[]string{"ca"}, []string{"north_america"},
		},
		{
			"standalone list connector",
			"We are hiring in Canada\nand\nthe US.",
			[]string{"ca", "us"}, []string{"north_america"},
		},
		{
			"standalone inline list connector",
			"<p>We are hiring in Canada\n<strong>and</strong>\nthe US.</p>",
			[]string{"ca", "us"}, []string{"north_america"},
		},
		{
			"blank line before standalone connector ends restriction",
			"We are hiring in Canada\n\nand\nthe US.",
			[]string{"ca"}, []string{"north_america"},
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
