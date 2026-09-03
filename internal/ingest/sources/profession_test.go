package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
)

// professionTestBoard is one of the two categories sources/profession.yml crawls.
const professionTestBoard = "itdev"

// professionSitemapIndexXML renders the platform's sitemap index. It carries a second,
// non-IT category so a needle that matched loosely would pick the wrong one.
const professionSitemapIndexXML = `<?xml version="1.0" encoding="UTF-8"?>
<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
<sitemap><loc>https://www.profession.hu/sitemap-listings-education-hu.xml</loc></sitemap>
<sitemap><loc>https://www.profession.hu/sitemap-listings-itdev-hu.xml</loc></sitemap>
<sitemap><loc>https://www.profession.hu/sitemap-listings-itops-hu.xml</loc></sitemap>
</sitemapindex>`

// professionCategorySitemapXML renders one category's sitemap around the given posting URLs.
func professionCategorySitemapXML(locs ...string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
	for _, loc := range locs {
		fmt.Fprintf(&b, `<url><loc>%s</loc><lastmod>2026-09-02T17:12:04+02:00</lastmod></url>`, loc)
	}
	b.WriteString(`</urlset>`)
	return b.String()
}

// professionPostingOpts describes one posting page as the fixtures vary it. The zero value is
// the common case: a sited posting stating its country in jobLocation.
type professionPostingOpts struct {
	title, company string
	// addressLine is the page's address line verbatim, badge and separator included.
	addressLine string
	// telecommute renders the posting the way the platform renders one that is not
	// strictly onsite: the TELECOMMUTE flag, no jobLocation, and the country named in
	// applicantLocationRequirements instead.
	telecommute            bool
	locationRequirement    string
	employmentType         string
	experienceRequirements string
	educationRequirements  string
	// sections are the body sections the page renders, in order, as (id, heading, inner HTML).
	sections [][3]string
}

// professionPostingHTML renders a posting page the way profession.hu does: a schema.org
// JobPosting inside an @graph alongside the site's own Organization and WebPage nodes — which
// is the shape LDJobPosting has to see through — plus the rendered body sections and the
// address line the structured block does not carry.
func professionPostingHTML(o professionPostingOpts) string {
	posting := map[string]any{
		"@type":                  "JobPosting",
		"title":                  o.title,
		"name":                   o.title,
		"datePosted":             "2026-09-02",
		"validThrough":           "2026-10-03T17:04:20",
		"employmentType":         o.employmentType,
		"experienceRequirements": o.experienceRequirements,
		"educationRequirements":  o.educationRequirements,
		"occupationalCategory":   "IT programozás, Fejlesztés - Programozó, Fejlesztő",
		// The block's own description: the same text as the sections below with its list
		// markup flattened, which is why the adapter must not read it.
		"description":        "<p>Feladatok:</p>Microservice-ek fejlesztése RESTful API-k tervezése",
		"hiringOrganization": map[string]any{"@type": "Organization", "name": o.company},
	}
	if o.telecommute {
		posting["jobLocationType"] = "TELECOMMUTE"
		posting["applicantLocationRequirements"] = []any{
			map[string]any{"@type": "Country", "name": o.locationRequirement},
		}
	} else {
		posting["jobLocation"] = map[string]any{
			"@type": "Place",
			"address": map[string]any{
				"@type": "PostalAddress", "addressLocality": "Budapest", "addressCountry": "HU",
			},
		}
	}
	graph, err := json.Marshal(map[string]any{
		"@context": "https://schema.org",
		"@graph": []any{
			map[string]any{"@type": "Organization", "name": "Profession.hu"},
			map[string]any{"@type": "BreadcrumbList"},
			posting,
		},
	})
	if err != nil {
		panic(err)
	}
	var body strings.Builder
	for _, s := range o.sections {
		fmt.Fprintf(&body, `<div id="box_%s" class="box-container">`+
			`<h2 class="advertisement-content-title"><span class="title"> %s </span></h2>`+
			`<div id="%s" itemprop="%s">%s</div></div>`, s[0], s[1], s[0], s[0], s[2])
	}
	return `<html><head><script type="application/ld+json">` + string(graph) + `</script></head><body>` +
		`<div class="my-auto font-size-16 address-data">` + o.addressLine + `</div>` +
		body.String() + `</body></html>`
}

