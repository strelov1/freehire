package sources

import (
	"context"
	"errors"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"

	"golang.org/x/net/html"
)

// posaoHRFake serves real pages fetched 2026-10-29 from www.posao.hr in a Croatian session,
// trimmed to the cards each test needs and with contact details redacted:
//
//   - county_a_1/county_a_2: Ličko-senjska, pages 1 and 2 of 4 — CGP's and ITEXIM's cards, then
//     Bent excellent's card followed by an employment-service (HZZ) card, where the walk stops.
//   - county_b_1: Sisačko-moslavačka page 1 (its pager edited to one page) — Pepco's card and
//     CGP's again.
//   - abroad_1: "abroad" (Inozemstvo), page 6's HSM card joined with page 1's ITEXIM card, which
//     is also filed under Croatian counties (pager edited to one page).
//   - posting pages: Bent excellent's form posting, Pepco's designed ad, HSM's form posting in
//     Germany, and an employment-service vacancy.
//
// Any other posting answers 404.
type posaoHRFake struct {
	// countyErr fails Ličko-senjska's second page.
	countyErr error
	// challenge answers Ličko-senjska's second page with a 200 that is not a listing.
	challenge bool
	// emptyPage answers Ličko-senjska's second page with the pager and no card.
	emptyPage bool
	// gone redirects this posting to the home page, as the site does for a taken-down one.
	gone string
	// republished answers this posting with the employment-service vacancy's page.
	republished string
	// english answers posting pages in English, as a session without the language switch is.
	english bool

	mu   sync.Mutex
	urls []string
}

const (
	phCGP    = "1253963" // Ličko-senjska and Sisačko-moslavačka
	phITEXIM = "1224550" // Ličko-senjska and abroad
	phBent   = "1253021"
	phPepco  = "1254723"
	phHSM    = "1246576" // abroad only
	phHZZ    = "1254594"
)

var phPostings = map[string]string{phBent: "posting_1253021.html", phPepco: "posting_1254723.html", phHSM: "posting_1246576.html"}

func (f *posaoHRFake) GetHTML(ctx context.Context, u string) (*html.Node, error) {
	n, _, err := f.GetHTMLResolved(ctx, u)
	return n, err
}

func (f *posaoHRFake) GetHTMLResolved(_ context.Context, u string) (*html.Node, string, error) {
	f.mu.Lock()
	f.urls = append(f.urls, u)
	f.mu.Unlock()
	county := posaoHRBaseURL + "/zupanije/"
	var name string
	switch u {
	case posaoHRLanguageURL:
		return &html.Node{Type: html.DocumentNode}, u, nil
	case county + "licko-senjska/":
		name = "county_a_1.html"
	case county + "licko-senjska/stranica/2":
		if f.countyErr != nil {
			return nil, "", f.countyErr
		}
		name = "county_a_2.html"
		if f.challenge {
			name = "posting_1253021.html"
		}
	case county + "sisacko-moslavacka/":
		name = "county_b_1.html"
	case county + "inozemstvo/":
		name = "abroad_1.html"
	default:
		for id, file := range phPostings {
			if !strings.HasSuffix(u, "/"+id+"/") {
				continue
			}
			switch id {
			case f.gone:
				return f.parse("county_b_1.html", posaoHRBaseURL+"/")
			case f.republished:
				name = "posting_hzz.html"
			default:
				name = file
			}
		}
	}
	if name == "" {
		return nil, "", &StatusError{Method: "GET", Code: 404, URL: u}
	}
	return f.parse(name, u)
}

func (f *posaoHRFake) parse(name, final string) (*html.Node, string, error) {
	raw, err := os.ReadFile("testdata/posaohr/" + name)
	if err != nil {
		return nil, "", err
	}
	page := string(raw)
	if f.emptyPage && name == "county_a_2.html" {
		start := strings.Index(page, `<a href="https://www.posao.hr/oglasi/`)
		end := strings.Index(page, `<div class="pagination"`)
		page = page[:start] + page[end:]
	}
	if f.english && strings.HasPrefix(name, "posting_") {
		page = strings.Replace(page, `lang="hr"`, `lang="en"`, 1)
	}
	n, err := html.Parse(strings.NewReader(page))
	return n, final, err
}

func (f *posaoHRFake) postingRequests() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, u := range f.urls {
		if strings.Contains(u, "/oglasi/") {
			out = append(out, u)
		}
	}
	return out
}

func newTestPosaoHR(f *posaoHRFake) posaoHR {
	s := NewPosaoHR(f).(posaoHR)
	s.limiter = noWait{}
	s.counties = []string{"licko-senjska", "sisacko-moslavacka", posaoHRAbroad}
	return s
}

