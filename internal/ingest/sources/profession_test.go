package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	"golang.org/x/net/html"
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
	// The board itself is the platform's own IT filing, so a posting from it asserts
	// confirmed-technical even though its category is left to a dictionary that resolves
	// only about half of them.
	if !hybrid.IsTechHint {
		t.Errorf("IsTechHint = false, want true (posting is filed under the platform's own IT board)")
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
	if !sited.IsTechHint {
		t.Errorf("sited IsTechHint = false, want true (posting is filed under the platform's own IT board)")
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

// professionGeneralBoard is a category the platform publishes that is not one of its two
// IT sections. The fixture's sitemap index names it, so it resolves the same way itdev
// does and only the filtering differs.
const professionGeneralBoard = "education"

// professionRecorder records every posting page requested. A prefilter is measured on the
// requests it did NOT make, and nothing else in the fixture can see that: a detail fetch
// for an unrouted URL fails, and a failed detail fetch is indistinguishable from the
// taken-down posting the adapter is supposed to skip quietly.
type professionRecorder struct {
	*routedHTTP
	mu    sync.Mutex
	pages []string
	xml   []string
}

func (r *professionRecorder) GetHTML(ctx context.Context, url string) (*html.Node, error) {
	r.mu.Lock()
	r.pages = append(r.pages, url)
	r.mu.Unlock()
	return r.routedHTTP.GetHTML(ctx, url)
}

func (r *professionRecorder) GetXML(ctx context.Context, url string, v any) error {
	r.mu.Lock()
	r.xml = append(r.xml, url)
	r.mu.Unlock()
	return r.routedHTTP.GetXML(ctx, url, v)
}

func professionGeneralEntry() CompanyEntry {
	return CompanyEntry{
		Company:  "Profession.hu — education",
		Provider: "profession",
		Board:    professionGeneralBoard,
	}
}

// TestProfessionRefusesAGeneralCategory pins that a board outside the platform's two IT
// sections is refused outright, before a request is made.
//
// It was briefly crawled behind a two-gate title filter, and a live audit of all 21 general
// categories on 2026-09-06 is why it is not any more: 11,403 postings listed, 227 passed
// the cheap gate, 159 were stored — and of those, 45 resolved through the bare English
// "analyst" (VAT, credit control, valuation), 23 through project management, 13 through
// business analysis, 11 through product and 13 through design/creative. Only 17 satisfied
// classify.IsTech. A gas fitter and a pump tester were among them. See AGENTS.md.
func TestProfessionRefusesAGeneralCategory(t *testing.T) {
	fixture := &professionRecorder{routedHTTP: (&routedHTTP{}).
		route("sitemap-listings-index-hu.xml", professionSitemapIndexXML).
		route("sitemap-listings-education-hu.xml", professionCategorySitemapXML(
			"https://www.profession.hu/allas/rendszergazda-acme-kft-gyor-2990001"))}

	_, err := NewProfession(fixture).Fetch(context.Background(), professionGeneralEntry())
	if err == nil {
		t.Fatal("a general category was crawled; it must be refused")
	}
	if !strings.Contains(err.Error(), professionGeneralBoard) {
		t.Errorf("the error does not name the board: %v", err)
	}
	// Refused before any request: which categories the catalogue takes is a fact about
	// the catalogue, not something the platform has to be asked about.
	if len(fixture.xml) != 0 || len(fixture.pages) != 0 {
		t.Errorf("requests were made for a refused board: xml=%v pages=%v", fixture.xml, fixture.pages)
	}
}

func TestProfessionITBoardIsUnfiltered(t *testing.T) {
	routed := (&routedHTTP{}).
		route("sitemap-listings-index-hu.xml", professionSitemapIndexXML).
		route("sitemap-listings-itdev-hu.xml", professionCategorySitemapXML(
			"https://www.profession.hu/allas/rendszergazda-acme-kft-gyor-2990001",
			"https://www.profession.hu/allas/senior-engineer-acme-kft-2989900",
		)).
		route("-2990001", professionPostingHTML(professionPostingOpts{
			title: "Rendszergazda", company: "ACME Kft.", addressLine: "Győr",
			sections: [][3]string{{"tasks", "Feladatok", "<ul><li>Linux</li></ul>"}},
		})).
		route("-2989900", professionPostingHTML(professionPostingOpts{
			title: "Senior Engineer", company: "ACME Kft.", addressLine: "Budapest",
			sections: [][3]string{{"tasks", "Feladatok", "<ul><li>Rendszerek tervezése</li></ul>"}},
		}))
	jobs, err := NewProfession(routed).Fetch(context.Background(), professionEntry())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("Fetch returned %d jobs, want both: %+v", len(jobs), jobs)
	}
}

// TestProfessionITBoardNames pins the enumeration cmd/backfill-profession-it-tech reads
// to build its search predicate — sorted, so the predicate is deterministic.
func TestProfessionITBoardNames(t *testing.T) {
	got := ProfessionITBoardNames()
	want := []string{"itdev", "itops"}
	if !slices.Equal(got, want) {
		t.Errorf("ProfessionITBoardNames() = %v, want %v", got, want)
	}
}

// TestProfessionConfirmsTech pins the predicate cmd/backfill-derive uses to recover the
// ingest-time IsTechHint from a stored row that cannot re-crawl to learn its board.
func TestProfessionConfirmsTech(t *testing.T) {
	tests := []struct {
		name, source, externalID string
		want                     bool
	}{
		{"profession itdev board", "profession", "itdev:123", true},
		{"profession itops board", "profession", "itops:456", true},
		{"case-insensitive board match", "profession", "ItDev:123", true},
		{"profession general-population board", "profession", "hr:123", false},
		{"another source sharing the board-name prefix", "greenhouse", "itdev:123", false},
		{"no namespace separator at all", "profession", "123", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ProfessionConfirmsTech(tt.source, tt.externalID); got != tt.want {
				t.Errorf("ProfessionConfirmsTech(%q, %q) = %v, want %v", tt.source, tt.externalID, got, tt.want)
			}
		})
	}
}