// professionGonePageHTML is what a taken-down posting's URL answers: a 200 for the category
// listing it redirects to, carrying no JobPosting block at all.
const professionGonePageHTML = `<html><head><title>Programozó, Fejlesztő állás | Profession.hu</title>
<script type="application/ld+json">{"@context":"https://schema.org","@graph":[{"@type":"WebSite"}]}</script>
</head><body><h1>491 db állás</h1></body></html>`

// professionHybridPosting is the fixture's ordinary posting: hybrid, which the ld+json spends
// its single TELECOMMUTE flag on and which therefore carries no jobLocation, so both the work
// mode and the city exist only on the address line.
func professionHybridPosting() string {
	return professionPostingHTML(professionPostingOpts{
		title: " Backend fejlesztő ", company: "TRENDENCY ONLINE Zrt.",
		addressLine:         `Hibrid <span class="location-separator">•</span> <span itemprop="jobLocation"><span itemprop="addressLocality"> 1092 Budapest, Knézits utca </span></span>`,
		telecommute:         true,
		locationRequirement: "Magyarország",
		employmentType:      "Teljes munkaidő, Alkalmazotti jogviszony",
		// The picklist's own spelling; the adapter reads the low end of the band.
		experienceRequirements: "3-5 év tapasztalat",
		// English at B2 and a college degree — the platform's two commonest answers.
		educationRequirements: "Angol középfok, Főiskola",
		sections: [][3]string{
			{"tasks", "Feladatok", "<ul><li>Microservice-ek fejlesztése</li></ul>"},
			{"requirements", "Elvárások", "<ul><li>Minimum 3 éves NodeJs tapasztalat</li></ul>"},
			{"offer", "Amit kínálunk", "<ul><li>Szakmai tréningek</li></ul>"},
			// Neither of these is part of the posting: the first is the employer's
			// standing blurb, the second is where its contact details land.
			{"c_info", "Céginformáció", "<p>A Trendency 180 fős csapata.</p>"},
			{"contact_info", "Jelentkezés", "<p>allas@trendency.hu</p>"},
		},
	})
}

// professionFixture wires the sitemap index, the itdev category sitemap, two live postings
// and one that has been taken down.
func professionFixture() *routedHTTP {
	return (&routedHTTP{}).
		route("sitemap-listings-index-hu.xml", professionSitemapIndexXML).
		route("sitemap-listings-itdev-hu.xml", professionCategorySitemapXML(
			"https://www.profession.hu/allas/backend-fejleszto-trendency-online-zrt-budapest-2990578",
			"https://www.profession.hu/allas/rendszergazda-acme-kft-gyor-2990001",
			"https://www.profession.hu/allas/senior-engineer-gone-kft-2989827",
		)).
		route("-2990578", professionHybridPosting()).
		route("-2990001", professionPostingHTML(professionPostingOpts{
			title: "Rendszergazda", company: "ACME Kft.",
			addressLine:            "9024 Győr, Práter utca 9.",
			employmentType:         "Alkalmazotti jogviszony",
			experienceRequirements: "Nem kell tapasztalat",
			sections:               [][3]string{{"tasks", "Feladatok", "<ul><li>Linux szerverek üzemeltetése</li></ul>"}},
		})).
		route("-2989827", professionGonePageHTML)
}

func professionEntry() CompanyEntry {
	return CompanyEntry{
		Company:  "Profession.hu — IT development",
		Provider: "profession",
		Board:    professionTestBoard,
	}
}

func TestProfessionProvider(t *testing.T) {
	if got := NewProfession(nil).Provider(); got != "profession" {
		t.Errorf("Provider() = %q, want %q", got, "profession")
	}
}

func TestProfessionRegisteredAndFacet(t *testing.T) {
	if _, ok := All(nil)["profession"]; !ok {
		t.Fatal("profession not registered in sources.All")
	}
	// The board selects a category of one central catalogue, not a tenant, and every
	// posting names its own employer.
	if !slices.Contains(BoardKeyedProviders(Taxonomy()), "profession") {
		t.Error("profession should be board-keyed")
	}
	if !slices.Contains(AggregatorProviders(Taxonomy()), "profession") {
		t.Error("profession should be an aggregator")
	}
	if !slices.Contains(FilterableProviders(), "profession") {
		t.Error("profession should appear in the source facet")
	}
}

