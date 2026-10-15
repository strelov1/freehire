package sources

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// The fixtures are trimmed from live occ.com.mx responses fetched 2026-10-15: listing page 1 cut
// to the two cards the site repeats at the top of every page plus three more (one of them an
// "Empresa confidencial" card), the category's real last page (219) verbatim, the page past it,
// and each posting JSON cut to the mapped keys (the recruiter's contact block removed).

const occTI = "tecnologias-de-la-informacion-sistemas"

type occFake struct {
	t     *testing.T
	pages map[string]string // URL → fixture name
	errs  map[string]error
	got   []string
}

func newOCCFake(t *testing.T) *occFake {
	f := &occFake{t: t, pages: map[string]string{
		"https://www.occ.com.mx/empleos/trabajo-en-" + occTI + "/":        "list_page1.html",
		"https://www.occ.com.mx/empleos/trabajo-en-" + occTI + "/?page=2": "list_page2.html",
		"https://www.occ.com.mx/empleos/trabajo-en-" + occTI + "/?page=3": "list_past_end.html",
	}}
	for _, id := range []string{"21367373", "21375686", "21375594", "21375580", "21297815"} {
		f.pages["https://oferta.occ.com.mx/offer/"+id+"/d/j?ipo=41&iapo=1"] = "offer_" + id + ".json"
	}
	return f
}

func (f *occFake) read(url string) ([]byte, error) {
	f.got = append(f.got, url)
	if err, ok := f.errs[url]; ok {
		return nil, err
	}
	name, ok := f.pages[url]
	if !ok {
		return nil, &StatusError{Method: "GET", Code: 404, URL: url}
	}
	b, err := os.ReadFile("testdata/occ/" + name)
	if err != nil {
		f.t.Fatal(err)
	}
	return b, nil
}

func (f *occFake) GetHTML(_ context.Context, url string) (*html.Node, error) {
	b, err := f.read(url)
	if err != nil {
		return nil, err
	}
	return html.Parse(strings.NewReader(string(b)))
}

func (f *occFake) GetJSON(_ context.Context, url string, v any) error {
	b, err := f.read(url)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func occFetch(t *testing.T, f *occFake) map[string]Job {
	t.Helper()
	jobs, err := NewOCC(f).Fetch(context.Background(), CompanyEntry{Board: occTI})
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]Job{}
	for _, j := range jobs {
		out[j.ExternalID] = j
	}
	return out
}

func TestOCCWalksPastTheRepeatedCardsAndSkipsConfidential(t *testing.T) {
	f := newOCCFake(t)
	jobs := occFetch(t, f)
	// Six distinct cards: the two repeated on every page, three more on page 1 (one
	// confidential) and one on page 2. The page past the end adds nothing and ends the walk.
	if len(jobs) != 5 {
		t.Fatalf("jobs = %d, want 5: %v", len(jobs), f.got)
	}
	if _, ok := jobs["21375513"]; ok {
		t.Error("a confidential posting was stored")
	}
	for _, u := range f.got {
		if strings.Contains(u, "21375513") {
			t.Errorf("a confidential posting cost a request: %s", u)
		}
		if strings.Contains(u, "?page=4") {
			t.Errorf("walk did not stop at the page past the end: %s", u)
		}
	}
}

func TestOCCMapsThePostingJSON(t *testing.T) {
	jobs := occFetch(t, newOCCFake(t))
	pm := jobs["21375686"]
	if pm.Title != "Project manager" || pm.Company != "PROCETI" || !pm.IsTechHint ||
		pm.Location != "San Pedro Garza García, Nuevo León" || pm.WorkMode != "onsite" || pm.Remote ||
		pm.EmploymentType != "full_time" {
		t.Errorf("identity: %+v", pm)
	}
	if pm.URL != "https://www.occ.com.mx/empleo/oferta/21375686-project-manager" {
		t.Errorf("url = %s", pm.URL)
	}
	if pm.SalaryMin == nil || *pm.SalaryMin != 45000 || pm.SalaryMax == nil || *pm.SalaryMax != 50000 ||
		pm.SalaryCurrency != "MXN" || pm.SalaryPeriod != "month" {
		t.Errorf("salary: %v-%v %s/%s", pm.SalaryMin, pm.SalaryMax, pm.SalaryCurrency, pm.SalaryPeriod)
	}
	if pm.PostedAt == nil || pm.PostedAt.Format("2006-01-02") != "2026-10-14" {
		t.Errorf("posted = %v", pm.PostedAt)
	}
	if !strings.Contains(pm.Description, "Supply Chain") {
		t.Errorf("body: %.200s", pm.Description)
	}
	// A salary the employer withholds is a sentence, not a range.
	sap := jobs["21367373"]
	if sap.SalaryMin != nil || sap.SalaryCurrency != "" || sap.Location != "México" {
		t.Errorf("hidden salary / state-only place: %+v", sap)
	}
}

