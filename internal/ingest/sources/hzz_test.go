package sources

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/html"
)

// hzzFake serves the real fixtures (fetched 2026-10-28): one county feed trimmed to three items
// in the site's ISO-8859-2 bytes (167579841 and 167016223, whose print pages are fixtures — the
// first with an empty feed title — and the title-less 167397641, which answers the expiry
// notice like any id with no fixture), three
// print pages and the expiry notice, all with their scripts and styles stripped. Every other
// county feed is empty, and any other vacancy answers the expiry notice, as the site does.
type hzzFake struct {
	countyErr map[int]error
	techErr   error
	pageErr   error

	mu    sync.Mutex
	pages []string
}

const hzzEmptyFeed = `<?xml version="1.0" encoding="iso-8859-2"?><rss version="2.0"><channel><title>HZZ</title></channel></rss>`

// hzzTechFeedDoc names one of the county feed's vacancies as an IT one.
const hzzTechFeedDoc = `<?xml version="1.0" encoding="iso-8859-2"?><rss version="2.0"><channel>
<item><title>X</title><pubDate>22.10.2026</pubDate><link>http://burzarada.hzz.hr/RadnoMjesto_Ispis.aspx?WebSifra=167016223</link></item>
<item><title>ABROAD</title><pubDate>22.10.2026</pubDate><link>http://burzarada.hzz.hr/RadnoMjesto_Ispis.aspx?WebSifra=999</link></item>
</channel></rss>`

func (f *hzzFake) GetXML(ctx context.Context, u string, v any) error {
	doc, err := f.raw(u)
	if err != nil {
		return err
	}
	return newXMLDecoder(strings.NewReader(doc)).Decode(v)
}

func (f *hzzFake) raw(u string) (string, error) {
	if u == hzzTechFeed {
		if f.techErr != nil {
			return "", f.techErr
		}
		return hzzTechFeedDoc, nil
	}
	for county := 1; county <= hzzCounties; county++ {
		if u != fmt.Sprintf(hzzCountyFeed, county) {
			continue
		}
		if err := f.countyErr[county]; err != nil {
			return "", err
		}
		if county != 4 {
			return hzzEmptyFeed, nil
		}
		raw, err := os.ReadFile("testdata/hzz/feed_county.xml")
		return string(raw), err
	}
	return "", &StatusError{Method: "GET", Code: 404, URL: u}
}

func (f *hzzFake) GetHTML(_ context.Context, u string) (*html.Node, error) {
	f.mu.Lock()
	f.pages = append(f.pages, u)
	f.mu.Unlock()
	if f.pageErr != nil {
		return nil, f.pageErr
	}
	file := "expired.html"
	switch u {
	case hzzPostingURL(167579841):
		file = "print_167579841.html"
	case hzzPostingURL(167016223):
		file = "print_167016223.html"
	}
	raw, err := os.ReadFile("testdata/hzz/" + file)
	if err != nil {
		return nil, err
	}
	return html.Parse(bytes.NewReader(raw))
}

func newTestHZZ(f *hzzFake) hzz {
	s := NewHZZ(f).(hzz)
	s.limiter = noWait{}
	return s
}

func hzzByID(jobs []Job) map[string]Job {
	out := map[string]Job{}
	for _, j := range jobs {
		out[j.ExternalID] = j
	}
	return out
}

func TestHZZParseFeedListsTitlelessItemsAndDecodesCroatian(t *testing.T) {
	f := &hzzFake{}
	doc, err := newTestHZZ(f).feed(context.Background(), fmt.Sprintf(hzzCountyFeed, 4))
	if err != nil {
		t.Fatal(err)
	}
	items := hzzFeedItems(doc)
	if len(items) != 3 {
		t.Fatalf("items = %d, want 3 (a title-less item is usually an open vacancy)", len(items))
	}
	by := map[int64]hzzPosting{}
	for _, it := range items {
		by[it.id] = it
	}
	if by[167579841].title != "" || by[167016223].title != "BRAVAR / BRAVARICA" {
		t.Errorf("titles = %q, %q", by[167579841].title, by[167016223].title)
	}
	if p := by[167016223].posted; p == nil || p.Format("2006-01-02") != "2026-10-01" {
		t.Errorf("posted = %v, want 2026-10-01", p)
	}
}

