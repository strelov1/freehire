package sources

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"

	"golang.org/x/net/html"
)

// mynaviFake serves the recorded listing page as page 1 of o16, an empty page past it, and the
// recorded detail pages by posting URL. Anything else is a 404.
type mynaviFake struct {
	t     *testing.T
	pages map[string]string
	mu    sync.Mutex
	asked []string
}

func newMynaviFake(t *testing.T) *mynaviFake {
	t.Helper()
	read := func(name string) string {
		b, err := os.ReadFile("testdata/mynavi/" + name)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	return &mynaviFake{t: t, pages: map[string]string{
		mynaviPageURL("o16", 1):                read("list_o16.html"),
		mynaviPageURL("o16", 2):                `<html><body><div class="pager_text"></div></body></html>`,
		mynaviBase + "/jobinfo-407963-1-11-1/": read("detail_407963-1-11-1.html"),
		mynaviBase + "/jobinfo-353384-1-21-1/": read("detail_353384-1-21-1.html"),
	}}
}

func (f *mynaviFake) GetHTML(_ context.Context, u string) (*html.Node, error) {
	f.mu.Lock()
	f.asked = append(f.asked, u)
	f.mu.Unlock()
	body, ok := f.pages[u]
	if !ok {
		return nil, &StatusError{Method: "GET", Code: 404, URL: u}
	}
	return html.Parse(strings.NewReader(body))
}

func (f *mynaviFake) count(prefix string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, u := range f.asked {
		if strings.HasPrefix(u, prefix) {
			n++
		}
	}
	return n
}

func mynaviByID(jobs []Job) map[string]Job {
	m := map[string]Job{}
	for _, j := range jobs {
		m[j.ExternalID] = j
	}
	return m
}

func TestMynaviListingSkipsAgencyCardsAndReadsTheCounter(t *testing.T) {
	f := newMynaviFake(t)
	root, err := f.GetHTML(context.Background(), mynaviPageURL("o16", 1))
	if err != nil {
		t.Fatal(err)
	}
	cards := mynaviCards(root)
	// Three direct postings and one agency card that links off-site.
	if len(cards) != 3 {
		t.Fatalf("cards = %d, want 3 (the agency card skipped): %+v", len(cards), cards)
	}
	c := cards[1]
	if c.ID != "353384-1-21-1" || c.Company != "株式会社precious" || c.Title != "【動画編集】定着率95%／フルリモ・副業可／残業ほぼゼロ" {
		t.Errorf("card = %+v", c)
	}
	if n, ok := mynaviTotal(root); !ok || n != 6802 {
		t.Errorf("total = %d, %v; want 6802", n, ok)
	}
}

func TestMynaviFetchHydratesNewPostingsFromJSONLD(t *testing.T) {
	f := newMynaviFake(t)
	jobs, err := NewMynavi(f).Fetch(context.Background(), CompanyEntry{Board: "o16"})
	if err != nil {
		t.Fatal(err)
	}
	// 477766-1-1-1 has no recorded detail, so the fake answers 404: the platform's "gone".
	got := mynaviByID(jobs)
	if len(got) != 2 {
		t.Fatalf("jobs = %d, want 2: %+v", len(got), jobs)
	}
	j := got["353384-1-21-1"]
	if j.URL != "https://tenshoku.mynavi.jp/jobinfo-353384-1-21-1/" || j.Company != "株式会社precious" {
		t.Errorf("identity = %q %q", j.URL, j.Company)
	}
	if j.Title != "動画編集／WEBデザイナー" {
		t.Errorf("title = %q, want the posting's own title", j.Title)
	}
	if !j.IsTechHint || len(j.Countries) != 1 || j.Countries[0] != "jp" {
		t.Errorf("tech = %v, countries = %v", j.IsTechHint, j.Countries)
	}
	if !strings.Contains(j.Location, "埼玉県") || !strings.Contains(j.Location, "港区, 東京都") {
		t.Errorf("location = %q", j.Location)
	}
	if j.EmploymentType != "full_time" || j.SalaryMin == nil || *j.SalaryMin != 3500000 ||
		j.SalaryMax == nil || *j.SalaryMax != 8000000 || j.SalaryCurrency != "JPY" || j.SalaryPeriod != "year" {
		t.Errorf("employment/salary = %q %v %v %q %q", j.EmploymentType, j.SalaryMin, j.SalaryMax, j.SalaryCurrency, j.SalaryPeriod)
	}
	if j.PostedAt == nil || j.PostedAt.Format("2006-01-02") != "2026-10-23" {
		t.Errorf("posted = %v", j.PostedAt)
	}
	if !strings.Contains(j.Description, "応募資格") || !strings.Contains(j.Description, "定着率95％") {
		t.Errorf("description lacks its sections: %.200q", j.Description)
	}
	if strings.Contains(j.Description, "lt;br") {
		t.Errorf("description keeps the escaped breaks: %.300q", j.Description)
	}
}

func TestMynaviSeenPostingsCostNoDetailRequest(t *testing.T) {
	f := newMynaviFake(t)
	jobs, err := NewMynavi(f).(HydratingSource).FetchNew(context.Background(), CompanyEntry{Board: "o16"},
		func(string) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	if n := f.count(mynaviBase + "/jobinfo-"); n != 0 {
		t.Errorf("detail requests = %d, want 0", n)
	}
	if len(jobs) != 3 {
		t.Fatalf("jobs = %d, want 3 refreshes", len(jobs))
	}
	for _, j := range jobs {
		if !j.SeenRefresh || j.Description != "" {
			t.Errorf("job %s: refresh = %v, body = %q", j.ExternalID, j.SeenRefresh, j.Description)
		}
	}
}

func TestMynaviGatedPostingsAreYieldedBodyless(t *testing.T) {
	f := newMynaviFake(t)
	gate := DetailGate{Rejected: func(j Job) bool { return true }}
	jobs, err := NewMynavi(f).(CoverageGated).FetchNewGated(context.Background(), CompanyEntry{Board: "o16"},
		func(string) bool { return false }, gate)
	if err != nil {
		t.Fatal(err)
	}
	if n := f.count(mynaviBase + "/jobinfo-"); n != 0 {
		t.Errorf("detail requests = %d, want 0", n)
	}
	if len(jobs) != 3 {
		t.Errorf("jobs = %d, want 3 body-less postings for the pipeline to count", len(jobs))
	}
}

func TestMynaviRejectsAnUnknownBoard(t *testing.T) {
	if _, err := NewMynavi(newMynaviFake(t)).Fetch(context.Background(), CompanyEntry{Board: "o11"}); err == nil {
		t.Error("want an error for a code the engineer edition does not serve")
	}
}

func TestMynaviFirstPageFailingFailsTheBoard(t *testing.T) {
	f := newMynaviFake(t)
	delete(f.pages, mynaviPageURL("o16", 1))
	if _, err := NewMynavi(f).Fetch(context.Background(), CompanyEntry{Board: "o16"}); err == nil {
		t.Error("want the board to fail when its first page cannot be read")
	}
}

func TestMynaviPageWithoutCounterIsNotAResultsPage(t *testing.T) {
	f := newMynaviFake(t)
	f.pages[mynaviPageURL("o16", 1)] = strings.Replace(f.pages[mynaviPageURL("o16", 1)], "pager_text", "renamed", 1)
	if _, err := NewMynavi(f).Fetch(context.Background(), CompanyEntry{Board: "o16"}); err == nil {
		t.Error("want an error for a page with cards but no result counter")
	}
}

func TestMynaviAllDetailsFailingFailsTheBoard(t *testing.T) {
	f := newMynaviFake(t)
	f.pages[mynaviBase+"/jobinfo-477766-1-1-1/"] = ""
	for u := range f.pages {
		if strings.Contains(u, "/jobinfo-") {
			f.pages[u] = "<html><body>maintenance</body></html>"
		}
	}
	if _, err := NewMynavi(f).Fetch(context.Background(), CompanyEntry{Board: "o16"}); err == nil {
		t.Error("want an error when no requested detail answered")
	}
}