func TestProfessionFetch(t *testing.T) {
	jobs, err := NewProfession(professionFixture()).Fetch(context.Background(), professionEntry())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	// The taken-down posting answers 200 with no JobPosting, so it is skipped rather than
	// stored body-less or counted as a board failure.
	if len(jobs) != 2 {
		t.Fatalf("Fetch returned %d jobs, want 2: %+v", len(jobs), jobs)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	hybrid, ok := byID["2990578"]
	if !ok {
		t.Fatalf("no job keyed on the platform's posting id; got %v", byID)
	}
	if hybrid.Title != "Backend fejlesztő" || hybrid.Company != "TRENDENCY ONLINE Zrt." {
		t.Errorf("identity = %q / %q", hybrid.Title, hybrid.Company)
	}
	// TELECOMMUTE covers hybrid too, so the badge is what decides — and Remote must not
	// follow it.
	if hybrid.WorkMode != "hybrid" || hybrid.Remote {
		t.Errorf("work mode = %q remote=%v, want hybrid/false", hybrid.WorkMode, hybrid.Remote)
	}
	// The postal code and street number would stop the location dictionary, and the badge
	// is not a place.
	if hybrid.Location != "Budapest, Knézits utca" {
		t.Errorf("location = %q", hybrid.Location)
	}
	// The country is stated only in applicantLocationRequirements on such a posting, and
	// only in Hungarian.
	if !slices.Equal(hybrid.Countries, []string{"hu"}) {
		t.Errorf("countries = %v, want [hu]", hybrid.Countries)
	}
	if hybrid.EmploymentType != "full_time" {
		t.Errorf("employment type = %q, want full_time", hybrid.EmploymentType)
	}
	if hybrid.ExperienceYearsMin == nil || *hybrid.ExperienceYearsMin != 3 {
		t.Errorf("experience = %v, want 3", hybrid.ExperienceYearsMin)
	}
	// The platform states both as a picklist, so neither is left to the English-prose
	// matchers — which would find nothing at all in a Hungarian body.
	if hybrid.EducationLevel != "bachelor" || hybrid.EnglishLevel != "b2" {
		t.Errorf("education/english = %q/%q, want bachelor/b2", hybrid.EducationLevel, hybrid.EnglishLevel)
	}
	if hybrid.PostedAt == nil || hybrid.PostedAt.Format("2006-01-02") != "2026-09-02" {
		t.Errorf("posted at = %v", hybrid.PostedAt)
	}
	// The category is left to the dictionary: the platform's own taxonomy is what picked
	// the board and is too coarse to name the role.
	if hybrid.Category != "" {
		t.Errorf("category = %q, want empty", hybrid.Category)
	}
	if hybrid.SalaryMin != nil || hybrid.SalaryCurrency != "" {
		t.Errorf("salary set from a slice that publishes none: %v %q", hybrid.SalaryMin, hybrid.SalaryCurrency)
	}

	// The body is the page's sections under the employer's own headings, with the list
	// markup the ld+json copy has lost — and without the two sections that are not the
	// posting.
	for _, want := range []string{"Feladatok", "<li>", "Microservice-ek fejlesztése", "Amit kínálunk"} {
		if !strings.Contains(hybrid.Description, want) {
			t.Errorf("description is missing %q: %s", want, hybrid.Description)
		}
	}
	for _, unwanted := range []string{"180 fős csapata", "allas@trendency.hu"} {
		if strings.Contains(hybrid.Description, unwanted) {
			t.Errorf("description carries %q, which is not the posting: %s", unwanted, hybrid.Description)
		}
	}

	sited := byID["2990001"]
	// No badge means the platform is stating an ordinary sited posting.
	if sited.WorkMode != "onsite" || sited.Remote {
		t.Errorf("sited work mode = %q remote=%v", sited.WorkMode, sited.Remote)
	}
	if sited.Location != "Győr, Práter utca ." {
		t.Errorf("sited location = %q", sited.Location)
	}
	if !slices.Equal(sited.Countries, []string{"hu"}) {
		t.Errorf("sited countries = %v, want [hu]", sited.Countries)
	}
	// "Alkalmazotti jogviszony" is employee status, not a schedule, so it names no type.
	if sited.EmploymentType != "" {
		t.Errorf("sited employment type = %q, want empty", sited.EmploymentType)
	}
	if sited.ExperienceYearsMin == nil || *sited.ExperienceYearsMin != 0 {
		t.Errorf("sited experience = %v, want 0", sited.ExperienceYearsMin)
	}
}

// TestProfessionFetchNewSkipsSeen pins the hydration saving: a posting the catalogue already
// has costs no detail request, and yields a liveness refresh rather than a content-less write.
func TestProfessionFetchNewSkipsSeen(t *testing.T) {
	http := professionFixture()
	jobs, err := NewProfession(http).(HydratingSource).
		FetchNew(context.Background(), professionEntry(), func(id string) bool { return id == "2990578" })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	var refreshed, hydrated int
	for _, j := range jobs {
		if j.SeenRefresh {
			refreshed++
			if j.ExternalID != "2990578" || j.Description != "" {
				t.Errorf("refresh should carry identity only: %+v", j)
			}
			continue
		}
		hydrated++
	}
	if refreshed != 1 || hydrated != 1 {
		t.Fatalf("refreshed=%d hydrated=%d, want 1/1", refreshed, hydrated)
	}
	// Four requests, not five: the sitemap index, the category sitemap, and a posting page
	// for the new posting and the taken-down one. The seen posting's page is what the
	// refresh saves.
	if http.calls != 4 {
		t.Errorf("requests = %d, want 4", http.calls)
	}
}

// TestProfessionUnknownCategoryFails pins the board error a mistyped category gets. An
// unknown board must not read as an empty one — nothing downstream could tell them apart,
// and an empty crawl closes the category's postings on the unseen sweep.
func TestProfessionUnknownCategoryFails(t *testing.T) {
	e := professionEntry()
	e.Board = "itdevv"
	_, err := NewProfession(professionFixture()).Fetch(context.Background(), e)
	if err == nil {
		t.Fatal("a category the sitemap index does not name must fail the board")
	}
	if !strings.Contains(err.Error(), "itdevv") {
		t.Errorf("error should name the board: %v", err)
	}
}

func TestProfessionExternalID(t *testing.T) {
	cases := map[string]string{
		"https://www.profession.hu/allas/backend-fejleszto-trendency-online-zrt-budapest-2990578": "2990578",
		// The platform links its own canonical variant with a trailing segment.
		"https://www.profession.hu/allas/backend-fejleszto-trendency-2990578/optimum": "2990578",
		// Not a posting: the sitemap filter is what this answers for.
		"https://www.profession.hu/allasok/programozo-fejleszto/1,10,0,0,75": "",
		"https://www.profession.hu/cegek":                                    "",
	}
	for loc, want := range cases {
		if got := professionExternalID(loc); got != want {
			t.Errorf("professionExternalID(%q) = %q, want %q", loc, got, want)
		}
	}
}

// TestProfessionPlace pins the two things the address line states and the ld+json does not:
// which of the platform's three work-mode badges the posting carries, and a place the
// location dictionary can read.
func TestProfessionPlace(t *testing.T) {
	cases := []struct {
		name, line, wantMode, wantPlace string
	}{
		{"sited address", "1087 Budapest, Kerepesi út 21.", "onsite", "Budapest, Kerepesi út ."},
		{"hybrid with a place", "Hibrid • 9024 Győr, Práter utca 9.", "hybrid", "Győr, Práter utca ."},
		{"hybrid with no place", "Hibrid", "hybrid", ""},
		{"remote", "Távmunka / Remote", "remote", ""},
		// "Nationwide coverage" is where the work is, not how it is done, and it names no
		// place either.
		{"nationwide", "Országos lefedettség", "", ""},
		{"bare city", "Budaörs", "onsite", "Budaörs"},
		{"nothing stated", "", "onsite", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mode, place := professionPlace(tc.line)
			if mode != tc.wantMode || place != tc.wantPlace {
				t.Errorf("professionPlace(%q) = %q/%q, want %q/%q",
					tc.line, mode, place, tc.wantMode, tc.wantPlace)
			}
		})
	}
}

