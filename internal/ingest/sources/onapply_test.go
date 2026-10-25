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

// onapplyFixture reads hits captured verbatim from the job-board API (2026-10-25), trimmed to
// three postings with the total rewritten to match; only each recruiter's contact section was
// replaced with a placeholder.
func onapplyFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/onapply/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}

// sequencedJSON answers each URL with its queued bodies in turn, repeating the last, so a test
// can serve a page that changes between walks — the unstable ordering onapply's index shows. An
// empty body is answered with a 502.
type sequencedJSON struct {
	mu     sync.Mutex
	bodies map[string][]string
	errs   map[string]error
	calls  int
}

func (f *sequencedJSON) GetJSON(_ context.Context, url string, v any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	for match, err := range f.errs {
		if strings.Contains(url, match) {
			return err
		}
	}
	for match, queue := range f.bodies {
		if !strings.Contains(url, match) {
			continue
		}
		body := queue[0]
		if len(queue) > 1 {
			f.bodies[match] = queue[1:]
		}
		if body == "" {
			return &StatusError{Code: 502}
		}
		return json.Unmarshal([]byte(body), v)
	}
	return &StatusError{Code: 404}
}

func newTestOnapply(c JSONGetter) onapply {
	s := NewOnapply(c).(onapply)
	s.pageSize = 2
	return s
}

func TestOnapplyProvider(t *testing.T) {
	s, ok := All(nil)["onapply"]
	if !ok || s.Provider() != "onapply" {
		t.Fatal("All() must register onapply")
	}
	if _, ok := s.(boardless); !ok {
		t.Error("onapply must be boardless: the index spans every tenant")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("onapply must be an aggregator: the employer is read per posting")
	}
}

