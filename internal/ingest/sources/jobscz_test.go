package sources

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

// jobsczFixtures holds pages fetched from www.jobs.cz on 2026-11-22, trimmed to a few postings:
// a listing page (a partner card, a card with pay and home-office tags, a bare card) and three
// posting pages (a general one, an IT one and the shell of a partner career site).
//
//go:embed testdata/jobscz
var jobsczFixtures embed.FS

func jobsczFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := jobsczFixtures.ReadFile("testdata/jobscz/" + name)
	if err != nil {
		t.Fatalf("fixture %s: %v", name, err)
	}
	return string(b)
}

// jobsczFake serves pages by URL; a URL it does not know answers 404. gets records every URL
// asked and the cookie it carried.
type jobsczFake struct {
	pages map[string]string
	errs  map[string]error

	mu      sync.Mutex
	gets    []string
	cookies []string
}

func (f *jobsczFake) GetTextWithHeaders(_ context.Context, u string, headers map[string]string) (string, error) {
	f.mu.Lock()
	f.gets = append(f.gets, u)
	f.cookies = append(f.cookies, headers["Cookie"])
	f.mu.Unlock()
	if err := f.errs[u]; err != nil {
		return "", err
	}
	if p, ok := f.pages[u]; ok {
		return p, nil
	}
	return "", &StatusError{Code: http.StatusNotFound}
}

func (f *jobsczFake) asked(sub string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, u := range f.gets {
		if strings.Contains(u, sub) {
			n++
		}
	}
	return n
}

const (
	jobsczEmptyListing = `<html><body><p>Nic jsme nenašli</p></body></html>`
	// The ids of the three cards in testdata/jobscz/listing.html.
	jobsczPartnerID = "2000215518"
	jobsczOwnID     = "2001522931"
	jobsczBareID    = "2001535953"
)

// jobsczWorld is a site whose only populated field is the first IT one, listing the fixture page
// as a field of three postings.
func jobsczWorld(t *testing.T) *jobsczFake {
	t.Helper()
	pages := map[string]string{
		jobsczQuery("", jobsczITFields[0], 1): strings.Replace(jobsczFixture(t, "listing.html"), "14\u00a0971", "3", 1),
		jobsczPostingURL(jobsczOwnID):         jobsczFixture(t, "detail_own.html"),
	}
	for _, f := range jobsczFields {
		if q := jobsczQuery("", f, 1); pages[q] == "" {
			pages[q] = jobsczEmptyListing
		}
	}
	return &jobsczFake{pages: pages, errs: map[string]error{jobsczPostingURL(jobsczBareID): errJobsCzPartner}}
}

func TestJobsCzListingCards(t *testing.T) {
	page := jobsczFixture(t, "listing.html")
	if n, ok := jobsczTotal(page); !ok || n != 14971 {
		t.Fatalf("total = %d, %v; want 14971, true", n, ok)
	}
	if _, ok := jobsczTotal(jobsczEmptyListing); ok {
		t.Error("an empty search states a count")
	}
	cards := jobsczCards(page)
	if len(cards) != 3 {
		t.Fatalf("cards = %d, want 3", len(cards))
	}
	if !cards[0].Partner || cards[0].ID != jobsczPartnerID || cards[0].Company != "Partner T-Mobile Czech Republic a.s." || cards[0].Location != "Liberec" {
		t.Errorf("partner card = %+v", cards[0])
	}
	own := cards[1]
	if own.Partner || own.ID != jobsczOwnID || own.Title != "Marketér s analytickou hlavou pro celý marketing Werelda" ||
		own.Company != "Wereldo.com, s.r.o." || own.Location != "Brno – Medlánky" ||
		len(own.Tags) != 1 || own.Tags[0] != "Možnost občasné práce z domova" {
		t.Errorf("own card = %+v", own)
	}
	if cards[2].Partner || len(cards[2].Tags) != 0 {
		t.Errorf("bare card = %+v", cards[2])
	}
}

