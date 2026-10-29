package sources

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
)

// mojposaoFake serves real fixtures fetched 2026-10-28: a two-page search trimmed to five
// postings — an IT posting (KONČAR - Digital), one with a gross salary (DEKRA), a hybrid one
// (ATM Valve Solutions), an "incognito" employer and one stored already — and the posting API's
// answers for the first three. Any other posting answers 404, as a gone one does.
type mojposaoFake struct {
	listErr  error
	laterErr error
	// emptyLater serves the second page with no postings, as a truncated listing would;
	// emptyGroups serves it with employer groups that hold no posting.
	emptyLater  bool
	emptyGroups bool
	inactive    bool

	mu       sync.Mutex
	lists    []string
	details  []string
	badHeads int
}

func (f *mojposaoFake) GetJSONWithHeaders(_ context.Context, u string, h map[string]string, v any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	ba := strings.HasPrefix(u, "https://www.mojposao.ba/")
	if want := map[bool]string{false: "mojposao.hr", true: "mojposao.ba"}[ba]; h["x-app"] != want {
		f.badHeads++
	}
	if strings.Contains(u, "/jobs/search?") {
		f.lists = append(f.lists, u)
		if f.listErr != nil {
			return f.listErr
		}
		if ba {
			return mojposaoFixture("testdata/mojposao/ba_list_1.json", v)
		}
		if strings.HasSuffix(u, "page=1") {
			return mojposaoFixture("testdata/mojposao/list_1.json", v)
		}
		if f.laterErr != nil {
			return f.laterErr
		}
		if f.emptyGroups {
			return json.Unmarshal([]byte(`{"totalPages":2,"items":[{"id":"x","jobs":[]}]}`), v)
		}
		if f.emptyLater {
			return json.Unmarshal([]byte(`{"totalPages":2,"items":[]}`), v)
		}
		return mojposaoFixture("testdata/mojposao/list_2.json", v)
	}
	f.details = append(f.details, u)
	id := u[strings.LastIndex(u, "/")+1:]
	path := "testdata/mojposao/job_" + id + ".json"
	if _, err := os.Stat(path); err != nil {
		return &StatusError{Method: "GET", Code: 404, URL: u}
	}
	if err := mojposaoFixture(path, v); err != nil {
		return err
	}
	if f.inactive {
		v.(*mojposaoDetail).IsActive = false
	}
	return nil
}

func mojposaoFixture(path string, v any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, v)
}

func newTestMojPosao(f *mojposaoFake) mojposao {
	return mojposao{get: f, limiter: noWait{}, maxNewPerRun: mojposaoMaxNewPerRun, maxPages: mojposaoMaxPages, deadlineMargin: mojposaoDeadlineMargin}
}

var mojposaoHR = CompanyEntry{Board: "mojposao.hr"}

const (
	mojposaoITID     = "2d1f3532-d1f5-11f1-8b88-0280a8fb46bd"
	mojposaoGrossID  = "ddefa77b-d2b0-11f1-8b88-0280a8fb46bd"
	mojposaoHybridID = "04815a32-1e75-4bf5-ab91-dbfef1896530"
	mojposaoHiddenID = "1a07fbe4-d2ba-11f1-8b88-0280a8fb46bd"
	mojposaoSeenID   = "80d956d8-c13f-4bb1-9922-206a1cce8159"
)

