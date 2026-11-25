package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/platform/db"
)

func row(slug, provider, board, name string, open int32) db.ListOpenJobsByCompanyBoardRow {
	return db.ListOpenJobsByCompanyBoardRow{CompanySlug: slug, Provider: provider, Board: board, BoardCompany: name, OpenJobs: open}
}

// The benchmark's cases, the chains that must stay quiet, and enough ordinary boards for each
// provider to count as one whose ids name employers.
func fixture() ([]db.ListOpenJobsByCompanyBoardRow, []company) {
	rows := []db.ListOpenJobsByCompanyBoardRow{
		row("xebia", "greenhouse", "xebiausa", "Xebia", 8),
		row("xebia-dach", "greenhouse", "xebiadach", "Xebia DACH", 4),
		row("poland-and-eastern-europe", "greenhouse", "xebiacee", "Poland and Eastern Europe", 83),
		row("exa-labs", "ashby", "exa", "Exa Labs", 73),
		row("exa", "inhire", "exa", "EXA", 5),
		row("exa", "recruitee", "exa", "Exa", 3),
		row("gomotive", "greenhouse", "gomotive", "GoMotive", 197),
		row("motive", "greenhouse", "motive", "Motive", 3),
		row("motive", "polymer", "motive", "Motive", 6),
		row("stripe", "greenhouse", "stripe", "Stripe", 50),
		row("acme", "ashby", "acme", "Acme", 2),
		// amazon states its own site; another company stating amazon.com does not make the
		// amazon board someone else's.
		row("amazon", "personio", "amazon", "Amazon", 4),
		// A chain: one name over many boards of one provider and slug, ids opaque or branded.
		row("chick-fil-a", "workstream", "004a7767", "Chick-fil-A", 3),
		row("chick-fil-a", "workstream", "00c2cf6e", "Chick-fil-A", 3),
		row("discovery-senior-living", "hiringthing", "allara-senior-living.oasisrecruit.com", "Discovery Senior Living", 2),
		row("discovery-senior-living", "hiringthing", "aston-gardens.oasisrecruit.com", "Discovery Senior Living", 2),
		row("discovery-senior-living", "hiringthing", "aston-pointe.oasisrecruit.com", "Discovery Senior Living", 2),
		row("discovery-senior-living", "hiringthing", "discovery.oasisrecruit.com", "Discovery Senior Living", 2),
	}
	// A job-search slice: one board name over many employers is a label, not evidence.
	for i := range labelSlugs + 1 {
		rows = append(rows, row(fmt.Sprintf("employer-%d", i), "arbeitsagentur", "Informatik", "Bundesagentur für Arbeit", 1))
	}
	companies := []company{
		{Slug: "exa", Name: "Exa", Website: "https://exa.ai"},
		{Slug: "motive", Name: "Motive", Website: "https://gomotive.com"},
		{Slug: "employer-0", Name: "Employer", Website: "employer.de"},
		{Slug: "stripe", Name: "Stripe", Website: "stripe.com"},
		{Slug: "amazon", Name: "Amazon", Website: "aboutamazon.com"},
		{Slug: "amazon-web-services", Name: "AWS", Website: "amazon.com"},
	}
	return rows, companies
}

func reasons(found []finding) map[string]string {
	out := map[string]string{}
	for _, f := range found {
		k := f.CompanySlug + " " + f.Provider + "/" + f.Board
		out[k] = strings.TrimPrefix(out[k]+","+f.Reason+":"+f.Confidence, ",")
	}
	return out
}

func TestSuspiciousFindsTheBenchmarkCollisions(t *testing.T) {
	got := reasons(suspicious(fixture()))
	want := map[string]string{
		// Xebia's regional board named after a region lands on a company of that name.
		"poland-and-eastern-europe greenhouse/xebiacee": "name_not_board_id:high",
		// Exa's own board sits on exa-labs while exa (exa.ai) carries the namesakes.
		"exa-labs ashby/exa": "token_names_other_company:high",
		// Motive's own board sits on gomotive, the label of the site motive states.
		"gomotive greenhouse/gomotive": "token_names_other_company:high",
		// motive's boards are the namesakes; only a "go" prefix separates them from the site.
		"motive greenhouse/motive": "website_mismatch:low",
		"motive polymer/motive":    "website_mismatch:low",
	}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("%s = %q, want %q", k, got[k], w)
		}
	}
	for k, r := range got {
		if _, ok := want[k]; !ok {
			t.Errorf("unexpected finding %s: %s", k, r)
		}
	}
}

func TestTokenCoreSkipsOpaqueIDs(t *testing.T) {
	for board, want := range map[string]string{
		"xebialatam":                           "xebialatam",
		"ensign.wd1.myworkdayjobs.com/arvada":  "ensign",
		"jackson-hewitt-1674":                  "jacksonhewitt",
		"004a7767":                             "",
		"40b4b8db-3e63-43fb-85d2-c178b2fffb87": "",
		"recruiting2.ultipro.com/cul1004culfs": "",
	} {
		if got, _ := tokenCore(board); got != want {
			t.Errorf("tokenCore(%q) = %q, want %q", board, got, want)
		}
	}
}

func TestWebsiteLabel(t *testing.T) {
	for site, want := range map[string]string{
		"https://exa.ai": "exa", "www.acme.co.uk/jobs": "acme", "https://about.google": "google", "careers.gomotive.com": "gomotive", "": "",
	} {
		if got := websiteLabel(site); got != want {
			t.Errorf("websiteLabel(%q) = %q, want %q", site, got, want)
		}
	}
}

func TestWriteCSVHeader(t *testing.T) {
	var buf bytes.Buffer
	if err := writeCSV(&buf, []finding{{CompanySlug: "a", Board: "b,c", OpenJobs: 2}}); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if lines[0] != "company_slug,company_name,company_website,provider,board_token,board_company_name,open_jobs,reason,suggested_action,confidence" ||
		!strings.Contains(lines[1], `"b,c"`) {
		t.Errorf("csv = %q", buf.String())
	}
}