var phEntry = CompanyEntry{Provider: "posaohr"}

func phSeen(ids ...string) func(string) bool {
	return func(id string) bool { return slices.Contains(ids, id) }
}

func phByID(t *testing.T, jobs []Job) map[string]Job {
	t.Helper()
	m := map[string]Job{}
	for _, j := range jobs {
		if _, dup := m[j.ExternalID]; dup {
			t.Fatalf("posting %s yielded twice", j.ExternalID)
		}
		m[j.ExternalID] = j
	}
	return m
}

func TestPosaoHRHydratesNewPostingsAndRefreshesSeenOnes(t *testing.T) {
	f := &posaoHRFake{}
	jobs, err := newTestPosaoHR(f).FetchNew(context.Background(), phEntry, phSeen(phCGP, phITEXIM))
	if err != nil {
		t.Fatal(err)
	}
	if f.urls[0] != posaoHRLanguageURL {
		t.Errorf("first request %s, want the language switch", f.urls[0])
	}
	if n := len(f.postingRequests()); n != 3 {
		t.Errorf("read %d posting pages, want the three unseen: %v", n, f.postingRequests())
	}
	byID := phByID(t, jobs)
	if len(byID) != 5 {
		t.Fatalf("got %d postings, want five (the HZZ card skipped): %v", len(byID), byID)
	}
	if _, ok := byID[phHZZ]; ok {
		t.Error("the employment-service card was yielded")
	}

	cgp := byID[phCGP]
	if !cgp.SeenRefresh || cgp.Company != "CGP d.d." || !slices.Equal(cgp.Countries, []string{"hr"}) {
		t.Errorf("CGP refresh = %+v, want a seen refresh in Croatia", cgp)
	}
	if itexim := byID[phITEXIM]; !itexim.SeenRefresh || itexim.Countries != nil {
		t.Errorf("ITEXIM, filed in Croatia and abroad, = %+v, want no structured country", itexim)
	}

	bent := byID[phBent]
	if bent.Title != "Serviser (m/ž)" || bent.Company != "Bent excellent d.o.o." {
		t.Errorf("Bent identity = %q / %q", bent.Title, bent.Company)
	}
	if bent.Location != "Ličko-senjska županija, Karlovačka županija" {
		t.Errorf("Bent location = %q, want the page's whole place, not the card's cut one", bent.Location)
	}
	if !slices.Equal(bent.Countries, []string{"hr"}) {
		t.Errorf("Bent countries = %v, want hr", bent.Countries)
	}
	for _, want := range []string{"<strong>Plaća:</strong> od 1 500 € neto", "O poslu", "Opis radnog mjesta", "terenski servis", "O zaposleniku", "Potrebno radno iskustvo"} {
		if !strings.Contains(bent.Description, want) {
			t.Errorf("Bent body lacks %q:\n%s", want, bent.Description)
		}
	}
	if strings.Contains(bent.Description, "Kalkulator") || strings.Contains(bent.Description, "Brza prijava") {
		t.Errorf("Bent body carries page chrome:\n%s", bent.Description)
	}
	if bent.ExperienceYearsMin == nil || *bent.ExperienceYearsMin != 3 {
		t.Errorf("Bent experience = %v, want 3 (tri godine)", bent.ExperienceYearsMin)
	}
	if bent.EmploymentType != "" {
		t.Errorf("Bent employment type = %q, want none for a multi-select of durations", bent.EmploymentType)
	}
	if bent.IsTechHint {
		t.Error("Bent is marked technical")
	}

	hsm := byID[phHSM]
	if hsm.Countries != nil || hsm.Location != "Ulm, Njemačka" {
		t.Errorf("HSM = %v / %q, want no structured country and the German place", hsm.Countries, hsm.Location)
	}
	if !strings.Contains(hsm.Description, "Tražimo radnika za gume") || !strings.Contains(hsm.Description, "od 2 453 € bruto") {
		t.Errorf("HSM body lacks the posting:\n%s", hsm.Description)
	}
	if strings.Contains(hsm.Description, "Münsterplatz") || strings.Contains(hsm.Description, "Kontakt podaci") {
		t.Errorf("HSM body carries the employer's contact section:\n%s", hsm.Description)
	}

	pepco := byID[phPepco]
	if !strings.Contains(pepco.Description, "BLAGAJNIK - PRODAVAČ") || !strings.Contains(pepco.Description, "naknadu za topli obrok") {
		t.Errorf("Pepco's designed ad was not read:\n%s", pepco.Description)
	}
	if strings.Contains(pepco.Description, "grid-template") {
		t.Errorf("Pepco's body carries the ad's stylesheet:\n%s", pepco.Description)
	}
	if !slices.Equal(pepco.Countries, []string{"hr"}) {
		t.Errorf("Pepco countries = %v, want hr", pepco.Countries)
	}
}

