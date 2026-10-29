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
	"time"

	"golang.org/x/time/rate"
)

// essFixture reads what the ESS API served on 2026-10-29: listing pages trimmed to a few cards
// (their totals restated to match), and detail records with the contact block and the
// employer's street address replaced by placeholders.
func essFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/ess/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return b
}

// essFake answers the listing by slice and page, and the detail by id.
type essFake struct {
	mu       sync.Mutex
	listing  map[string][]byte // key: essFakeKey(slice, page)
	listErr  map[string]error
	details  map[string][]byte
	detErr   map[string]error
	detCalls []string
}

func essFakeKey(field, workingTime string, page int) string {
	return fmt.Sprintf("%s|%s|%d", field, workingTime, page)
}

func (f *essFake) PostJSON(_ context.Context, url string, body, v any) error {
	if !strings.HasPrefix(url, essListingURL+"?user_key=") {
		return fmt.Errorf("unexpected POST %s", url)
	}
	req := body.(essSearchRequest)
	if len(req.Countries) != 1 || req.Countries[0] != "SI" || req.Sort != essSortNewest {
		return fmt.Errorf("unexpected query %+v", req)
	}
	field := ""
	if len(req.Fields) == 1 {
		field = req.Fields[0]
	}
	key := essFakeKey(field, req.WorkingTime, req.Page)
	if err := f.listErr[key]; err != nil {
		return err
	}
	b, ok := f.listing[key]
	if !ok {
		b = []byte(`{"steviloDelovnihMest":0,"seznamDelovnihMest":[]}`)
	}
	return json.Unmarshal(b, v)
}

func (f *essFake) GetJSON(_ context.Context, url string, v any) error {
	_, id, ok := strings.Cut(url, "&idDelovnoMesto=")
	if !strings.HasPrefix(url, essDetailURL+"?user_key=") || !ok {
		return fmt.Errorf("unexpected GET %s", url)
	}
	f.mu.Lock()
	f.detCalls = append(f.detCalls, id)
	f.mu.Unlock()
	if err := f.detErr[id]; err != nil {
		return err
	}
	b, ok := f.details[id]
	if !ok {
		b = []byte(`{"idDelovnoMesto":"` + id + `","steviloOgledov":1}`)
	}
	return json.Unmarshal(b, v)
}

func newTestESS(t *testing.T) (ess, *essFake) {
	fake := &essFake{
		listing: map[string][]byte{
			essFakeKey("", "", 1):          essFixture(t, "listing_1.json"),
			essFakeKey("", "", 2):          essFixture(t, "listing_2.json"),
			essFakeKey(essITField, "", 1):  essFixture(t, "listing_it.json"),
			essFakeKey("", essPartTime, 1): essFixture(t, "listing_parttime.json"),
		},
		listErr: map[string]error{},
		details: map[string][]byte{
			"3475977": essFixture(t, "detail_3475977.json"),
			"3497653": essFixture(t, "detail_3497653.json"),
			"3488829": essFixture(t, "detail_3488829.json"),
			"3499064": essFixture(t, "detail_3499064.json"),
		},
		detErr: map[string]error{},
	}
	s := NewESS(fake).(ess)
	s.limiter = rate.NewLimiter(rate.Inf, 1)
	return s, fake
}

func TestESSProvider(t *testing.T) {
	s, ok := All(nil)["ess"]
	if !ok || s.Provider() != "ess" {
		t.Fatal("All() must register ess")
	}
	if _, ok := s.(CoverageGated); !ok {
		t.Error("ess must be CoverageGated: the body costs a request and the listing names the employer")
	}
	if _, ok := s.(boardless); !ok {
		t.Error("ess must be boardless: one national listing names every vacancy")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("ess must be an aggregator: the employer is read per vacancy")
	}
}

