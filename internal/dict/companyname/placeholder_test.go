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

type recordingFakeHTTP struct {
	fakeHTTP
	requests []string
}

func (f *recordingFakeHTTP) GetText(ctx context.Context, url string) (string, error) {
	f.requests = append(f.requests, url)
	return f.fakeHTTP.GetText(ctx, url)
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

// Every case is a footer the 2026-10-24 sweep of all 226 placeholder Taleo boards met.
func TestExtractTaleoCopyright(t *testing.T) {
	cases := map[string]string{
		"2025 TTEC. All rights reserved.":                                     "TTEC",
		"2018 by Hyatt Corporation":                                           "Hyatt Corporation",
		"2017 The Marcus Corporation. All Rights Reserved.":                   "The Marcus Corporation",
		"2025 D.R. Horton, Inc. All Right Reserved |":                         "D.R. Horton, Inc.",
		"Moxa Inc. All rights reserved.":                                      "Moxa Inc.",
		"2022 Mercedes-Benz Group AG. All Rights Reserved.":                   "Mercedes-Benz Group AG",
		"2017  Commonwealth of Massachusetts.":                                "Commonwealth of Massachusetts",
		"Copyright 2025, Cowan Systems LLC. All rights reserved.":             "Cowan Systems LLC",
		"Copyright 2013 IEEE – All rights reserved. Use of this Web site":     "IEEE",
		"2019 Navy Exchange Service Command all right reserved. Navy Exch":    "Navy Exchange Service Command",
		"2026 Tampa General Hospital | Affiliated with the USF Health":        "Tampa General Hospital",
		"2025 Children’s Hospitals and Clinics of Minnesota |":                "Children’s Hospitals and Clinics of Minnesota",
		"2018, Schneider Electric":                                            "Schneider Electric",
		"WSP 2023":                                                            "WSP",
		"2016 3D Systems, Inc. All rights reserved.":                          "3D Systems, Inc.",
		"City of St. Louis":                                                   "City of St. Louis",
		"Southern Methodist University, PO Box 750100, Dallas, TX 75275":      "Southern Methodist University",
		"2025 West Virginia University. West Virginia University is an Equal": "West Virginia University",
		"2020":                                       "",
		"2025 All rights reserved.":                  "",
		"2025 Oracle Corporation":                    "",
		"2025 Acme Inc. | 100 Main Street":           "Acme Inc.",
		"2016 | 8000 York Road | Towson, MD 21252 |": "",
		"": "",
	}
	for in, want := range cases {
		if got := ExtractTaleoCopyright(in); got != want {
			t.Errorf("ExtractTaleoCopyright(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestExtractTaleoLogoAlt(t *testing.T) {
	cases := map[string]string{
		"Marcus Careers":                        "Marcus",
		"Burns&amp;McDonnell logo":              "Burns&McDonnell",
		"The black version of the Stantec logo": "Stantec",
		"Hyatt Taleo":                           "Hyatt",
		"Careers at Bealls Outlet":              "Bealls Outlet",
		"CNH Jobs":                              "CNH",
		"Tenet Healthcare":                      "Tenet Healthcare",
		"Oracle Taleo":                          "",
		"PHP Staging Mobile Taleo":              "",
		"Job Search":                            "",
		"":                                      "",
	}
	for in, want := range cases {
		if got := ExtractTaleoLogoAlt(in); got != want {
			t.Errorf("ExtractTaleoLogoAlt(%q) = %q, want %q", in, got, want)
		}
	}
}

// The footer wins over the logo; a page with neither names nothing.
func TestPlaceholderTaleoPrefersTheFooter(t *testing.T) {
	const page = "https://hyatt.taleo.net/careersection/10780/jobsearch.ftl?lang=en"
	getter := fakeHTTP{
		page: `<img class="logo_img" alt="Hyatt Taleo" src="/x.png"><p>&#169; 2025 Oracle Corporation</p><div>&copy; 2018 by Hyatt Corporation</div>`,
		"https://hdr.taleo.net/careersection/hdr_mobile/jobsearch.ftl?lang=en": `<img class="logo_img" alt="Marcus Careers">`,
		"https://none.taleo.net/careersection/2/jobsearch.ftl?lang=en":         `<title>Job Search</title>`,
	}
	reg := NewPlaceholderRegistry(getter)
	for board, want := range map[string]string{
		"hyatt.taleo.net/10780":    "Hyatt Corporation",
		"hdr.taleo.net/hdr_mobile": "Marcus",
		"none.taleo.net/2":         "",
	} {
		if got, err := reg["taleo"].Name(context.Background(), board); err != nil || got != want {
			t.Errorf("Name(%s) = %q, %v; want %q", board, got, err, want)
		}
	}
}

// Live-verified 2026-10-28: the legacy /jobs page titles itself "¡Oportunidades de Empleo en
// {Name}!", the newer /portal (where /jobs redirects) "{Name} | Portal externo".
func TestPlaceholderHiringRoomReadsBothPortalTitles(t *testing.T) {
	getter := fakeHTTP{
		"https://agd.hiringroom.com/jobs": `<html><head><title>¡Oportunidades de Empleo en Aceitera General Deheza S.A.!</title></head></html>`,
		"https://x28.hiringroom.com/jobs": `<html><head><title>X-28 Alarmas | Portal externo</title></head></html>`,
	}
	reg := NewPlaceholderRegistry(getter)
	for board, want := range map[string]string{"agd": "Aceitera General Deheza S.A.", "x28": "X-28 Alarmas"} {
		got, err := reg["hiringroom"].Name(context.Background(), board)
		if err != nil || got != want {
			t.Errorf("Name(%s) = %q, %v; want %q", board, got, err, want)
		}
	}
}

func TestExtractHiringRoomTitleRefusesOtherPages(t *testing.T) {
	for _, title := range []string{"Error 404 - La página no existe", "", "HiringRoom"} {
		if got := ExtractHiringRoomTitle(title); got != "" {
			t.Errorf("ExtractHiringRoomTitle(%q) = %q, want none", title, got)
		}
	}
	if got := ExtractHiringRoomTitle("¡Oportunidades de Empleo en AFP HABITAT !"); got != "AFP HABITAT" {
		t.Errorf("trailing space before ! = %q", got)
	}
}

// Live-verified 2026-10-28: 20 of 20 sampled placeholder boards (company == the careers host)
// named the employer in the careers page's og:site_name, while its <title> is whatever the
// tenant typed ("Join our team and deliver real innovation...", "Jobs at Careers").
func TestPlaceholderZohoRecruitUsesTheSiteName(t *testing.T) {
	getter := fakeHTTP{
		"https://scrumconnect.zohorecruit.eu/jobs/Careers":       `<head><title>Join our team and deliver real innovation that improves people’s lives.</title><meta property="og:site_name" content="Scrumconnect Limited" /></head>`,
		"https://burtandassociates.zohorecruit.com/jobs/Careers": `<head><meta property="og:site_name" content="MB&amp;A" /></head>`,
		"https://nometa.zohorecruit.in/jobs/Careers":             `<head><title>Jobs at Careers</title></head>`,
	}
	reg := NewPlaceholderRegistry(getter)
	for board, want := range map[string]string{
		"scrumconnect.zohorecruit.eu":       "Scrumconnect Limited",
		"burtandassociates.zohorecruit.com": "MB&A",
		"nometa.zohorecruit.in":             "",
	} {
		got, err := reg["zohorecruit"].Name(context.Background(), board)
		if err != nil || got != want {
			t.Errorf("Name(%s) = %q, %v; want %q", board, got, err, want)
		}
	}
}

func TestHostOnlyBoardRequiresBareHost(t *testing.T) {
	for board, want := range map[string]bool{
		"jobs.bookbeat.com":         true,
		"":                          false,
		"jobs.example.com/path":     false,
		"jobs.example.com:8443":     false,
		"user@jobs.example.com":     false,
		"jobs.example.com?region=1": false,
		"jobs.example.com#fragment": false,
		"jobs..example.com":         false,
		"jobs_.example.com":         false,
		"-jobs.example.com":         false,
		"jobs.example-.com":         false,
		"jobs.example.com.":         false,
		"jobs":                      false,
		"127.0.0.1":                 false,
		"xn--bcher-kva.example":     true} {
		if got := hostOnlyBoard(board); got != want {
			t.Errorf("hostOnlyBoard(%q) = %v; want %v", board, got, want)
		}
	}
}

func TestPlaceholderResolversDoNotFetchNonHostBoards(t *testing.T) {
	for _, provider := range []string{"successfactors", "teamtailor"} {
		t.Run(provider, func(t *testing.T) {
			for _, board := range []string{"jobs.invalid/path", "bad_host.example", "127.0.0.1", "jobs..example.com"} {
				getter := &recordingFakeHTTP{}
				resolver := NewPlaceholderRegistry(getter)[provider]
				got, err := resolver.Name(context.Background(), board)
				if err != nil || got != "" {
					t.Errorf("Name(%q) = %q, %v; want empty, nil", board, got, err)
				}
				if len(getter.requests) != 0 {
					t.Errorf("non-host board %q triggered HTTP requests: %v", board, getter.requests)
				}
			}
		})
	}
}

// Live-verified 2026-11-02: the SuccessFactors host's title is "Jobs at Volvo Group".
func TestPlaceholderSuccessFactorsUsesCareerPageTitle(t *testing.T) {
	getter := fakeHTTP{
		"https://jobs.volvogroup.com/":  `<html><head><title>Jobs at Volvo Group</title></head></html>`,
		"https://jobs.generic.com/":     `<html><head><title>Job Search</title></head></html>`,
		"https://jobs.careers.example/": `<html><head><title>Jobs at Careers</title></head></html>`,
		"https://jobs.invalid/path/":    `<html><head><title>Jobs at Should Not Accept</title></head></html>`,
	}
	reg := NewPlaceholderRegistry(getter)
	resolver, ok := reg["successfactors"]
	if !ok {
		t.Fatal("placeholder registry has no SuccessFactors resolver")
	}
	for board, want := range map[string]string{
		"jobs.volvogroup.com":  "Volvo Group",
		"jobs.generic.com":     "",
		"jobs.careers.example": "",
		"jobs.invalid/path":    "",
	} {
		got, err := resolver.Name(context.Background(), board)
		if err != nil || got != want {
			t.Errorf("Name(%s) = %q, %v; want %q", board, got, err, want)
		}
	}
}

// Live-verified 2026-11-02: the Teamtailor jobs page's og:site_name is "BookBeat".
func TestPlaceholderTeamtailorUsesSiteName(t *testing.T) {
	getter := fakeHTTP{
		"https://jobs.bookbeat.com/jobs":     `<head><meta property="og:site_name" content="BookBeat" /></head>`,
		"https://jobs.no-meta.com/jobs":      `<head><title>Jobs at No Meta</title></head>`,
		"https://jobs.careers.example/jobs":  `<head><meta property="og:site_name" content="Careers" /></head>`,
		"https://jobs.platform.example/jobs": `<head><meta property="og:site_name" content="Teamtailor" /></head>`,
		"https://jobs.empty.example/jobs":    `<head><meta property="og:site_name" content="" /></head>`,
		"https://jobs.entity.example/jobs":   `<head><meta property="og:site_name" content="BookBeat &amp; Co" /></head>`,
		"https://jobs.spaces.example/jobs":   `<head><meta property="og:site_name" content=" Careers " /></head>`,
		"https://jobs.invalid/path/jobs":     `<head><meta property="og:site_name" content="Should Not Accept" /></head>`,
	}
	reg := NewPlaceholderRegistry(getter)
	resolver, ok := reg["teamtailor"]
	if !ok {
		t.Fatal("placeholder registry has no Teamtailor resolver")
	}
	for board, want := range map[string]string{
		"jobs.bookbeat.com":     "BookBeat",
		"jobs.no-meta.com":      "",
		"jobs.careers.example":  "",
		"jobs.platform.example": "",
		"jobs.empty.example":    "",
		"jobs.entity.example":   "BookBeat & Co",
		"jobs.spaces.example":   "",
		"jobs.invalid/path":     "",
	} {
		got, err := resolver.Name(context.Background(), board)
		if err != nil || got != want {
			t.Errorf("Name(%s) = %q, %v; want %q", board, got, err, want)
		}
	}
}

// Live-sampled 2026-11-13: 22 of 60 hex-labelled Paycor boards use the branded CareerHome
// template, whose <title> names the employer; the plain template has no title at all.
func TestPlaceholderPaycorUsesTheCareerHomeTitle(t *testing.T) {
	const board = "8a7883d074274b7d01744b8825240e7f"
	getter := fakeHTTP{
		"https://recruitingbypaycor.com/career/CareerHome.action?clientId=" + board: `<html><head><title>Careers | Salvagnini America, Inc.</title></head></html>`,
	}
	got, err := NewPlaceholderRegistry(getter)["paycor"].Name(context.Background(), board)
	if err != nil || got != "Salvagnini America, Inc." {
		t.Errorf("Name = %q, %v; want Salvagnini America, Inc.", got, err)
	}
}

func TestExtractPaycorTitle(t *testing.T) {
	for title, want := range map[string]string{
		"Careers | Paloma Resort Properties":               "Paloma Resort Properties",
		"CAREERS | HORAN Wealth, LLC":                      "HORAN Wealth, LLC",
		"Smith Dairy Queens, Ltd. | Careers":               "Smith Dairy Queens, Ltd.",
		"Current Opportunities | Craig Technologies":       "Craig Technologies",
		"The Dirty Buffalo Careers":                        "The Dirty Buffalo",
		"Girl Scouts of Greater Iowa":                      "Girl Scouts of Greater Iowa",
		"The Magic House, St. Louis Children&#39;s Museum": "The Magic House, St. Louis Children's Museum",
		"":                                    "",
		"Careers":                             "",
		"Join Our Team | Careers":             "",
		"Acme | Join the Best Team":           "", // two candidate parts: refuse rather than guess
		"Careers - Jelly Belly Candy Company": "Jelly Belly Candy Company",
		"Careers — RNDC-IL":                   "RNDC-IL",
		"Careers at Urban Chestnut Brewing Company":                        "Urban Chestnut Brewing Company",
		"Meeder Investment Management -":                                   "Meeder Investment Management",
		"Aero Propulsion Support, Inc. - Employment":                       "Aero Propulsion Support, Inc.",
		"Lutheran Care Center at Concord Village : Who We Are :":           "Lutheran Care Center at Concord Village",
		"Athelas Institute, Inc. : About Us : Employment Opportunities":    "Athelas Institute, Inc.",
		"Ohio Valley Residential Services – Balancing Mission, Management": "",
		"Mysite":                                "",
		"Add Company Name":                      "",
		"Jobs at Company Name":                  "",
		"Careers at Add Company Name":           "",
		"Careers at Company Name Careers":       "",
		"Company Name":                          "",
		"neighorhood":                           "",
		"GR Metrology Employment Opportunities": "GR Metrology",
		"The Little Gym Jobs":                   "The Little Gym",
		"Join Our Team at Ice Industries":       "Ice Industries",
	} {
		if got := ExtractPaycorTitle(title); got != want {
			t.Errorf("ExtractPaycorTitle(%q) = %q, want %q", title, got, want)
		}
	}
}