// TestProfessionEmploymentType pins the picklist's two halves apart: it states a contract
// form and a schedule in one comma-joined field, and only the schedule is an employment type.
func TestProfessionEmploymentType(t *testing.T) {
	cases := map[string]string{
		"":                                     "",
		"Alkalmazotti jogviszony":              "",
		"Vállalkozói, Alkalmazotti jogviszony": "",
		"Teljes munkaidő, Alkalmazotti jogviszony": "full_time",
		"Részmunkaidő, Alkalmazotti jogviszony":    "part_time",
		"Diákmunka": "internship",
		// Both schedules ticked names neither.
		"Teljes munkaidő, Részmunkaidő": "",
	}
	for field, want := range cases {
		if got := professionEmploymentType(field); got != want {
			t.Errorf("professionEmploymentType(%q) = %q, want %q", field, got, want)
		}
	}
}

func TestProfessionExperienceYears(t *testing.T) {
	cases := map[string]*int{
		"Nem kell tapasztalat":      intPtr(0),
		"Pályakezdő/friss diplomás": intPtr(0),
		"1-3 év tapasztalat":        intPtr(1),
		"3-5 év tapasztalat":        intPtr(3),
		"5-10 év tapasztalat":       intPtr(5),
		">10 év tapasztalat":        intPtr(10),
		"":                          nil,
		"Some new band":             nil,
	}
	for level, want := range cases {
		got := professionExperienceYears(level)
		switch {
		case want == nil && got != nil:
			t.Errorf("professionExperienceYears(%q) = %d, want nil", level, *got)
		case want != nil && (got == nil || *got != *want):
			t.Errorf("professionExperienceYears(%q) = %v, want %d", level, got, *want)
		}
	}
}