func TestESSHydratesUnseenVacancies(t *testing.T) {
	s, _ := newTestESS(t)
	jobs, err := s.FetchNew(context.Background(), CompanyEntry{Provider: "ess"}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 4 {
		t.Fatalf("got %d jobs, want 4 (page 2 repeats a page-1 card): %+v", len(jobs), jobs)
	}
	got := byExternalID(jobs)

	nurse := got["3475977"]
	if nurse.Title != "SREDNJA MEDICINSKA SESTRA I" {
		t.Errorf("title = %q, want the gender marker cut", nurse.Title)
	}
	if nurse.Company != "Dom na Krasu" || nurse.Location != "Nova Gorica" {
		t.Errorf("company/location = %q / %q", nurse.Company, nurse.Location)
	}
	if nurse.URL != essPostingURL+"3475977" {
		t.Errorf("url = %q", nurse.URL)
	}
	if len(nurse.Countries) != 1 || nurse.Countries[0] != "si" {
		t.Errorf("countries = %v, want [si] from the query", nurse.Countries)
	}
	if nurse.EmploymentType != "full_time" || nurse.IsTechHint {
		t.Errorf("employment/tech = %q / %v", nurse.EmploymentType, nurse.IsTechHint)
	}
	want := time.Date(2026, 10, 29, 0, 0, 0, 0, time.FixedZone("", 3600))
	if nurse.PostedAt == nil || !nurse.PostedAt.Equal(want) {
		t.Errorf("posted = %v, want %v", nurse.PostedAt, want)
	}
	if !strings.Contains(nurse.Description, "<li>izvajanje zdravstvene nege in oskrbe stanovalcev") {
		t.Errorf("an inline dash list must render as a list: %q", nurse.Description)
	}
	if !strings.Contains(nurse.Description, "<li>Delovne izkušnje: 1 leto</li>") {
		t.Errorf("the record's facts must be listed: %q", nurse.Description)
	}
	if strings.Contains(nurse.Description, "REDACTED") || strings.Contains(nurse.Description, "example.com") {
		t.Errorf("the contact block must never be copied: %q", nurse.Description)
	}
	if nurse.SalaryMin != nil {
		t.Errorf("salary = %v, want none for an empty okvirnaPlaca", *nurse.SalaryMin)
	}

	fitter := got["3497653"]
	if fitter.SalaryMin == nil || *fitter.SalaryMin != 2200 || *fitter.SalaryMax != 2200 ||
		fitter.SalaryCurrency != "EUR" || fitter.SalaryPeriod != "month" {
		t.Errorf("salary = %v-%v %s/%s, want 2200 EUR/month", fitter.SalaryMin, fitter.SalaryMax, fitter.SalaryCurrency, fitter.SalaryPeriod)
	}

	if dev := got["3488829"]; !dev.IsTechHint || dev.Title != "PROGRAMER APLIKACIJ VII/2 (I)" {
		t.Errorf("IT slice vacancy: tech=%v title=%q", dev.IsTechHint, dev.Title)
	}
	if pa := got["3499064"]; pa.EmploymentType != "part_time" {
		t.Errorf("part-time slice vacancy: employment = %q", pa.EmploymentType)
	}
}

func TestESSRefreshesSeenAndSparesGatedVacancies(t *testing.T) {
	s, fake := newTestESS(t)
	seen := func(id string) bool { return id == "3475977" }
	gate := DetailGate{
		Rejected: func(j Job) bool { return j.ExternalID == "3499064" },
	}
	jobs, err := s.FetchNewGated(context.Background(), CompanyEntry{Provider: "ess"}, seen, gate)
	if err != nil {
		t.Fatalf("FetchNewGated: %v", err)
	}
	got := byExternalID(jobs)
	if !got["3475977"].SeenRefresh || got["3475977"].Description != "" {
		t.Errorf("a stored vacancy must be refreshed by identity: %+v", got["3475977"])
	}
	if got["3499064"].Description != "" {
		t.Errorf("a rejected vacancy must be yielded body-less")
	}
	for _, id := range fake.detCalls {
		if id == "3475977" || id == "3499064" {
			t.Errorf("detail bought for %s, which needed none", id)
		}
	}
}

func TestESSCoveredEmployerSkipsDetail(t *testing.T) {
	s, fake := newTestESS(t)
	var asked []string
	gate := DetailGate{Covered: func(companies []string) map[string]bool {
		asked = companies
		return map[string]bool{"Dom na Krasu": true}
	}}
	jobs, err := s.FetchNewGated(context.Background(), CompanyEntry{Provider: "ess"}, func(string) bool { return false }, gate)
	if err != nil {
		t.Fatalf("FetchNewGated: %v", err)
	}
	if len(asked) != 4 {
		t.Errorf("Covered asked about %v, want every listed employer", asked)
	}
	if j := byExternalID(jobs)["3475977"]; j.Description != "" || j.Company != "Dom na Krasu" {
		t.Errorf("covered vacancy = %+v, want body-less with its employer", j)
	}
	for _, id := range fake.detCalls {
		if id == "3475977" {
			t.Error("detail bought for a covered employer's vacancy")
		}
	}
}

func TestESSGoneAndFailedDetails(t *testing.T) {
	s, fake := newTestESS(t)
	fake.details["3475977"] = essFixture(t, "detail_gone.json")
	fake.detErr["3497653"] = errors.New("unexpected status 503")
	jobs, err := s.FetchNew(context.Background(), CompanyEntry{Provider: "ess"}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	got := byExternalID(jobs)
	if _, ok := got["3475977"]; ok {
		t.Error("a vacancy the API no longer holds must not be yielded")
	}
	if _, ok := got["3497653"]; ok {
		t.Error("a vacancy whose detail failed must be deferred, not yielded")
	}
	if len(got) != 2 {
		t.Errorf("got %d jobs, want the 2 readable ones", len(got))
	}
}

func TestESSNoDetailAnsweredFailsTheCrawl(t *testing.T) {
	s, fake := newTestESS(t)
	for _, id := range []string{"3475977", "3497653", "3488829", "3499064"} {
		fake.detErr[id] = errors.New("unexpected status 403")
	}
	if _, err := s.FetchNew(context.Background(), CompanyEntry{Provider: "ess"}, func(string) bool { return false }); err == nil {
		t.Fatal("want an error: a crawl that read no detail at all is broken, not quiet")
	}
}

func TestESSListingFailures(t *testing.T) {
	t.Run("a later catalogue page failing fails the crawl", func(t *testing.T) {
		s, fake := newTestESS(t)
		fake.listErr[essFakeKey("", "", 2)] = errors.New("unexpected status 500")
		if _, err := s.FetchNew(context.Background(), CompanyEntry{Provider: "ess"}, func(string) bool { return false }); err == nil {
			t.Fatal("want an error: a partial listing would let the sweep close what it did not reach")
		}
	})
	t.Run("an empty catalogue fails the crawl", func(t *testing.T) {
		s, fake := newTestESS(t)
		delete(fake.listing, essFakeKey("", "", 1))
		if _, err := s.FetchNew(context.Background(), CompanyEntry{Provider: "ess"}, func(string) bool { return false }); err == nil {
			t.Fatal("want an error: a national exchange with no vacancy is a failed measurement")
		}
	})
	t.Run("an answer without a total fails the crawl", func(t *testing.T) {
		s, fake := newTestESS(t)
		fake.listing[essFakeKey("", "", 1)] = []byte(`{"title":"One or more validation errors occurred."}`)
		if _, err := s.FetchNew(context.Background(), CompanyEntry{Provider: "ess"}, func(string) bool { return false }); err == nil {
			t.Fatal("want an error for an answer that carries no listing")
		}
	})
	t.Run("a failed slice costs only its mark", func(t *testing.T) {
		s, fake := newTestESS(t)
		fake.listErr[essFakeKey(essITField, "", 1)] = errors.New("unexpected status 500")
		fake.listErr[essFakeKey("", essPartTime, 1)] = errors.New("unexpected status 500")
		jobs, err := s.FetchNew(context.Background(), CompanyEntry{Provider: "ess"}, func(string) bool { return false })
		if err != nil {
			t.Fatalf("FetchNew: %v", err)
		}
		for _, j := range jobs {
			if j.IsTechHint || j.EmploymentType != "" {
				t.Errorf("%s: tech=%v employment=%q, want neither stated", j.ExternalID, j.IsTechHint, j.EmploymentType)
			}
		}
	})
}

func TestESSHydrationBound(t *testing.T) {
	s, fake := newTestESS(t)
	s.maxNewPerRun = 1
	jobs, err := s.FetchNew(context.Background(), CompanyEntry{Provider: "ess"}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 1 || len(fake.detCalls) != 1 || jobs[0].ExternalID != "3475977" {
		t.Errorf("got %d jobs from %v, want only the newest hydrated", len(jobs), fake.detCalls)
	}
}

func TestESSTitle(t *testing.T) {
	for in, want := range map[string]string{
		"SREDNJA MEDICINSKA SESTRA I - M/Ž":   "SREDNJA MEDICINSKA SESTRA I",
		"OSEBNI/A ASISTENT/KA - M/Ž":          "OSEBNI/A ASISTENT/KA",
		"Skladiščnik (m/ž)":                   "Skladiščnik",
		"PROGRAMER APLIKACIJ VII/2 (I) - M/Ž": "PROGRAMER APLIKACIJ VII/2 (I)",
		"VODJA PROJEKTOV":                     "VODJA PROJEKTOV",
	} {
		if got := essTitle(in); got != want {
			t.Errorf("essTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestESSPlace(t *testing.T) {
	for in, want := range map[string]string{
		"NOVA GORICA":       "Nova Gorica",
		"ŠMARJE PRI JELŠAH": "Šmarje pri Jelšah",
		"RADLJE OB DRAVI":   "Radlje ob Dravi",
		"IZOLA - ISOLA":     "Izola - Isola",
		"Domžale":           "Domžale",
	} {
		if got := essPlace(in); got != want {
			t.Errorf("essPlace(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestESSSalary(t *testing.T) {
	for in, want := range map[string]int{
		"1.500,00 EUR bruto mesečno": 1500,
		"1.711,02 EUR bruto mesečno": 1711,
		"950 EUR bruto mesečno":      950,
		"":                           0,
		"po dogovoru":                0,
		"1,00 EUR bruto mesečno":     0,
		"12.000,00 EUR bruto letno":  0,
	} {
		got, ok := essSalary(in)
		if (want == 0) == ok || got != want {
			t.Errorf("essSalary(%q) = %d, %v; want %d", in, got, ok, want)
		}
	}
}