func TestOCCRemoteAndScheduleComeFromTheFilterLinks(t *testing.T) {
	b, err := os.ReadFile("testdata/occ/offer_21299142.json")
	if err != nil {
		t.Fatal(err)
	}
	var d occDetail
	if err := json.Unmarshal(b, &d); err != nil {
		t.Fatal(err)
	}
	j, ok := d.toJob("21299142", true)
	if !ok || !j.Remote || j.WorkMode != "remote" || j.EmploymentType != "part_time" || j.Company != "iMerit Technology" {
		t.Errorf("remote posting: ok=%v %+v", ok, j)
	}
	// The JSON must be the posting the card named.
	if _, ok := d.toJob("1", true); ok {
		t.Error("a detail for another id was accepted")
	}
}

func TestOCCSeenPostingCostsNoDetail(t *testing.T) {
	f := newOCCFake(t)
	jobs, err := NewOCC(f).(HydratingSource).FetchNew(context.Background(), CompanyEntry{Board: occTI},
		func(string) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 5 {
		t.Fatalf("jobs = %d, want 5", len(jobs))
	}
	for _, j := range jobs {
		if !j.SeenRefresh || j.Title == "" {
			t.Errorf("seen posting: %+v", j)
		}
	}
	for _, u := range f.got {
		if strings.Contains(u, "oferta.occ.com.mx") {
			t.Errorf("seen posting fetched: %s", u)
		}
	}
}

func TestOCCFailures(t *testing.T) {
	if _, err := NewOCC(newOCCFake(t)).Fetch(context.Background(), CompanyEntry{Board: "../x"}); err == nil {
		t.Error("a board that is not a slug must fail")
	}
	f := newOCCFake(t)
	f.errs = map[string]error{"https://www.occ.com.mx/empleos/trabajo-en-" + occTI + "/": errors.New("403")}
	if _, err := NewOCC(f).Fetch(context.Background(), CompanyEntry{Board: occTI}); err == nil {
		t.Error("a first page that fails must fail the board")
	}
	f = newOCCFake(t)
	for u := range f.pages {
		if strings.Contains(u, "oferta.occ.com.mx") {
			f.errs = map[string]error{}
			delete(f.pages, u)
		}
	}
	f.errs["https://oferta.occ.com.mx/offer/21375686/d/j?ipo=41&iapo=1"] = &StatusError{Method: "GET", Code: 503}
	if _, err := NewOCC(f).Fetch(context.Background(), CompanyEntry{Board: occTI}); err == nil {
		t.Error("listing new postings and reading none of them must fail")
	}
}

// onBoard re-serves the fixtures under another category's URLs.
func (f *occFake) onBoard(board string) *occFake {
	for u, name := range f.pages {
		if strings.Contains(u, occTI) {
			f.pages[strings.Replace(u, occTI, board, 1)] = name
		}
	}
	return f
}

// The catalogue is broader than IT, so every category is a board; only the site's own IT
// category vouches that its postings are technical. The rest are left to the classifier.
func TestOCCOnlyTheITCategoryHintsTech(t *testing.T) {
	jobs, err := NewOCC(newOCCFake(t).onBoard("ventas")).Fetch(context.Background(), CompanyEntry{Board: "ventas"})
	if err != nil || len(jobs) == 0 {
		t.Fatalf("Fetch = %d jobs, %v", len(jobs), err)
	}
	for _, j := range jobs {
		if j.IsTechHint {
			t.Errorf("%s on the ventas board is hinted technical", j.ExternalID)
		}
	}
	for _, j := range occFetch(t, newOCCFake(t)) {
		if !j.IsTechHint {
			t.Errorf("%s on the IT board is not hinted technical", j.ExternalID)
		}
	}
}

// A board whose new postings outnumber what one run can read defers the rest, unstored and so
// still new, instead of pushing the run past its timeout with nothing written.
func TestOCCHydratesAtMostTheBudgetPerBoard(t *testing.T) {
	f := newOCCFake(t)
	jobs, err := occ{http: f, maxNewPerBoard: 1}.Fetch(context.Background(), CompanyEntry{Board: occTI})
	if err != nil {
		t.Fatal(err)
	}
	details := 0
	for _, u := range f.got {
		if strings.HasPrefix(u, "https://oferta.occ.com.mx/") {
			details++
		}
	}
	if details != 1 || len(jobs) != 1 {
		t.Fatalf("detail requests %d, jobs %d; want one of each", details, len(jobs))
	}
}