func TestMojPosaoHydratesNewPostingsAndRefreshesSeenOnes(t *testing.T) {
	f := &mojposaoFake{}
	jobs, err := newTestMojPosao(f).FetchNew(context.Background(), mojposaoHR, func(id string) bool { return id == mojposaoSeenID })
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	if len(jobs) != 4 {
		t.Fatalf("got %d jobs, want three hydrated and one refreshed (incognito dropped): %+v", len(jobs), jobs)
	}
	if _, ok := byID[mojposaoHiddenID]; ok {
		t.Error("the incognito posting was yielded")
	}
	if r := byID[mojposaoSeenID]; !r.SeenRefresh || r.Description != "" || r.Company != "Valamar Riviera d.d." {
		t.Errorf("seen posting = %+v, want a body-less SeenRefresh", r)
	}

	it := byID[mojposaoITID]
	if it.Company != "KONČAR - Digital d.o.o." || !it.IsTechHint {
		t.Errorf("IT posting company/tech = %q / %v", it.Company, it.IsTechHint)
	}
	if it.Location != "Zagreb" || len(it.Countries) != 1 || it.Countries[0] != "hr" {
		t.Errorf("location = %q countries = %v", it.Location, it.Countries)
	}
	if it.WorkMode != "onsite" || it.EmploymentType != "" {
		t.Errorf("work mode / employment = %q / %q", it.WorkMode, it.EmploymentType)
	}
	if it.ExperienceYearsMin == nil || *it.ExperienceYearsMin != 5 {
		t.Errorf("experience = %v, want 5", it.ExperienceYearsMin)
	}
	if !strings.Contains(it.Description, "<h3>Potrebno obrazovanje, znanja i vještine</h3>") || !strings.Contains(it.Description, "<li>") {
		t.Errorf("description lost its sections or bullets: %.300q", it.Description)
	}
	if strings.Contains(it.Description, "&lt;") {
		t.Errorf("an inline tag was escaped into the text: %.300q", it.Description)
	}
	if it.PostedAt == nil || it.PostedAt.UTC().Format("2006-01-02 15:04") != "2026-10-27 11:20" {
		t.Errorf("posted at = %v", it.PostedAt)
	}
	if it.URL != "https://mojposao.hr/posao/"+mojposaoITID+"/senior-inzenjer-za-vmware-nsx-i-avi-load-balancer-m-z" {
		t.Errorf("url = %q", it.URL)
	}

	g := byID[mojposaoGrossID]
	if g.IsTechHint {
		t.Error("an administrative posting carries the IT hint")
	}
	if g.SalaryMin != nil || g.SalaryPeriod != "" {
		t.Errorf("structured salary set though the platform states no period: %v %q", g.SalaryMin, g.SalaryPeriod)
	}
	if !strings.HasPrefix(g.Description, "<p>Plaća: od 1490 EUR bruto</p>") {
		t.Errorf("salary line missing: %.120q", g.Description)
	}

	if h := byID[mojposaoHybridID]; h.WorkMode != "hybrid" || h.Remote || h.ExperienceYearsMin != nil {
		t.Errorf("hybrid posting = %q remote=%v exp=%v", h.WorkMode, h.Remote, h.ExperienceYearsMin)
	}
	if len(f.details) != 3 {
		t.Errorf("posting requests = %v, want only the three unseen named postings", f.details)
	}
	if f.badHeads != 0 {
		t.Errorf("%d requests went out without the market's x-app header", f.badHeads)
	}
}

func TestMojPosaoBosnianMarket(t *testing.T) {
	f := &mojposaoFake{}
	jobs, err := newTestMojPosao(f).FetchNew(context.Background(), CompanyEntry{Board: "mojposao.ba"}, func(string) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want both Bosnian postings: %+v", len(jobs), jobs)
	}
	for _, j := range jobs {
		if j.Company == "" {
			t.Errorf("posting %s lost the employer its group names", j.ExternalID)
		}
		if !strings.HasPrefix(j.URL, "https://www.mojposao.ba/posao/"+j.ExternalID+"/") {
			t.Errorf("url = %q, want the Bosnian host", j.URL)
		}
		if len(j.FallbackCountries) != 1 || j.FallbackCountries[0] != "ba" {
			t.Errorf("fallback countries = %v", j.FallbackCountries)
		}
		if len(j.Description) < 500 {
			t.Errorf("posting %s: the html-only body was not read: %q", j.ExternalID, j.Description)
		}
	}
	if f.badHeads != 0 {
		t.Errorf("%d requests went out without the market's x-app header", f.badHeads)
	}
}

