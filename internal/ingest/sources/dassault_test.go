package sources

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/MoeForge/moego/regexp"
)

// dassaultFake serves canned Exalead answers keyed by the request's start offset.
type dassaultFake struct {
	pages map[int]string
	fail  map[int]bool
	urls  []string
}

var dassaultStartRE = regexp.MustCompile(`start=(\d+)`)

func (f *dassaultFake) GetXML(_ context.Context, u string, v any) error {
	f.urls = append(f.urls, u)
	start, _ := strconv.Atoi(dassaultStartRE.FindStringSubmatch(u)[1])
	if f.fail[start] {
		return errors.New("dassaultFake: boom")
	}
	raw, ok := f.pages[start]
	if !ok {
		raw = `<Answer nhits="0" estimated="false"><hits></hits></Answer>`
	}
	return xml.Unmarshal([]byte(raw), v)
}

func dassaultFixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// dassaultSyntheticPage builds a page of n minimal cards with ids from first, stating total.
func dassaultSyntheticPage(first, n, total int) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<Answer nhits="%d" estimated="false"><hits>`, total)
	meta := func(name, value string) string {
		return fmt.Sprintf(`<Meta name="%s"><MetaString name="value">%s</MetaString></Meta>`, name, value)
	}
	for id := first; id < first+n; id++ {
		b.WriteString("<Hit><metas>")
		b.WriteString(meta("card_id", strconv.Itoa(id)))
		b.WriteString(meta("content_title", "Job "+strconv.Itoa(id)))
		b.WriteString(meta("content_summary", "&lt;p&gt;Body&lt;/p&gt;"))
		b.WriteString(meta("content_cta_1_url", fmt.Sprintf("https://www.3ds.com/careers/jobs/job-%d", id)))
		b.WriteString("</metas></Hit>")
	}
	b.WriteString("</hits></Answer>")
	return b.String()
}

func TestDassaultProviderAndMarkers(t *testing.T) {
	s := NewDassault(nil)
	if s.Provider() != "dassault" {
		t.Errorf("Provider() = %q", s.Provider())
	}
	if _, ok := s.(boardless); !ok {
		t.Error("dassault should implement the boardless marker")
	}
	if !FullBoardListingProviders(All(nil))["dassault"] {
		t.Error("FullBoardListingProviders(All(nil)) should include dassault")
	}
}

// Real answer (the Poland slice of the English career cards, 2026-10-15): two internships,
// postings with and without an experience level.
func TestDassaultFetchMapsRealResponse(t *testing.T) {
	f := &dassaultFake{pages: map[int]string{0: dassaultFixture(t, "dassault_search_poland.xml")}}
	jobs, err := NewDassault(f).Fetch(context.Background(), CompanyEntry{Company: "Dassault Systèmes"})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 5 {
		t.Fatalf("got %d jobs, want 5", len(jobs))
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	intern := byID["550372"]
	if !strings.HasPrefix(intern.URL, "https://www.3ds.com/careers/jobs/") || !strings.HasSuffix(intern.URL, "-550372") {
		t.Errorf("URL = %q", intern.URL)
	}
	if intern.Company != "Dassault Systèmes" || intern.Title == "" {
		t.Errorf("company/title = %q / %q", intern.Company, intern.Title)
	}
	if intern.EmploymentType != "internship" || intern.Seniority != "intern" {
		t.Errorf("internship = %q/%q", intern.EmploymentType, intern.Seniority)
	}
	if intern.Location != "Krakow, Poland" || len(intern.Countries) != 1 || intern.Countries[0] != "pl" {
		t.Errorf("location = %q countries %v", intern.Location, intern.Countries)
	}
	if intern.ExperienceYearsMin == nil || *intern.ExperienceYearsMin != 0 {
		t.Errorf("0 to 3 years must read as a minimum of 0, got %v", intern.ExperienceYearsMin)
	}
	if intern.PostedAt == nil || intern.PostedAt.Year() != 2026 {
		t.Errorf("PostedAt = %v", intern.PostedAt)
	}
	// The card's summary is the posting's whole body, served entity-escaped inside the XML.
	if !strings.Contains(intern.Description, "<p>") || strings.Contains(intern.Description, "&lt;") || len(intern.Description) < 1000 {
		t.Errorf("description = %.200s", intern.Description)
	}

	if ns := byID["550461"]; ns.ExperienceYearsMin != nil || ns.EmploymentType != "" || ns.Location != "Warsaw, Poland" {
		t.Errorf("Not specified / Regular = years %v type %q location %q", ns.ExperienceYearsMin, ns.EmploymentType, ns.Location)
	}
	if four := byID["548679"]; four.ExperienceYearsMin == nil || *four.ExperienceYearsMin != 4 {
		t.Errorf("4 to 5 years = %v", four.ExperienceYearsMin)
	}
}

func TestDassaultFetchRealNetherlands(t *testing.T) {
	f := &dassaultFake{pages: map[int]string{0: dassaultFixture(t, "dassault_search_netherlands.xml")}}
	jobs, err := NewDassault(f).Fetch(context.Background(), CompanyEntry{Company: "Dassault Systèmes"})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 4 {
		t.Fatalf("got %d jobs, want 4", len(jobs))
	}
	for _, j := range jobs {
		if j.Location != "'s-Hertogenbosch, Netherlands" {
			t.Errorf("%s location = %q", j.ExternalID, j.Location)
		}
	}
}

func TestDassaultRequestCarriesBothRefinements(t *testing.T) {
	f := &dassaultFake{pages: map[int]string{0: dassaultSyntheticPage(1, 3, 3)}}
	if _, err := NewDassault(f).Fetch(context.Background(), CompanyEntry{Company: "Dassault Systèmes"}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"r=f/card_content_type/career", "cards%20language/en", "hf=100"} {
		if !strings.Contains(f.urls[0], want) {
			t.Errorf("request %s lacks %s", f.urls[0], want)
		}
	}
}

func TestDassaultFetchPagesToTheStatedTotal(t *testing.T) {
	f := &dassaultFake{pages: map[int]string{
		0:   dassaultSyntheticPage(1, 100, 102),
		100: dassaultSyntheticPage(101, 2, 102),
	}}
	jobs, err := NewDassault(f).Fetch(context.Background(), CompanyEntry{Company: "Dassault Systèmes"})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 102 || len(f.urls) != 2 {
		t.Errorf("got %d jobs in %d requests, want 102 in 2", len(jobs), len(f.urls))
	}
}

func TestDassaultFetchFailsWhenNotWhole(t *testing.T) {
	cases := map[string]*dassaultFake{
		"first page fails": {fail: map[int]bool{0: true}},
		"later page fails": {pages: map[int]string{0: dassaultSyntheticPage(1, 100, 150)}, fail: map[int]bool{100: true}},
		"count short of total": {pages: map[int]string{
			0:   dassaultSyntheticPage(1, 100, 103),
			100: dassaultSyntheticPage(100, 3, 103),
		}},
	}
	for name, f := range cases {
		t.Run(name, func(t *testing.T) {
			if jobs, err := NewDassault(f).Fetch(context.Background(), CompanyEntry{Company: "Dassault Systèmes"}); err == nil {
				t.Errorf("want an error, got %d jobs", len(jobs))
			}
		})
	}
}

func TestDassaultLocation(t *testing.T) {
	for _, c := range []struct{ city, country, want string }{
		{"India, MH, Pune", "India", "Pune, India"},
		{"Korea, Republic of, 11, Seoul", "Korea", "Seoul, Korea"},
		{"France, Vélizy-Villacoublay", "France", "Vélizy-Villacoublay, France"},
		{"France", "France", "France"},
	} {
		if got := dassaultLocation(c.city, c.country); got != c.want {
			t.Errorf("dassaultLocation(%q, %q) = %q, want %q", c.city, c.country, got, c.want)
		}
	}
}

func TestDassaultCountry(t *testing.T) {
	for raw, want := range map[string]string{
		"India":              "India",
		"Korea, Republic of": "Korea",
		"Taiwan,China":       "Taiwan",
		"United States":      "United States",
	} {
		if got := dassaultCountry(raw); got != want {
			t.Errorf("dassaultCountry(%q) = %q, want %q", raw, got, want)
		}
	}
}