func TestPosaoHRReadsNewestFirstWithinTheBudget(t *testing.T) {
	f := &posaoHRFake{}
	s := newTestPosaoHR(f)
	s.maxNewPerRun = 1
	jobs, err := s.FetchNew(context.Background(), phEntry, phSeen(phCGP, phITEXIM))
	if err != nil {
		t.Fatal(err)
	}
	if got := f.postingRequests(); len(got) != 1 || !strings.HasSuffix(got[0], "/"+phPepco+"/") {
		t.Errorf("posting requests = %v, want only the newest (Pepco)", got)
	}
	if len(jobs) != 3 {
		t.Errorf("got %d postings, want two refreshes and one hydrated", len(jobs))
	}
}

func TestPosaoHRGateSparesThePostingPage(t *testing.T) {
	f := &posaoHRFake{}
	gate := DetailGate{
		Covered:  func([]string) map[string]bool { return map[string]bool{"Pepco Croatia d.o.o.": true} },
		Rejected: func(j Job) bool { return j.ExternalID == phHSM },
	}
	jobs, err := newTestPosaoHR(f).FetchNewGated(context.Background(), phEntry, phSeen(phCGP, phITEXIM), gate)
	if err != nil {
		t.Fatal(err)
	}
	if got := f.postingRequests(); len(got) != 1 || !strings.HasSuffix(got[0], "/"+phBent+"/") {
		t.Errorf("posting requests = %v, want only Bent's", got)
	}
	byID := phByID(t, jobs)
	if byID[phPepco].Description != "" || byID[phHSM].Description != "" {
		t.Error("a gated posting was hydrated")
	}
}

func TestPosaoHRFailsAPartialListing(t *testing.T) {
	for name, f := range map[string]*posaoHRFake{
		"page error":     {countyErr: errors.New("boom")},
		"not a listing":  {challenge: true},
		"short of total": {emptyPage: true},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := newTestPosaoHR(f).FetchNew(context.Background(), phEntry, phSeen()); err == nil {
				t.Fatal("want the crawl to fail")
			}
			if n := len(f.postingRequests()); n != 0 {
				t.Errorf("read %d posting pages after a failed listing", n)
			}
		})
	}
}

func TestPosaoHRSkipsGoneAndRepublishedPostings(t *testing.T) {
	f := &posaoHRFake{gone: phBent, republished: phHSM}
	jobs, err := newTestPosaoHR(f).FetchNew(context.Background(), phEntry, phSeen(phCGP, phITEXIM))
	if err != nil {
		t.Fatal(err)
	}
	byID := phByID(t, jobs)
	if _, ok := byID[phBent]; ok {
		t.Error("a posting redirected to the home page was yielded")
	}
	if _, ok := byID[phHSM]; ok {
		t.Error("an employment-service vacancy was yielded")
	}
	if byID[phPepco].Description == "" {
		t.Error("the live posting was not hydrated")
	}
}

func TestPosaoHRDefersAPageInAnotherLanguage(t *testing.T) {
	f := &posaoHRFake{english: true}
	_, err := newTestPosaoHR(f).FetchNew(context.Background(), phEntry, phSeen(phCGP, phITEXIM))
	if err == nil || !strings.Contains(err.Error(), "none answered") {
		t.Fatalf("err = %v, want the crawl failing with no page read", err)
	}
}

func TestPosaoHRFacts(t *testing.T) {
	for in, want := range map[string]int{"jedna godina": 1, "dvije godine": 2, "pet godina": 5, "šest do deset godina": 6, "": 0, "nije važno": 0} {
		if got, ok := posaoHRYears(in); got != want || ok != (want > 0) {
			t.Errorf("posaoHRYears(%q) = %d, %v; want %d", in, got, ok, want)
		}
	}
	for in, want := range map[string]string{
		"honorarni posao / freelance":                             "contract",
		"honorarni posao / freelance, student servis":             "",
		"stalni radni odnos":                                      "",
		"stalni radni odnos, rad na određeno vrijeme":             "",
		"rad na određeno vrijeme, student servis, sezonski posao": "",
	} {
		if got := posaoHREmploymentType(in); got != want {
			t.Errorf("posaoHREmploymentType(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{
		"Ličko-senjska županija,...": "Ličko-senjska županija",
		"Zagreb ili terenski rad...": "Zagreb ili terenski rad",
		" Novska ":                   "Novska",
	} {
		if got := posaoHRCardPlace(in); got != want {
			t.Errorf("posaoHRCardPlace(%q) = %q, want %q", in, got, want)
		}
	}
}
