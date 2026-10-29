package sources

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	"golang.org/x/net/html"
)

// optiusFake serves the real fixtures (fetched 2026-10-29 from www.optius.com): the listing's
// first page trimmed to four cards — JP LPT's Projektant (the site's own template), Krka's
// pharmaceutical worker (Krka's own template), Kariera's forklift driver (an embedded
// third-party page) and Gloria's Austrian posting — the cardless page the site answers past a
// listing's end, the abroad slice's first page trimmed to the Austrian card, and the posting
// pages of the first three. Any other posting answers 404, as a gone one does.
type optiusFake struct {
	// pageErr fails the main listing's second page.
	pageErr error
	// challenge answers the main listing's second page with a 200 that is not a listing.
	challenge bool
	// gone answers 404 for this posting id.
	gone string

	mu   sync.Mutex
	urls []string
}

const (
	opProjektant = "965281"
	opKrka       = "959691"
	opKariera    = "963987"
	opAustria    = "964949"
)

func (f *optiusFake) GetHTML(_ context.Context, u string) (*html.Node, error) {
	f.mu.Lock()
	f.urls = append(f.urls, u)
	f.mu.Unlock()
	var path string
	switch u {
	case optiusBaseURL + optiusListing + "?s=0":
		path = "listing_0.html"
	case optiusBaseURL + optiusListing + "?s=20":
		if f.pageErr != nil {
			return nil, f.pageErr
		}
		path = "listing_end.html"
		if f.challenge {
			path = "posting_" + opProjektant + ".html"
		}
	case optiusBaseURL + optiusAbroadListing + "?s=0":
		path = "abroad_0.html"
	case optiusBaseURL + optiusAbroadListing + "?s=20":
		path = "abroad_end.html"
	default:
		for _, id := range []string{opProjektant, opKrka, opKariera} {
			if strings.HasSuffix(u, "-"+id+"-"+id+"/") && id != f.gone {
				path = "posting_" + id + ".html"
			}
		}
	}
	if path == "" {
		return nil, &StatusError{Method: "GET", Code: 404, URL: u}
	}
	raw, err := os.ReadFile("testdata/optius/" + path)
	if err != nil {
		return nil, err
	}
	return html.Parse(strings.NewReader(string(raw)))
}

func (f *optiusFake) postingRequests() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, u := range f.urls {
		if !strings.Contains(u, "?s=") {
			n++
		}
	}
	return n
}

func newTestOptius(f *optiusFake) optius {
	s := NewOptius(f).(optius)
	s.limiter = noWait{}
	return s
}

var opEntry = CompanyEntry{Provider: "optius"}

func optiusByID(jobs []Job) map[string]Job {
	m := map[string]Job{}
	for _, j := range jobs {
		m[j.ExternalID] = j
	}
	return m
}

func TestOptiusHydratesNewPostingsAndRefreshesSeenOnes(t *testing.T) {
	f := &optiusFake{}
	jobs, err := newTestOptius(f).FetchNew(context.Background(), opEntry, func(id string) bool { return id == opAustria })
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 4 {
		t.Fatalf("got %d jobs, want three hydrated and one refreshed: %+v", len(jobs), jobs)
	}
	if n := f.postingRequests(); n != 3 {
		t.Errorf("posting requests = %d, want 3 (the seen posting costs none)", n)
	}
	by := optiusByID(jobs)

	p := by[opProjektant]
	if p.Title != "Projektant (m/ž)" || p.Company != "Javno podjetje Ljubljanska parkirišča in tržnice, d.o.o." || p.Location != "Ljubljana" {
		t.Errorf("title/company/place = %q / %q / %q", p.Title, p.Company, p.Location)
	}
	if p.URL != "https://www.optius.com/iskalci/prosta-delovna-mesta/projektant-mz-965281-965281/" {
		t.Errorf("url = %q", p.URL)
	}
	for _, want := range []string{"na podlagi odlokov", "Opis delovnega mesta", "cestnoprometne signalizacije", "Pričakujemo", "VII. stopnja izobrazbe", "Ponujamo", "Delovni čas"} {
		if !strings.Contains(p.Description, want) {
			t.Errorf("description lacks %q: %.300q", want, p.Description)
		}
	}
	for _, leak := range []string{"Shrani oglas", "Posreduj", "Pogosta vprašanja", "Prijava na delovno mesto"} {
		if strings.Contains(p.Description, leak) {
			t.Errorf("page chrome %q leaked into the description", leak)
		}
	}
	if p.EmploymentType != "full_time" || p.ExperienceYearsMin == nil || *p.ExperienceYearsMin != 1 {
		t.Errorf("employment / experience = %q / %v", p.EmploymentType, p.ExperienceYearsMin)
	}
	if p.PostedAt == nil || p.PostedAt.Format("2006-01-02") != "2026-10-29" {
		t.Errorf("posted = %v", p.PostedAt)
	}
	if len(p.FallbackCountries) != 1 || p.FallbackCountries[0] != "si" || p.Countries != nil {
		t.Errorf("countries / fallback = %v / %v, want the Slovenian fallback only", p.Countries, p.FallbackCountries)
	}

	k := by[opKrka]
	if k.Company != "KRKA, d.d., Novo mesto" || !strings.Contains(k.Description, "Vaš prispevek") || !strings.Contains(k.Description, "poklicno ali srednješolsko izobrazbo") {
		t.Errorf("Krka's own template: company %q, description %.300q", k.Company, k.Description)
	}
	if k.ExperienceYearsMin != nil {
		t.Errorf("Krka experience = %d, want none for an empty picklist", *k.ExperienceYearsMin)
	}

	// The page embeds the agency's own site; the body is the JobPosting's text.
	ka := by[opKariera]
	if !strings.Contains(ka.Description, "Na Karieri") || !strings.Contains(ka.Description, "Upravljanje viličarja") {
		t.Errorf("embedded posting's description = %.300q", ka.Description)
	}
	if ka.ExperienceYearsMin != nil {
		t.Errorf("a 0 - 1 year band states no minimum, got %d", *ka.ExperienceYearsMin)
	}

	a := by[opAustria]
	if !a.SeenRefresh || a.Description != "" {
		t.Errorf("the seen posting must be a body-less refresh: %+v", a)
	}
	if a.FallbackCountries != nil || a.Location != "Avstrija, Lienz z okolico" {
		t.Errorf("an abroad posting must carry no Slovenian fallback: %v / %q", a.FallbackCountries, a.Location)
	}
}