func TestHZZFetchNewHydratesNewVacanciesOnly(t *testing.T) {
	f := &hzzFake{}
	seen := func(id string) bool { return id == "167016223" }
	jobs, err := newTestHZZ(f).FetchNew(context.Background(), CompanyEntry{}, seen)
	if err != nil {
		t.Fatal(err)
	}
	by := hzzByID(jobs)
	if len(by) != 2 {
		t.Fatalf("jobs = %d, want 2 (the expired vacancy is dropped): %+v", len(by), jobs)
	}
	if r := by["167016223"]; !r.SeenRefresh || !r.IsTechHint {
		t.Errorf("a stored vacancy must be refreshed by identity, carrying the IT feed's mark: %+v", r)
	}
	if len(f.pages) != 2 || f.pages[0] == hzzPostingURL(167016223) || f.pages[1] == hzzPostingURL(167016223) {
		t.Errorf("pages read = %v, want only the two new vacancies", f.pages)
	}
	j := by["167579841"]
	if j.Company != "BOVIS d.o.o." {
		t.Errorf("company = %q, want the register name without its activity", j.Company)
	}
	if j.Title != "STOČARSKI RADNIK/STOČARSKA RADNICA" || j.Description == "" {
		t.Errorf("title %q (an empty feed title is read from the page), description %d bytes", j.Title, len(j.Description))
	}
	if j.Location != "Ivankovo, Vukovarsko-Srijemska Županija" {
		t.Errorf("location = %q", j.Location)
	}
	if j.IsTechHint {
		t.Error("only a vacancy the IT category feed names is technical")
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("employment = %q", j.EmploymentType)
	}
	if j.PostedAt == nil || j.PostedAt.Format("2006-01-02") != "2026-10-19" {
		t.Errorf("posted = %v, want the page's 19.10.2026", j.PostedAt)
	}
	if len(j.FallbackCountries) != 1 || j.FallbackCountries[0] != "hr" {
		t.Errorf("fallback countries = %v", j.FallbackCountries)
	}
}

func TestHZZJobReadsStructuredSalaryAndTrainee(t *testing.T) {
	read := func(file string) Job {
		raw, err := os.ReadFile("testdata/hzz/" + file)
		if err != nil {
			t.Fatal(err)
		}
		root, err := html.Parse(bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		j, ok := hzzJob(root, hzzPosting{id: 1, title: "feed title"})
		if !ok {
			t.Fatalf("%s: not a job", file)
		}
		return j
	}
	pay := read("print_167579841.html")
	if pay.SalaryMin == nil || *pay.SalaryMin != 1050 || *pay.SalaryMax != 1050 || pay.SalaryCurrency != "EUR" || pay.SalaryPeriod != "month" {
		t.Errorf("salary = %v-%v %s/%s, want 1050 EUR/month", pay.SalaryMin, pay.SalaryMax, pay.SalaryCurrency, pay.SalaryPeriod)
	}
	if !strings.Contains(pay.Description, "hranidbu") {
		t.Errorf("the Opis posla field is missing from the body: %.200s", pay.Description)
	}
	trainee := read("print_167680338.html")
	if trainee.EmploymentType != "internship" {
		t.Errorf("employment = %q, want internship for a pripravnik call", trainee.EmploymentType)
	}
	if trainee.Company != "HRVATSKI ZAVOD ZA MIROVINSKO OSIGURANJE" {
		t.Errorf("company = %q", trainee.Company)
	}
	if !strings.Contains(trainee.Description, "Viši informatičar") || strings.Contains(trainee.Description, "pisana zamolba") {
		t.Errorf("body must be the Ostale informacije block and stop before the employer section: %.300s", trainee.Description)
	}
	if trainee.SalaryMin != nil {
		t.Error("no salary is stated on this page")
	}
}

func TestHZZExpiredVacancyIsGoneNotFailed(t *testing.T) {
	f := &hzzFake{}
	_, ok, err := newTestHZZ(f).detail(context.Background(), hzzPosting{id: 5})
	if err != nil || ok {
		t.Fatalf("ok=%v err=%v, want the expiry notice read as gone", ok, err)
	}
}

func TestHZZCountyFeedFailureFailsTheCrawl(t *testing.T) {
	f := &hzzFake{countyErr: map[int]error{7: errors.New("boom")}}
	if _, err := newTestHZZ(f).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false }); err == nil {
		t.Fatal("a missing county must fail the crawl, or the sweep closes that county's vacancies")
	}
}

