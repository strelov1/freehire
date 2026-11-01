package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
)

// mojedeloFake serves the real fixtures (fetched 2026-10-29 from api.mojedelo.com): a search page
// trimmed to five rows — a Logatec posting (on site, a salary band, an "O podjetju" paragraph), a
// Ljubljana call-centre posting, an anonymous one, one in Munich with no structured country and
// one in Austria — and the posting resources of the Logatec, Munich and Austrian ones. Any other
// posting answers 404, as a gone one does.
type mojedeloFake struct {
	searchErr error
	// laterErr fails every search page after the first, whose total then claims 500 rows.
	laterErr error
	// detailErr fails every posting resource.
	detailErr error

	mu      sync.Mutex
	urls    []string
	headers []map[string]string
}

const (
	mdLogatec = "6d0cb158-1242-4f65-8953-bf31776bdffb"
	mdClient  = "03397102-366c-42db-b653-4c094e131b6b"
	mdAnon    = "a0e919fa-03b4-4ec0-8a88-13fa948ab455"
	mdMunich  = "404b5144-b14c-43cf-b450-69fb444880c2"
	mdAustria = "934adb93-aed8-4151-8b11-0e9ea5316b7d"
)

func (f *mojedeloFake) GetJSONWithHeaders(_ context.Context, u string, headers map[string]string, v any) error {
	f.mu.Lock()
	f.urls = append(f.urls, u)
	f.headers = append(f.headers, headers)
	f.mu.Unlock()
	if f.detailErr != nil && strings.HasPrefix(u, mojedeloAPI+"/job-ads/") {
		return f.detailErr
	}
	var path string
	switch {
	case u == fmt.Sprintf(mojedeloSearchURL, mojedeloPageSize, 0):
		if f.searchErr != nil {
			return f.searchErr
		}
		path = "testdata/mojedelo/listing.json"
	case strings.HasPrefix(u, mojedeloAPI+"/job-ads-search"):
		if f.laterErr != nil {
			return f.laterErr
		}
		return json.Unmarshal([]byte(`{"data":{"items":[],"total":0}}`), v)
	case u == fmt.Sprintf(mojedeloJobURL, mdLogatec), u == fmt.Sprintf(mojedeloJobURL, mdMunich), u == fmt.Sprintf(mojedeloJobURL, mdAustria):
		path = "testdata/mojedelo/job_" + u[len(u)-36:] + ".json"
	default:
		return &StatusError{Method: "GET", Code: 404, URL: u}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return err
	}
	if s, ok := v.(*mojedeloSearch); ok && f.laterErr != nil {
		s.Data.Total = 500
	}
	return nil
}

func (f *mojedeloFake) postingRequests() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, u := range f.urls {
		if strings.HasPrefix(u, mojedeloAPI+"/job-ads/") {
			n++
		}
	}
	return n
}

func newTestMojeDelo(f *mojedeloFake) mojedelo {
	s := NewMojeDelo(f).(mojedelo)
	s.limiter = noWait{}
	return s
}

func TestMojeDeloHydratesNewPostingsAndRefreshesSeenOnes(t *testing.T) {
	f := &mojedeloFake{}
	jobs, err := newTestMojeDelo(f).FetchNew(context.Background(), CompanyEntry{}, func(id string) bool {
		return id == mdClient
	})
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	if len(jobs) != 4 {
		t.Fatalf("got %d jobs, want three hydrated and one refreshed (the anonymous one dropped): %+v", len(jobs), jobs)
	}
	if _, ok := byID[mdAnon]; ok {
		t.Error("the anonymous posting was yielded under the site operator's name")
	}
	if f.postingRequests() != 3 {
		t.Errorf("posting requests = %d, want 3 (the seen and the anonymous one cost none)", f.postingRequests())
	}
	for _, h := range f.headers {
		if h["tenantId"] == "" || h["channelId"] == "" || h["languageId"] == "" {
			t.Fatalf("a request went out without the tenant headers: %v", h)
		}
	}

	g := byID[mdLogatec]
	if g.Title != "KV monter hladilnih naprav" || g.Company != "ALPFRIGO d.o.o." || g.Location != "Logatec" {
		t.Errorf("title/company/place = %q / %q / %q", g.Title, g.Company, g.Location)
	}
	if g.URL != "https://www.mojedelo.com/oglas/kv-monter-hladilnih-naprav/"+mdLogatec {
		t.Errorf("url = %q", g.URL)
	}
	if len(g.Countries) != 1 || g.Countries[0] != "si" {
		t.Errorf("countries = %v, want [si]", g.Countries)
	}
	if g.WorkMode != "onsite" || g.Remote || g.EmploymentType != "full_time" {
		t.Errorf("work mode / remote / employment = %q / %v / %q", g.WorkMode, g.Remote, g.EmploymentType)
	}
	if g.SalaryMin == nil || *g.SalaryMin != 1501 || *g.SalaryMax != 2000 || g.SalaryCurrency != "EUR" || g.SalaryPeriod != "month" {
		t.Errorf("salary = %v-%v %q %q", g.SalaryMin, g.SalaryMax, g.SalaryCurrency, g.SalaryPeriod)
	}
	// endDate is the last day's local end (23:59:59 CET), so its UTC date is that day.
	if g.ApplicationDeadline == nil || g.ApplicationDeadline.Format("2006-01-02") != "2026-11-07" {
		t.Errorf("application deadline = %v, want 2026-11-07", g.ApplicationDeadline)
	}
	if g.ExperienceYearsMin == nil || *g.ExperienceYearsMin != 1 {
		t.Errorf("experience = %v, want 1", g.ExperienceYearsMin)
	}
	if g.PostedAt == nil || g.PostedAt.Format("2006-01-02") != "2026-10-12" {
		t.Errorf("posted = %v", g.PostedAt)
	}
	for _, want := range []string{"Pričakujemo", "Nudimo", "O podjetju"} {
		if !strings.Contains(g.Description, want) {
			t.Errorf("description lacks %q: %.300q", want, g.Description)
		}
	}

	m := byID[mdMunich]
	if m.Location != "München" || len(m.Countries) != 0 {
		t.Errorf("Munich posting = %q %v, want the town and no structured country", m.Location, m.Countries)
	}
	a := byID[mdAustria]
	if a.Location != "Tilmitsch ali Gleinstätten" || len(a.Countries) != 1 || a.Countries[0] != "at" {
		t.Errorf("Austrian posting = %q %v, want the free-text place and [at]", a.Location, a.Countries)
	}

	c := byID[mdClient]
	if !c.SeenRefresh || c.Description != "" || c.Company == "" {
		t.Errorf("seen posting = %+v, want a body-less refresh", c)
	}
}