func TestOptiusFailsTheCrawlOnAListingPageFailure(t *testing.T) {
	f := &optiusFake{pageErr: errors.New("connection reset")}
	if _, err := newTestOptius(f).FetchNew(context.Background(), opEntry, func(string) bool { return false }); err == nil {
		t.Fatal("a listing page failing must fail the crawl: the sweep would close what it never reached")
	}
	if n := f.postingRequests(); n != 0 {
		t.Errorf("posting requests = %d, want none before the listing is whole", n)
	}
}

func TestOptiusFailsTheCrawlOnAPageThatIsNotAListing(t *testing.T) {
	f := &optiusFake{challenge: true}
	if _, err := newTestOptius(f).FetchNew(context.Background(), opEntry, func(string) bool { return false }); err == nil {
		t.Fatal("a cardless page without the result counter is no answer, not the listing's end")
	}
}

func TestOptiusBoundsThePostingPagesOneRunReads(t *testing.T) {
	f := &optiusFake{}
	s := newTestOptius(f)
	s.maxNewPerRun = 1
	jobs, err := s.FetchNew(context.Background(), opEntry, func(string) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	if n := f.postingRequests(); n != 1 {
		t.Errorf("posting requests = %d, want 1", n)
	}
	// Newest first: the listing's first card is the one read.
	if len(jobs) != 1 || jobs[0].ExternalID != opProjektant {
		t.Errorf("jobs = %+v, want the newest posting only", jobs)
	}
}

func TestOptiusDropsAGonePosting(t *testing.T) {
	f := &optiusFake{gone: opKrka}
	jobs, err := newTestOptius(f).FetchNew(context.Background(), opEntry, func(id string) bool { return id == opAustria })
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := optiusByID(jobs)[opKrka]; ok || len(jobs) != 3 {
		t.Errorf("a 404 posting must not be yielded: %d jobs", len(jobs))
	}
}

func TestOptiusSkipsTheDetailOfAGatedPosting(t *testing.T) {
	f := &optiusFake{}
	gate := DetailGate{Covered: func(companies []string) map[string]bool {
		return map[string]bool{"KRKA, d.d., Novo mesto": true}
	}}
	jobs, err := newTestOptius(f).FetchNewGated(context.Background(), opEntry, func(id string) bool { return id == opAustria }, gate)
	if err != nil {
		t.Fatal(err)
	}
	if k := optiusByID(jobs)[opKrka]; k.ExternalID == "" || k.Description != "" {
		t.Errorf("a covered posting is yielded body-less: %+v", k)
	}
	if n := f.postingRequests(); n != 2 {
		t.Errorf("posting requests = %d, want 2", n)
	}
}

func TestOptiusPlace(t *testing.T) {
	for in, want := range map[string]string{
		"Ljubljana":                       "Ljubljana",
		"Podravska - MB , Slovenija":      "Podravska, Slovenija",
		"Gorenjska - KR , Slovenija":      "Gorenjska, Slovenija",
		"Osrednjeslovenska - LJ":          "Osrednjeslovenska",
		"Notranjsko - kraška - PO":        "Notranjsko - kraška",
		"Avstrija - 8330 Feldbach":        "Avstrija - 8330 Feldbach",
		"  Grm - Trebnje,  Trebnje  ":     "Grm - Trebnje, Trebnje",
		"Hoče / Tujina":                   "Hoče / Tujina",
		"Zgornji Brnik, Brnik - Aerodrom": "Zgornji Brnik, Brnik - Aerodrom",
	} {
		if got := optiusPlace(in); got != want {
			t.Errorf("optiusPlace(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestOptiusPicklists(t *testing.T) {
	for in, want := range map[string]string{"polni delovni čas": "full_time", "skrajšani delovni čas": "part_time", "": ""} {
		if got := optiusEmploymentType(in); got != want {
			t.Errorf("optiusEmploymentType(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]int{"1 - 3 let": 1, "4 - 5 let": 4, "6 - 9 let": 6, "0 - 1 leto": 0, "": 0} {
		got, ok := optiusYears(in)
		if got != want || ok != (want > 0) {
			t.Errorf("optiusYears(%q) = %d, %v; want %d", in, got, ok, want)
		}
	}
}