// TestProfessionEducationAndLanguage pins the platform's educationRequirements picklist,
// which states two different things in one comma-joined field: zero or more LANGUAGE
// levels and exactly one SCHOOL level. The whole closed vocabulary — 28 tokens — was read
// off all 709 live postings, so a value outside it is a change on the platform's side and
// must yield nothing rather than a guess.
func TestProfessionEducationAndLanguage(t *testing.T) {
	cases := []struct {
		name, field, wantEdu, wantEng string
	}{
		// The two commonest values, together 46% of the slice.
		{"intermediate english, secondary school", "Angol középfok, Középiskola", "none", "b2"},
		{"intermediate english, college", "Angol középfok, Főiskola", "bachelor", "b2"},
		// Hungary's exam levels are NOT "basic/medium/high": alapfok/középfok/felsőfok are
		// the state-recognised levels and map to B1/B2/C1 (Gov. Decree 137/2008). The trio
		// has no name for C2, which is why felsőfok is so often mistranslated as one.
		{"basic english is B1, not A-something", "Angol alapfok, Középiskola", "none", "b1"},
		{"advanced english is C1, not C2", "Angol felsőfok, Egyetem", "master", "c1"},
		{"native english", "Angol anyanyelvi szint, Egyetem", "master", "native"},
		// The platform's own way of saying no language is required.
		{"no language needed", "Nem kell nyelvtudás, Középiskola", "none", "none"},
		// A posting may require several languages; only English answers this facet, and a
		// posting naming other languages says nothing about English.
		{"english alongside german", "Angol középfok, Német középfok, Középiskola", "none", "b2"},
		{"german only states no english level", "Német felsőfok, Egyetem", "master", ""},
		{"hungarian native alongside english", "Angol felsőfok, Magyar anyanyelvi szint, Egyetem", "master", "c1"},
		// School levels below a degree are the platform stating that no degree is needed.
		{"primary school", "Nem kell nyelvtudás, Általános iskola", "none", "none"},
		{"vocational school", "Angol alapfok, Szakiskola / szakmunkás képző", "none", "b1"},
		// Felsőoktatási szakképzés is a two-year tertiary programme, below a bachelor's and
		// above secondary — freehire's four-value vocabulary has no member for it, so it
		// yields nothing rather than rounding to a neighbour.
		{"higher vocational maps to neither", "Angol középfok, Felsőoktatási szakképzés", "", "b2"},
		{"empty field", "", "", ""},
		{"a value the platform has not published before", "Angol mesterfok, Doktori", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			edu, eng := professionEducationAndLanguage(tc.field)
			if edu != tc.wantEdu || eng != tc.wantEng {
				t.Errorf("professionEducationAndLanguage(%q) = %q/%q, want %q/%q",
					tc.field, edu, eng, tc.wantEdu, tc.wantEng)
			}
		})
	}
}