func TestJobsCzDetailToJob(t *testing.T) {
	d, ok := jobsczParseDetail(jobsczFixture(t, "detail_own.html"))
	if !ok {
		t.Fatal("the posting page was not read")
	}
	card := jobsczItem{ID: jobsczOwnID, Location: "Brno – Medlánky", Tags: []string{"Možnost občasné práce z domova"}}
	job, ok := d.toJob(card)
	if !ok {
		t.Fatal("no job")
	}
	if job.Title != "Marketér s analytickou hlavou pro celý marketing Werelda" || job.Company != "Wereldo.com, s.r.o." ||
		job.URL != "https://www.jobs.cz/rpd/2001522931/" || job.Location != "Brno – Medlánky" {
		t.Errorf("job = %+v", job)
	}
	if !strings.Contains(job.Description, "Co bude tvůj cíl?") || strings.Contains(job.Description, "Odpovědět") {
		t.Errorf("description is not the body alone: %.200s", job.Description)
	}
	if job.SalaryMin == nil || *job.SalaryMin != 50000 || *job.SalaryMax != 85000 || job.SalaryCurrency != "CZK" || job.SalaryPeriod != "month" {
		t.Errorf("salary = %v-%v %s/%s", job.SalaryMin, job.SalaryMax, job.SalaryCurrency, job.SalaryPeriod)
	}
	if job.WorkMode != "hybrid" || job.Remote {
		t.Errorf("work mode = %q remote %v", job.WorkMode, job.Remote)
	}
	if job.EmploymentType != "" {
		t.Errorf("a posting stating full and part time got %q", job.EmploymentType)
	}
	if job.IsTechHint {
		t.Error("a Marketing posting is hinted technical")
	}
	if len(job.FallbackCountries) != 1 || job.FallbackCountries[0] != "cz" {
		t.Errorf("fallback countries = %v", job.FallbackCountries)
	}
}

func TestJobsCzITFieldIsHinted(t *testing.T) {
	d, ok := jobsczParseDetail(jobsczFixture(t, "detail_it.html"))
	if !ok {
		t.Fatal("the posting page was not read")
	}
	job, ok := d.toJob(jobsczItem{ID: "2001445127"})
	if !ok {
		t.Fatal("no job")
	}
	if !job.IsTechHint {
		t.Error("a posting filed under an IS/IT field is not hinted technical")
	}
	if job.EmploymentType != "full_time" {
		t.Errorf("employment type = %q, want full_time", job.EmploymentType)
	}
	if job.Location == "" {
		t.Error("the page's address is not the fallback location")
	}
}

func TestJobsCzPartnerShellIsNotAPosting(t *testing.T) {
	if _, ok := jobsczParseDetail(jobsczFixture(t, "detail_partner.html")); ok {
		t.Error("the partner shell was read as a posting")
	}
}

func TestJobsCzFetchNew(t *testing.T) {
	f := jobsczWorld(t)
	seen := func(id string) bool { return id == jobsczBareID }
	jobs, err := jobscz{http: f}.FetchNew(context.Background(), CompanyEntry{}, seen)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	if len(byID) != 2 {
		t.Fatalf("jobs = %d (%v), want the new posting and the seen one", len(byID), byID)
	}
	if j := byID[jobsczOwnID]; j.Description == "" || j.SeenRefresh {
		t.Errorf("the new posting was not hydrated: %+v", j)
	}
	if j := byID[jobsczBareID]; !j.SeenRefresh || j.Description != "" || j.Title == "" {
		t.Errorf("the seen posting is not a liveness refresh: %+v", j)
	}
	if f.asked("/rpd/"+jobsczPartnerID) != 0 {
		t.Error("a partner card cost a request")
	}
	if f.asked("/rpd/"+jobsczBareID) != 0 {
		t.Error("a seen posting cost a request")
	}
	for i, c := range f.cookies {
		if c != jobsczHeaders["Cookie"] {
			t.Fatalf("request %s carried cookie %q", f.gets[i], c)
		}
	}
}