func TestOnapplyWalksTheIndexAndMapsPostings(t *testing.T) {
	fake := &sequencedJSON{bodies: map[string][]string{
		"/search/0/2?": {onapplyFixture(t, "page_0.json")},
		"/search/2/2?": {onapplyFixture(t, "page_2.json")},
	}}
	jobs, err := newTestOnapply(fake).Fetch(context.Background(), CompanyEntry{Provider: "onapply"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 3 || fake.calls != 2 {
		t.Fatalf("jobs/requests = %d/%d, want 3 postings from one walk of two pages", len(jobs), fake.calls)
	}
	j := jobs[0]
	if j.ExternalID != "276897" || j.URL != "https://stotax.onapply.de/details/276897.html" ||
		j.Company != "STOTaX GmbH & Co. KG" || !strings.HasPrefix(j.Title, "Software Developer") {
		t.Errorf("identity = %q %q %q %q", j.ExternalID, j.URL, j.Company, j.Title)
	}
	if j.PostedAt == nil || j.PostedAt.UTC().Format("2006-01-02 15:04") != "2024-09-04 04:03" {
		t.Errorf("PostedAt = %v, want 06:03 Berlin summer time", j.PostedAt)
	}
	if len(j.Countries) != 1 || j.Countries[0] != "de" {
		t.Errorf("Countries = %v, want [de]", j.Countries)
	}
	for _, want := range []string{"<h3>Über uns</h3>", "<h3>Deine interessanten Aufgaben</h3>", "C#", "<h3>Deine Vorteile</h3>"} {
		if !strings.Contains(j.Description, want) {
			t.Errorf("Description lacks %q", want)
		}
	}
	if strings.Contains(j.Description, "Mustermann") {
		t.Error("Description must leave out the recruiter's contact section")
	}
	if j.EmploymentType != "" {
		t.Errorf("EmploymentType = %q, want none for Vollzeit + Teilzeit", j.EmploymentType)
	}

	multi := jobs[1]
	if got := strings.Count(multi.Location, ";"); got != 4 {
		t.Errorf("Location = %q, want the first five distinct places", multi.Location)
	}
	if strings.Contains(multi.Location, "Niedersachsen") {
		t.Errorf("Location = %q: a posting across two regions names neither", multi.Location)
	}
	if jobs[2].EmploymentType != "internship" {
		t.Errorf("EmploymentType = %q, want internship for Praktikum", jobs[2].EmploymentType)
	}
}

// The index sends no sort, so a later page can repeat an earlier posting and skip another. A
// walk short of the stated total walks again, and the union is the crawl.
func TestOnapplyWalksAgainWhileShortOfTheTotal(t *testing.T) {
	fake := &sequencedJSON{bodies: map[string][]string{
		"/search/0/2?": {onapplyFixture(t, "page_0.json")},
		"/search/2/2?": {onapplyFixture(t, "page_2_repeat.json"), onapplyFixture(t, "page_2.json")},
	}}
	jobs, err := newTestOnapply(fake).Fetch(context.Background(), CompanyEntry{Provider: "onapply"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 3 || fake.calls != 4 {
		t.Fatalf("jobs/requests = %d/%d, want 3 distinct postings from two walks", len(jobs), fake.calls)
	}
}

// A walk that never reaches the total stops at maxPasses with what the walks found.
func TestOnapplyStopsAfterMaxPasses(t *testing.T) {
	fake := &sequencedJSON{bodies: map[string][]string{
		"/search/0/2?": {onapplyFixture(t, "page_0.json")},
		"/search/2/2?": {onapplyFixture(t, "page_2_repeat.json")},
	}}
	jobs, err := newTestOnapply(fake).Fetch(context.Background(), CompanyEntry{Provider: "onapply"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 || fake.calls != 2*onapplyMaxPasses {
		t.Fatalf("jobs/requests = %d/%d, want 2 postings after %d walks", len(jobs), fake.calls, onapplyMaxPasses)
	}
}

// A repeat walk whose first page fails states no total; the run keeps walking toward the first
// walk's total rather than reading the failure as "nothing left to find".
func TestOnapplyRepeatWalkFailureKeepsTheTotal(t *testing.T) {
	fake := &sequencedJSON{bodies: map[string][]string{
		"/search/0/2?": {onapplyFixture(t, "page_0.json"), "", onapplyFixture(t, "page_0.json")},
		"/search/2/2?": {onapplyFixture(t, "page_2_repeat.json"), onapplyFixture(t, "page_2.json")},
	}}
	jobs, err := newTestOnapply(fake).Fetch(context.Background(), CompanyEntry{Provider: "onapply"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("jobs = %d, want all 3 after a third walk", len(jobs))
	}
}

func TestOnapplyFirstPageFailureFailsTheCrawl(t *testing.T) {
	fake := &sequencedJSON{errs: map[string]error{"/search/0/": errors.New("boom")}}
	if _, err := newTestOnapply(fake).Fetch(context.Background(), CompanyEntry{Provider: "onapply"}); err == nil {
		t.Fatal("a failed first page must fail the crawl")
	}
}

func TestOnapplyLaterPageFailureKeepsWhatWasRead(t *testing.T) {
	fake := &sequencedJSON{
		bodies: map[string][]string{"/search/0/2?": {onapplyFixture(t, "page_0.json")}},
		errs:   map[string]error{"/search/2/": errors.New("boom")},
	}
	jobs, err := newTestOnapply(fake).Fetch(context.Background(), CompanyEntry{Provider: "onapply"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("jobs = %d, want the first page's 2", len(jobs))
	}
}

func TestOnapplyPostingURL(t *testing.T) {
	for raw, want := range map[string]string{
		"https://duvenbeck.onapply.de/details/12845.html?source=careerSiteWidget": "https://duvenbeck.onapply.de/details/12845.html",
		"http://duvenbeck.onapply.de/details/12845.html":                          "",
		"https://evil.example/details/12845.html":                                 "",
		"": "",
	} {
		if got := onapplyPostingURL(raw); got != want {
			t.Errorf("onapplyPostingURL(%q) = %q, want %q", raw, got, want)
		}
	}
}
