package companyname

import (
	"context"
	"encoding/json"
	"testing"
)

// fakeHTTP serves canned bodies keyed by URL for both GetText and GetJSON, since the
// placeholder registry mixes a JSON resolver (ukgready) with title scrapes.
type fakeHTTP map[string]string

func (f fakeHTTP) GetText(_ context.Context, url string) (string, error) { return f[url], nil }

func (f fakeHTTP) GetJSON(_ context.Context, url string, v any) error {
	body, ok := f[url]
	if !ok {
		return nil
	}
	return json.Unmarshal([]byte(body), v)
}

// Live-verified 2026-10-23: 15 of 15 sampled placeholder boards titled their listing page
// "{Name} - Job Opportunities".
func TestPlaceholderPaylocityUsesTheListingTitle(t *testing.T) {
	getter := fakeHTTP{
		"https://recruiting.paylocity.com/recruiting/jobs/All/7b2002bf-ee66-4c07-8735-9e64ef7c2ffc": `<html><head><title>Blue Plate Restaurant Company Inc - Job Opportunities</title></head></html>`,
	}
	reg := NewPlaceholderRegistry(getter)
	got, err := reg["paylocity"].Name(context.Background(), "7b2002bf-ee66-4c07-8735-9e64ef7c2ffc")
	if err != nil || got != "Blue Plate Restaurant Company Inc" {
		t.Errorf("Name = %q, %v; want Blue Plate Restaurant Company Inc", got, err)
	}
}

// Live-verified 2026-10-23: 15 of 15 sampled placeholder boards carried the tenant's legal
// name in format-settings' company.name, beside the short_name that IS the board's tenant id.
func TestPlaceholderUKGReadyUsesFormatSettings(t *testing.T) {
	getter := fakeHTTP{
		"https://secure.entertimeonline.com/ta/rest/ui/bootstrap/companies/%7C1553aps/format-settings": `{"company":{"short_name":"1553APS","name":"Copper Moon Cafes LLC"}}`,
	}
	reg := NewPlaceholderRegistry(getter)
	got, err := reg["ukgready"].Name(context.Background(), "secure.entertimeonline.com/1553aps")
	if err != nil || got != "Copper Moon Cafes LLC" {
		t.Errorf("Name = %q, %v; want Copper Moon Cafes LLC", got, err)
	}
	if got, err := reg["ukgready"].Name(context.Background(), "no-tenant"); err != nil || got != "" {
		t.Errorf("Name(no-tenant) = %q, %v; want empty, nil", got, err)
	}
}

func TestPlaceholderOracleUsesTheSiteName(t *testing.T) {
	getter := fakeHTTP{
		// og:site_name, not <title>: Maggiano's page JS-escapes the apostrophe in its title.
		"https://fa-etjg-saasfaprod1.fa.ocs.oraclecloud.com/hcmUI/CandidateExperience/en/sites/Maggianos/requisitions": `<head><meta property="og:site_name" content="Maggiano&#39;s"><title>Maggiano\'s</title></head>`,
	}
	reg := NewPlaceholderRegistry(getter)
	got, err := reg["oracle"].Name(context.Background(), "fa-etjg-saasfaprod1.fa.ocs.oraclecloud.com/Maggianos")
	if err != nil || got != "Maggiano's" {
		t.Errorf("Name = %q, %v; want Maggiano's", got, err)
	}
}

// The site names below are the live 2026-10-23 sample: an Oracle site is named by whoever
// configured it, so most carry the employer and a careers tail, and some carry only what the
// site is for.
func TestExtractOracleSiteName(t *testing.T) {
	cases := map[string]string{
		"Mouser":                        "Mouser",
		"Akamai Career Site":            "Akamai",
		"DNV careers":                   "DNV",
		"Good Samaritan Careers":        "Good Samaritan",
		"Harmonic Inc.":                 "Harmonic Inc.",
		"University of Tulsa":           "University of Tulsa",
		"Student Casual":                "",
		"Career Site  - New Applicants": "",
		"Dallah Recruitment System":     "",
		"Lazard Student":                "",
		"POLK_BoCC_Career Site":         "",
		"Candidate Experience site":     "",
		"":                              "",
		// The noise in the 613 names the first prod dry-run resolved (2026-10-24).
		"Working at Signature Aviation":           "Signature Aviation",
		"Job Listings at Liquidity Services Inc.": "Liquidity Services Inc.",
		"EXL Talent Acquisition Team":             "EXL",
		"Mycronic Job Openings":                   "Mycronic",
		"Crawford Job Listings - Global":          "Crawford",
		"UT Health San Antonio Staff Positions":   "UT Health San Antonio",
		"IRENA Employment":                        "IRENA",
		"Yanfeng Human Resources":                 "Yanfeng",
		"BDO USA Campus":                          "BDO USA",
		"Schroders Referral":                      "Schroders",
		"NemoursCareerSite":                       "Nemours",
		"OMNES Education - Carrière":              "OMNES Education",
		"City of Greeley -":                       "City of Greeley",
		"UConn Health - Community Network":        "UConn Health - Community Network",
		"WorkSafeNB":                              "WorkSafeNB",
		"Vylla Home":                              "Vylla Home",
		"Work with us":                            "",
		"Job Search":                              "",
		"Talent Acquisition Team":                 "",
		"Graduate and Apprenticeship":             "",
		"Sitio de carrera Laureate":               "",
		"Home Page":                               "",
		"Global":                                  "",
		"Schools":                                 "",
		"Generic":                                 "",
		// A "Career" inside the name is part of it: refused by the purpose word, never cut to
		// a different employer ("Summit").
		"Summit Career College": "",
	}
	for in, want := range cases {
		if got := ExtractOracleSiteName(in); got != want {
			t.Errorf("ExtractOracleSiteName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAcceptPlaceholder(t *testing.T) {
	cases := []struct {
		board, candidate, want string
		ok                     bool
	}{
		{"secure.entertimeonline.com/1553aps", "Copper Moon Cafes LLC", "Copper Moon Cafes LLC", true},
		{"x/y", "  Sava&#8217;s ", "Sava’s", true},
		{"x/y", "", "", false},
		{"x/y", "X/Y", "", false},            // the placeholder itself is no improvement
		{"x/y", "Page Not Found", "", false}, // junk
	}
	for _, c := range cases {
		got, ok := AcceptPlaceholder(c.board, c.candidate)
		if got != c.want || ok != c.ok {
			t.Errorf("AcceptPlaceholder(%q, %q) = %q, %v; want %q, %v", c.board, c.candidate, got, ok, c.want, c.ok)
		}
	}
}