func TestJobsCzRedirectKeepsRequestsOnTheMeasuredHost(t *testing.T) {
	ctx := context.Background()
	posting, err := http.NewRequestWithContext(ctx, http.MethodGet, jobsczPostingURL(jobsczBareID), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		target string
		want   error
	}{
		{"https://t-mobile.jobs.cz/pd/" + jobsczBareID + "/", errJobsCzPartner},
		{"https://t-mobile.jobs.cz/pd/" + jobsczOwnID + "/", http.ErrUseLastResponse},
		{"https://beta.jobs.cz/rpd/" + jobsczBareID + "/", http.ErrUseLastResponse},
		{"https://other.example/pd/" + jobsczBareID + "/", http.ErrUseLastResponse},
	} {
		redirect, err := http.NewRequestWithContext(ctx, http.MethodGet, tc.target, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := jobsCzClient().httpClient.CheckRedirect(redirect, []*http.Request{posting}); !errors.Is(err, tc.want) {
			t.Errorf("redirect to %s returned %v, want %v", tc.target, err, tc.want)
		}
	}
}

func TestJobsCzFetchDropsAPartnerPageFoundOnProbe(t *testing.T) {
	f := jobsczWorld(t)
	jobs, err := jobscz{http: f}.FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	// The bare card resolves to a confirmed partner redirect and is probed once.
	if f.asked("/rpd/"+jobsczBareID) != 1 {
		t.Errorf("probes of the partner page = %d, want 1", f.asked("/rpd/"+jobsczBareID))
	}
	if len(jobs) != 1 || jobs[0].ExternalID != jobsczOwnID {
		t.Errorf("jobs = %+v, want only the readable posting", jobs)
	}
}

func TestJobsCzOversizedFieldIsSplitByRegion(t *testing.T) {
	field := jobsczITFields[0]
	big := strings.Replace(jobsczFixture(t, "listing.html"), "14 971", "2 000", 1)
	pages := map[string]string{jobsczQuery("", field, 1): big}
	for _, region := range jobsczRegions {
		// A region holding a single page of the fixture.
		pages[jobsczQuery(region, field, 1)] = strings.Replace(big, "2 000", "3", 1)
	}
	for _, f := range jobsczFields {
		if q := jobsczQuery("", f, 1); pages[q] == "" {
			pages[q] = jobsczEmptyListing
		}
	}
	f := &jobsczFake{pages: pages}
	items, err := jobscz{http: f}.list(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Errorf("items = %d, want the three cards once across %d regions", len(items), len(jobsczRegions))
	}
	for _, region := range jobsczRegions {
		if f.asked("/prace/"+region+"/") != 1 {
			t.Errorf("region %s asked %d times, want 1", region, f.asked("/prace/"+region+"/"))
		}
	}
	// The unsplit field is not walked past its first page.
	if f.asked("field[0]="+field+"&page=2") != 0 {
		t.Error("the oversized field was walked whole")
	}
}

func TestJobsCzEmptyListingFails(t *testing.T) {
	f := &jobsczFake{pages: map[string]string{}}
	for _, fld := range jobsczFields {
		f.pages[jobsczQuery("", fld, 1)] = jobsczEmptyListing
	}
	if _, err := (jobscz{http: f}).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Error("a listing that read nothing succeeded")
	}
}

func TestJobsCzFailedPageFailsTheCrawl(t *testing.T) {
	f := jobsczWorld(t)
	delete(f.pages, jobsczQuery("", jobsczFields[3], 1))
	if _, err := (jobscz{http: f}).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Error("a listing page that failed did not fail the crawl")
	}
}

func TestJobsCzPartialDetailOutagePreservesIdentity(t *testing.T) {
	f := jobsczWorld(t)
	f.errs[jobsczPostingURL(jobsczBareID)] = errors.New("unavailable")
	jobs, err := (jobscz{http: f}).Fetch(context.Background(), CompanyEntry{})
	if err != nil || len(jobs) != 2 {
		t.Fatalf("got %d jobs and %v, want a posting and an unreadable marker", len(jobs), err)
	}
	for _, job := range jobs {
		if job.ExternalID == jobsczBareID && job.Unreadable && job.URL == jobsczPostingURL(jobsczBareID) {
			return
		}
	}
	t.Fatalf("listing identity missing from %+v", jobs)
}

func TestJobsCzCrawlThatReadNothingFails(t *testing.T) {
	f := jobsczWorld(t)
	f.errs[jobsczPostingURL(jobsczOwnID)] = errors.New("unavailable")
	f.errs[jobsczPostingURL(jobsczBareID)] = errors.New("unavailable")
	s := jobscz{http: f}
	if jobs, err := s.FetchNew(context.Background(), CompanyEntry{}, func(id string) bool { return id == jobsczBareID }); err == nil {
		t.Fatalf("an entirely unreadable detail batch reported %d jobs", len(jobs))
	}
}

func TestJobsCzConfirmedAbsencesCountAsAnswers(t *testing.T) {
	f := jobsczWorld(t)
	delete(f.pages, jobsczPostingURL(jobsczOwnID))
	jobs, err := (jobscz{http: f}).Fetch(context.Background(), CompanyEntry{})
	if err != nil || len(jobs) != 0 {
		t.Fatalf("got %d jobs and %v for a gone posting and a partner redirect", len(jobs), err)
	}
}