func TestHZZTechFeedFailureCostsOnlyTheMark(t *testing.T) {
	f := &hzzFake{techErr: errors.New("boom")}
	jobs, err := newTestHZZ(f).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	for _, j := range jobs {
		if j.IsTechHint {
			t.Errorf("%s marked technical with no IT feed", j.ExternalID)
		}
	}
}

func TestHZZAllPagesFailingFailsTheCrawl(t *testing.T) {
	f := &hzzFake{pageErr: errors.New("timeout")}
	if _, err := newTestHZZ(f).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false }); err == nil {
		t.Fatal("a crawl that read no page must fail")
	}
}

func TestHZZBudgetAndDeadlineDefer(t *testing.T) {
	f := &hzzFake{}
	s := newTestHZZ(f)
	s.maxNewPerRun = 1
	jobs, err := s.FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	if len(f.pages) != 1 || len(jobs) != 1 {
		t.Errorf("pages %d, jobs %d: want one page read and one vacancy deferred", len(f.pages), len(jobs))
	}

	f = &hzzFake{}
	s = newTestHZZ(f)
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Minute))
	defer cancel()
	if _, err := s.FetchNew(ctx, CompanyEntry{}, func(string) bool { return false }); err != nil {
		t.Fatal(err)
	}
	if len(f.pages) != 0 {
		t.Errorf("read %d pages inside the deadline margin", len(f.pages))
	}
}

func TestHZZRejectedTitleCostsNoPage(t *testing.T) {
	f := &hzzFake{}
	gate := DetailGate{Rejected: func(Job) bool { return true }}
	jobs, err := newTestHZZ(f).FetchNewGated(context.Background(), CompanyEntry{}, func(string) bool { return false }, gate)
	if err != nil {
		t.Fatal(err)
	}
	// 167016223's feed title is rejected: yielded body-less, no page. The two title-less
	// vacancies cannot be judged before their page and are read.
	if len(f.pages) != 2 || len(jobs) != 2 {
		t.Errorf("pages %v, jobs %d: a rejected vacancy is yielded body-less without a page", f.pages, len(jobs))
	}
	for _, p := range f.pages {
		if p == hzzPostingURL(167016223) {
			t.Error("the rejected vacancy's page was bought")
		}
	}
}

func TestHZZCompanyName(t *testing.T) {
	for in, want := range map[string]string{
		"EUROTIM d.o.o. za proizvodnju i trgovinu":                                                              "EUROTIM d.o.o.",
		"JLM - PERKOVIĆ d. o. o. za proizvodnju, trgovinu i inženjering":                                        "JLM - PERKOVIĆ d. o. o.",
		"CONER  društvo s ograničenom odgovornošću za proizvodnju, trgovinu i usluge":                           "CONER d.o.o.",
		"FERRO-PREIS  D.O.O. ZA PROIZVODNJU LJEVANIH,KOVANIH I PREŠANIH METALNIH PROIZVODA":                     "FERRO-PREIS D.O.O.",
		`OBRT ZA UGOSTITELJSTVO, VINOGRADARSTVO, TRGOVINU I IZNAJMLJIVANJE "GT LUDBREG"- Sanja S, Ludbreg, K 2`: "GT LUDBREG",
		"MEXICAN BAR TEQUILA, ugostiteljski obrt, vl. D P, Vukovar, S R 72":                                     "MEXICAN BAR TEQUILA",
		"DJEČJI VRTIĆ VLADIMIR NAZOR, DARUVAR":                                                                  "DJEČJI VRTIĆ VLADIMIR NAZOR, DARUVAR",
		"HRVATSKI ZAVOD ZA MIROVINSKO OSIGURANJE":                                                               "HRVATSKI ZAVOD ZA MIROVINSKO OSIGURANJE",
	} {
		if got := hzzCompanyName(in); got != want {
			t.Errorf("hzzCompanyName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHZZSalaryBounds(t *testing.T) {
	for in, want := range map[string]int{"1210 €": 1210, "1.400 €": 1400, "1050,00 €": 1050, "1 €": 0, "250000 €": 0, "dogovor": 0} {
		got, ok := hzzSalary(in)
		if (want == 0) == ok || (ok && got != want) {
			t.Errorf("hzzSalary(%q) = %d,%v, want %d", in, got, ok, want)
		}
	}
}
