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
		// The second prod pass (2026-10-24): 245 sites the first rule refused or never met.
		"Dollar General External Career Site": "Dollar General",
		"Mayo Career Site US":                 "Mayo",
		"FMOL Health Career Portal":           "FMOL Health",
		"JPMC Candidate Experience page":      "JPMC",
		"St. Croix County Career Section":     "St. Croix County",
		"Calderys Career Opportunities":       "Calderys",
		"Jobs @ Carmeuse":                     "Carmeuse",
		"Chubb External":                      "Chubb",
		"Mountaire Jobs":                      "Mountaire",
		"Careers | BXP":                       "BXP",
		"Verisk Careers | Verisk":             "Verisk",
		// Two parts naming different things are refused, never chosen between by shape.
		"Local Truck Driving Jobs | Centerline Drivers": "",
		"Careers at Acme | North America":               "",
		"Chubb External Jobs":                           "Chubb",
		"Corporate Careers | Trueblue":                  "Trueblue",
		"Work with us | Copa Airlines - NEW":            "Copa Airlines",
		"the Myriad Genetics Candidate site":            "Myriad Genetics",
		"The Kroger Co.":                                "The Kroger Co.",
		"Search Jobs - INL Interns":                     "INL",
		"Testing Site - JPMC":                           "",
		"All Jobs":                                      "",
		"Candidate Experience site - Asia":              "",
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

// Live-verified 2026-10-24: 13 of 15 sampled placeholder boards named the tenant in the
// careers page's Next.js data as candidateCorrespondenceClientName — the sender name on the
// mail a candidate receives. One named a department instead ("Human Resources"), one nothing.
func TestPlaceholderDayforceUsesTheCorrespondenceName(t *testing.T) {
	getter := fakeHTTP{
		"https://jobs.dayforcehcm.com/en-US/ymcadetroit/boll": `<script id="__NEXT_DATA__" type="application/json">{"props":{"pageProps":{"dehydratedState":{"queries":[{"state":{"data":{"clientNamespace":"ymcadetroit","candidateCorrespondenceClientName":"YMCA of Metropolitan Detroit"}}}]}}}}</script>`,
		"https://jobs.dayforcehcm.com/en-US/feg/site":         `{"candidateCorrespondenceClientName":"Foley \"FEG\" Group"}`,
	}
	reg := NewPlaceholderRegistry(getter)
	if got, err := reg["dayforce"].Name(context.Background(), "ymcadetroit/boll"); err != nil || got != "YMCA of Metropolitan Detroit" {
		t.Errorf("Name = %q, %v; want YMCA of Metropolitan Detroit", got, err)
	}
	if got, _ := reg["dayforce"].Name(context.Background(), "feg/site"); got != `Foley "FEG" Group` {
		t.Errorf("Name = %q; want the JSON string unescaped", got)
	}
}

func TestExtractDayforceClientName(t *testing.T) {
	cases := map[string]string{
		"YMCA of Metropolitan Detroit": "YMCA of Metropolitan Detroit",
		"NexTier ":                     "NexTier",
		"AERO-FLITE, INC.":             "AERO-FLITE, INC.",
		"Human Resources":              "",
		"HR":                           "",
		"Recruiting Team":              "",
		"Talent Acquisition":           "",
		"":                             "",
	}
	for in, want := range cases {
		if got := ExtractDayforceClientName(in); got != want {
			t.Errorf("ExtractDayforceClientName(%q) = %q, want %q", in, got, want)
		}
	}
}

// Live-verified 2026-10-24: 11 of 15 sampled placeholder boards carried the employer's brand
// as the alt text of the job board's header logo; the rest had no logo, or the platform's
// unconfigured "Default Brand".
func TestPlaceholderUKGUsesTheLogoAlt(t *testing.T) {
	getter := fakeHTTP{
		"https://recruiting2.ultipro.com/min1006/JobBoard/3ca60bcf-ef8c-d9d3-2f88-ace3de16072d/": `<img class="small-logo logo" src="/x" alt="Novares US Engine Components" data-automation="navbar-small-logo" />`,
	}
	reg := NewPlaceholderRegistry(getter)
	got, err := reg["ukg"].Name(context.Background(), "recruiting2.ultipro.com/min1006/3ca60bcf-ef8c-d9d3-2f88-ace3de16072d")
	if err != nil || got != "Novares US Engine Components" {
		t.Errorf("Name = %q, %v; want Novares US Engine Components", got, err)
	}
	if got, err := reg["ukg"].Name(context.Background(), "two/parts"); err != nil || got != "" {
		t.Errorf("Name(two/parts) = %q, %v; want empty, nil", got, err)
	}
}

func TestExtractUKGBrandName(t *testing.T) {
	cases := map[string]string{
		"Custom Glass &amp; Doors ": "Custom Glass & Doors",
		"PhyNet Brand":              "PhyNet",
		"Pure Salt Baytown, LLC":    "Pure Salt Baytown, LLC",
		"Default Brand":             "",
		"Brand":                     "",
		"":                          "",
	}
	for in, want := range cases {
		if got := ExtractUKGBrandName(in); got != want {
			t.Errorf("ExtractUKGBrandName(%q) = %q, want %q", in, got, want)
		}
	}
}