func TestMojeDeloGateSkipsTheBodyOfACoveredEmployer(t *testing.T) {
	f := &mojedeloFake{}
	gate := DetailGate{Covered: func(companies []string) map[string]bool {
		return map[string]bool{"ALPFRIGO d.o.o.": true}
	}}
	jobs, err := newTestMojeDelo(f).FetchNewGated(context.Background(), CompanyEntry{}, func(string) bool { return false }, gate)
	if err != nil {
		t.Fatal(err)
	}
	// The other three unseen postings are read (the call-centre one answers 404, gone).
	if f.postingRequests() != 3 {
		t.Errorf("posting requests = %d, want 3 (none for the covered employer)", f.postingRequests())
	}
	for _, j := range jobs {
		if j.ExternalID == mdLogatec && j.Description != "" {
			t.Error("a covered employer's posting was hydrated")
		}
	}
}

func TestMojeDeloFailsTheCrawlOnAnySearchPage(t *testing.T) {
	boom := errors.New("boom")
	if _, err := newTestMojeDelo(&mojedeloFake{searchErr: boom}).Fetch(context.Background(), CompanyEntry{}); !errors.Is(err, boom) {
		t.Errorf("first page failing: err = %v", err)
	}
	if _, err := newTestMojeDelo(&mojedeloFake{laterErr: boom}).Fetch(context.Background(), CompanyEntry{}); !errors.Is(err, boom) {
		t.Errorf("later page failing: err = %v, want the crawl failed", err)
	}
}

func TestMojeDeloFailsWhenNoPostingAnswers(t *testing.T) {
	_, err := newTestMojeDelo(&mojedeloFake{detailErr: errors.New("timeout")}).Fetch(context.Background(), CompanyEntry{})
	if err == nil {
		t.Fatal("a crawl whose every posting failed reported success")
	}
}

func TestMojeDeloBoundsNewPostingsPerRun(t *testing.T) {
	f := &mojedeloFake{}
	s := newTestMojeDelo(f)
	s.maxNewPerRun = 1
	jobs, err := s.Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatal(err)
	}
	if f.postingRequests() != 1 || len(jobs) != 1 {
		t.Errorf("posting requests / jobs = %d / %d, want 1 / 1", f.postingRequests(), len(jobs))
	}
}

func TestMojeDeloFacets(t *testing.T) {
	named := func(s ...string) []mojedeloNamed {
		var out []mojedeloNamed
		for _, x := range s {
			out = append(out, mojedeloNamed{Translation: x})
		}
		return out
	}
	for _, tc := range []struct {
		modes []mojedeloNamed
		want  string
	}{
		{named("Od doma"), "remote"},
		{named("Hibridno (delno od doma)"), "hybrid"},
		{named("Na delovnem mestu"), "onsite"},
		{named("Na terenu"), ""},
		{named("Od doma", "Hibridno (delno od doma)", "Na delovnem mestu"), ""},
		{nil, ""},
	} {
		if got := mojedeloWorkMode(tc.modes); got != tc.want {
			t.Errorf("work mode %v = %q, want %q", tc.modes, got, tc.want)
		}
	}
	for band, want := range map[string]int{"0 to 1 year": 0, "1+ to 3 years": 1, "5+ to 10 years": 5, "Not important": 0} {
		if got := mojedeloYears(band); got != want {
			t.Errorf("years %q = %d, want %d", band, got, want)
		}
	}
	for band, want := range map[string][2]int{"1101 - 1500 EUR Gross": {1101, 1500}, "negotiable": {}, "0 - 800 EUR Gross": {}, "10000+ EUR Gross": {}} {
		lo, hi, ok := mojedeloSalary(band)
		if ok != (want != [2]int{}) || lo != want[0] || hi != want[1] {
			t.Errorf("salary %q = %d-%d %v, want %v", band, lo, hi, ok, want)
		}
	}
	full := &mojedeloNamed{Name: "Full time"}
	part := &mojedeloNamed{Name: "Part time"}
	for _, tc := range []struct {
		wt    *mojedeloNamed
		types []mojedeloNamed
		want  string
	}{
		{full, []mojedeloNamed{{Name: "Permanent employment"}}, "full_time"},
		{part, nil, "part_time"},
		{full, []mojedeloNamed{{Name: "Contract work"}}, "contract"},
		{full, []mojedeloNamed{{Name: "Practise"}}, "internship"},
		{full, []mojedeloNamed{{Name: "Temporary employment"}, {Name: "Contract work"}}, "full_time"},
		{nil, []mojedeloNamed{{Name: "Student work"}}, ""},
	} {
		if got := mojedeloEmploymentType(tc.wt, tc.types); got != tc.want {
			t.Errorf("employment %v %v = %q, want %q", tc.wt, tc.types, got, tc.want)
		}
	}
}
