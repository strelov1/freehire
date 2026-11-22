package sources

import (
	"context"
	"embed"
	"errors"
	"strings"
	"sync"
	"testing"

	"golang.org/x/net/html"
)

//go:embed testdata/cvbankas
var cvbankasFixtures embed.FS

func cvbankasFixture(tb testing.TB, name string) *html.Node {
	tb.Helper()
	raw, err := cvbankasFixtures.ReadFile("testdata/cvbankas/" + name)
	if err != nil {
		tb.Fatal(err)
	}
	root, err := html.Parse(strings.NewReader(string(raw)))
	if err != nil {
		tb.Fatal(err)
	}
	return root
}

// cvbankasFake serves the live fixtures (fetched 2026-11-22: three rows of the front page and the
// Pigu posting's page with its description section). Page 1 is the listing; page 2 repeats it,
// as the site answers a page past the end with an earlier one.
type cvbankasFake struct {
	t       testing.TB
	pageErr error

	mu    sync.Mutex
	asked []string
}

const cvbankasPiguURL = "https://www.cvbankas.lt/aukstuminio-keltuvo-vairuotojas-a-vilniuje/1-14181996"

func (f *cvbankasFake) GetHTML(_ context.Context, u string) (*html.Node, error) {
	f.mu.Lock()
	f.asked = append(f.asked, u)
	f.mu.Unlock()
	switch {
	case strings.HasPrefix(u, cvbankasBase+"/?page="):
		return cvbankasFixture(f.t, "list.html"), nil
	case u == cvbankasPiguURL:
		if f.pageErr != nil {
			return nil, f.pageErr
		}
		return cvbankasFixture(f.t, "detail_14181996.html"), nil
	}
	return nil, &StatusError{Method: "GET", Code: 404, URL: u}
}

func TestCVbankasListingRows(t *testing.T) {
	rows := cvbankasRows(cvbankasFixture(t, "list.html"))
	if len(rows) != 3 {
		t.Fatalf("want 3 rows, got %d", len(rows))
	}
	r := rows[0]
	if r.id != "14181996" || r.url != cvbankasPiguURL || r.title != "Aukštuminio keltuvo vairuotojas (-a)" ||
		r.company != "UAB „Pigu“" || len(r.cities) != 1 || r.cities[0] != "Vilniuje" {
		t.Errorf("row 0: %+v", r)
	}
	if r.salary != (cvbankasSalary{min: 2150, max: 2480, period: "month"}) {
		t.Errorf("gross monthly salary: %+v", r.salary)
	}
	if rows[1].salary.period != "" {
		t.Errorf("a figure after tax must not be kept: %+v", rows[1].salary)
	}
	if rows[2].salary != (cvbankasSalary{min: 7, max: 10, period: "hour"}) {
		t.Errorf("hourly salary: %+v", rows[2].salary)
	}
}

func TestCVbankasFetchHydratesNewAndRefreshesSeen(t *testing.T) {
	f := &cvbankasFake{t: t}
	s := NewCVbankas(f).(cvbankas)
	rows := cvbankasRows(cvbankasFixture(t, "list.html"))
	stored := map[string]bool{}
	for _, r := range rows[1:] {
		stored[r.id] = true
	}
	jobs, err := s.FetchNew(context.Background(), CompanyEntry{Provider: "cvbankas"}, func(id string) bool { return stored[id] })
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 3 {
		t.Fatalf("want 3 jobs, got %d", len(jobs))
	}
	var pigu Job
	refreshed := 0
	for _, j := range jobs {
		if j.SeenRefresh {
			refreshed++
		}
		if j.ExternalID == "14181996" {
			pigu = j
		}
	}
	if refreshed != 2 {
		t.Errorf("want 2 refreshes, got %d", refreshed)
	}
	if pigu.Title != "Aukštuminio keltuvo vairuotojas (-a)" || pigu.Company != "UAB „Pigu“" {
		t.Errorf("title/company: %q / %q", pigu.Title, pigu.Company)
	}
	if !strings.Contains(pigu.Description, "aukštuminį krautuvą") {
		t.Errorf("description missing the body: %q", pigu.Description)
	}
	if pigu.Location != "Vilnius" {
		t.Errorf("location: %q", pigu.Location)
	}
	if pigu.PostedAt == nil || pigu.PostedAt.Format("2006-01-02") != "2026-11-20" {
		t.Errorf("posted: %v", pigu.PostedAt)
	}
	if pigu.SalaryMin == nil || *pigu.SalaryMin != 2150 || *pigu.SalaryMax != 2480 || pigu.SalaryPeriod != "month" {
		t.Errorf("salary: %+v", pigu)
	}
	if len(pigu.FallbackCountries) != 1 || pigu.FallbackCountries[0] != "lt" {
		t.Errorf("country: %v", pigu.FallbackCountries)
	}
	// Page 1 names new rows, page 2 repeats them: the walk ends there.
	pages := 0
	for _, u := range f.asked {
		if strings.Contains(u, "?page=") {
			pages++
		}
	}
	if pages != 2 {
		t.Errorf("want the walk to end at the first page with nothing new (2 requests), got %d", pages)
	}
}

func TestCVbankasNoAnswerFailsBoard(t *testing.T) {
	f := &cvbankasFake{t: t, pageErr: errors.New("boom")}
	s := NewCVbankas(f).(cvbankas)
	// The other rows answer 404. Those answers still count when the Pigu page is unreadable.
	jobs, err := s.Fetch(context.Background(), CompanyEntry{Provider: "cvbankas"})
	if err != nil || len(jobs) != 1 {
		t.Fatalf("got %d jobs and %v, want one unreadable marker", len(jobs), err)
	}
	if got := jobs[0]; got.ExternalID != "14181996" || got.URL != cvbankasPiguURL || !got.Unreadable {
		t.Errorf("unreadable marker = %+v", got)
	}
	s.http = errorGetter{}
	if _, err := s.list(context.Background()); err == nil {
		t.Fatal("a failed first listing page must fail the crawl")
	}
}

type errorGetter struct{}

func (errorGetter) GetHTML(context.Context, string) (*html.Node, error) {
	return nil, errors.New("down")
}

func TestCVbankasSalaryParsing(t *testing.T) {
	cases := []struct {
		amount, period, tax string
		want                cvbankasSalary
	}{
		{"Nuo 1800", "€/mėn.", "Neatskaičius mokesčių", cvbankasSalary{1800, 1800, "month"}},
		{"6,50-8", "€/val.", "Neatskaičius mokesčių", cvbankasSalary{7, 8, "hour"}},
		{"1500-1700", "€/mėn.", "Į rankas", cvbankasSalary{}},
		{"", "", "", cvbankasSalary{}},
	}
	for _, c := range cases {
		if got := cvbankasParseSalary(c.amount, c.period, c.tax); got != c.want {
			t.Errorf("%q %q %q: got %+v want %+v", c.amount, c.period, c.tax, got, c.want)
		}
	}
}
