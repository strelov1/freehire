package companyname

import (
	"context"
	"testing"
)

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