func TestJobsCzChooseFreshRotates(t *testing.T) {
	var fresh []jobsczItem
	for i := range 10 {
		fresh = append(fresh, jobsczItem{ID: fmt.Sprintf("20015000%02d", i)})
	}
	day := func(d int) jobscz {
		return jobscz{now: func() time.Time { return time.Date(2026, 1, d, 0, 0, 0, 0, time.UTC) }}
	}
	chosen, deferred := day(1).chooseFresh(append([]jobsczItem{}, fresh...), 4)
	if len(chosen) != 4 || deferred != 6 {
		t.Fatalf("chosen %d deferred %d, want 4 and 6", len(chosen), deferred)
	}
	if chosen[0].ID != "2001500009" || chosen[1].ID != "2001500008" {
		t.Errorf("the newest are not first: %v", chosen)
	}
	other, _ := day(2).chooseFresh(append([]jobsczItem{}, fresh...), 4)
	if chosen[2].ID == other[2].ID && chosen[3].ID == other[3].ID {
		t.Error("the rotating window did not move with the day")
	}
	if all, d := day(1).chooseFresh(fresh, 20); len(all) != 10 || d != 0 {
		t.Errorf("under budget: chosen %d deferred %d", len(all), d)
	}
}

func TestJobsCzSalary(t *testing.T) {
	cases := []struct {
		in       string
		lo, hi   int
		cur, per string
	}{
		{"50 000 \u200d–\u200d 85 000 Kč", 50000, 85000, "CZK", "month"},
		{"34 200 Kč", 34200, 34200, "CZK", "month"},
		{"250 – 300 Kč/hod", 250, 300, "CZK", "hour"},
		{"3 000 – 4 000 EUR", 3000, 4000, "EUR", "month"},
		{"", 0, 0, "", ""},
		{"dohodou", 0, 0, "", ""},
		{"50 000 USD", 0, 0, "", ""},
	}
	for _, c := range cases {
		lo, hi, cur, per := jobsczSalary(c.in)
		got := [...]any{deref(lo), deref(hi), cur, per}
		want := [...]any{jobsczWantInt(c.lo), jobsczWantInt(c.hi), c.cur, c.per}
		if got != want {
			t.Errorf("jobsczSalary(%q) = %v, want %v", c.in, got, want)
		}
	}
}

// jobsczWantInt is the expected pay figure as deref reports it: nil when none is stated.
func jobsczWantInt(n int) any {
	if n == 0 {
		return nil
	}
	return n
}

func TestJobsCzEmploymentTypeAndWorkMode(t *testing.T) {
	for in, want := range map[string]string{
		"Práce na plný úvazek":                           "full_time",
		"Práce na zkrácený úvazek":                       "part_time",
		"Práce na plný úvazek, práce na zkrácený úvazek": "",
		"Práce na dohodu (DPP/DPČ)":                      "contract",
		"Práce na živnostenský list":                     "contract",
		"Stáž, praxe a trainee programy":                 "internship",
		"":                                               "",
	} {
		if got := jobsczEmploymentType([]string{in}); got != want {
			t.Errorf("jobsczEmploymentType(%q) = %q, want %q", in, got, want)
		}
	}
	for _, c := range []struct {
		tags []string
		want string
	}{
		{[]string{"Možnost občasné práce z domova"}, "hybrid"},
		{[]string{"Možnost práce z domova", "Práce převážně z domova"}, "remote"},
		{[]string{"Flexibilní pracovní doba"}, ""},
	} {
		if got := jobsczWorkMode(c.tags); got != c.want {
			t.Errorf("jobsczWorkMode(%v) = %q, want %q", c.tags, got, c.want)
		}
	}
}

func TestJobsCzQuery(t *testing.T) {
	for _, c := range []struct {
		region, field string
		page          int
		want          string
	}{
		{"", "", 2, "https://www.jobs.cz/prace/?page=2"},
		{"", "200900011", 1, "https://www.jobs.cz/prace/?field[0]=200900011&page=1"},
		{"praha", "200900026", 3, "https://www.jobs.cz/prace/praha/?field[0]=200900026&locality[radius]=0&page=3"},
		{"zahranici", "200900001", 1, "https://www.jobs.cz/prace/zahranici/?field[0]=200900001&page=1"},
	} {
		if got := jobsczQuery(c.region, c.field, c.page); got != c.want {
			t.Errorf("jobsczQuery(%q, %q, %d) = %s, want %s", c.region, c.field, c.page, got, c.want)
		}
	}
}

func TestJobsCzIsABoardlessAggregator(t *testing.T) {
	s := NewJobsCz(nil)
	if _, ok := s.(interface{ Boardless() }); !ok {
		t.Error("jobscz is not boardless")
	}
	if _, ok := s.(interface{ Aggregator() }); !ok {
		t.Error("jobscz is not an aggregator")
	}
	if s.Provider() != "jobscz" {
		t.Errorf("provider = %q", s.Provider())
	}
}