func TestMojPosaoRejectsAnUnknownBoard(t *testing.T) {
	f := &mojposaoFake{}
	if _, err := newTestMojPosao(f).FetchNew(context.Background(), CompanyEntry{Board: "mojposao.rs"}, func(string) bool { return false }); err == nil {
		t.Error("an unknown market was crawled")
	}
	if len(f.lists) != 0 {
		t.Errorf("requests went out for an unknown market: %v", f.lists)
	}
}

func TestMojPosaoDoesNotJudgeTheCatalogueFilterOnTheCard(t *testing.T) {
	f := &mojposaoFake{}
	gate := DetailGate{Rejected: func(Job) bool { return true }}
	if _, err := newTestMojPosao(f).FetchNewGated(context.Background(), mojposaoHR, func(string) bool { return false }, gate); err != nil {
		t.Fatal(err)
	}
	if len(f.details) != 4 {
		t.Errorf("posting requests = %v, want all four named postings bought: the card lacks the IT position", f.details)
	}
}

func TestMojPosaoGateSkipsCoveredEmployers(t *testing.T) {
	f := &mojposaoFake{}
	gate := DetailGate{Covered: func(cs []string) map[string]bool {
		return map[string]bool{"KONČAR - Digital d.o.o.": true}
	}}
	jobs, err := newTestMojPosao(f).FetchNewGated(context.Background(), mojposaoHR, func(string) bool { return false }, gate)
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range f.details {
		if strings.HasSuffix(u, mojposaoITID) {
			t.Error("a covered employer's posting was bought")
		}
	}
	for _, j := range jobs {
		if j.ExternalID == mojposaoITID && j.Description != "" {
			t.Error("a covered posting was yielded with a body")
		}
	}
}

func TestMojPosaoListingFailuresFailTheCrawl(t *testing.T) {
	cases := map[string]*mojposaoFake{
		"first page":       {listErr: errors.New("boom")},
		"later page":       {laterErr: errors.New("boom")},
		"empty later page": {emptyLater: true},
		"empty groups":     {emptyGroups: true},
	}
	for name, f := range cases {
		if _, err := newTestMojPosao(f).FetchNew(context.Background(), mojposaoHR, func(string) bool { return false }); err == nil {
			t.Errorf("%s: a cut-short listing was reported as a success", name)
		}
	}
}

func TestMojPosaoInactivePostingIsGoneNotAFailure(t *testing.T) {
	f := &mojposaoFake{inactive: true}
	jobs, err := newTestMojPosao(f).FetchNew(context.Background(), mojposaoHR, func(id string) bool { return id == mojposaoSeenID })
	if err != nil {
		t.Fatalf("inactive postings are answers, not failures: %v", err)
	}
	if len(jobs) != 1 {
		t.Errorf("got %d jobs, want only the seen refresh", len(jobs))
	}
}

func TestMojPosaoBudgetBuysNewestFirst(t *testing.T) {
	f := &mojposaoFake{}
	s := newTestMojPosao(f)
	s.maxNewPerRun = 1
	if _, err := s.FetchNew(context.Background(), mojposaoHR, func(string) bool { return false }); err != nil {
		t.Fatal(err)
	}
	if len(f.details) != 1 || !strings.HasSuffix(f.details[0], mojposaoGrossID) {
		t.Errorf("posting requests = %v, want only the newest (published 2026-10-28)", f.details)
	}
}

func TestMojPosaoExperience(t *testing.T) {
	for in, want := range map[string]int{"1-3": 1, "5+": 5, "2": 2, "<1": 0, "": 0, "8-10": 8} {
		got, ok := mojposaoExperience(in)
		if want == 0 && ok || want != 0 && got != want {
			t.Errorf("mojposaoExperience(%q) = %d, %v; want %d", in, got, ok, want)
		}
	}
}
