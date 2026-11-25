package sources

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/html"
)

// The fixtures are trimmed from live python.org responses fetched 2026-10-15: the first three
// cards of each listing page verbatim, and each posting page's <article> verbatim.

// pythonorgFake serves the fixtures by URL; an unknown URL is a 404, which is also what the
// live listing answers past its last page.
type pythonorgFake struct {
	t     *testing.T
	pages map[string]string // URL → fixture name
	errs  map[string]error
	// mu guards got, which the detail workers append to concurrently.
	mu  sync.Mutex
	got []string
}

func (f *pythonorgFake) GetHTML(_ context.Context, url string) (*html.Node, error) {
	f.mu.Lock()
	f.got = append(f.got, url)
	f.mu.Unlock()
	if err, ok := f.errs[url]; ok {
		return nil, err
	}
	name, ok := f.pages[url]
	if !ok {
		return nil, &StatusError{Method: "GET", Code: 404, URL: url}
	}
	b, err := os.ReadFile("testdata/pythonorg/" + name)
	if err != nil {
		f.t.Fatal(err)
	}
	return html.Parse(strings.NewReader(string(b)))
}

func newPythonOrgFake(t *testing.T) *pythonorgFake {
	return &pythonorgFake{t: t, pages: map[string]string{
		"https://www.python.org/jobs/":        "list_page1.html",
		"https://www.python.org/jobs/?page=2": "list_page2.html",
		"https://www.python.org/jobs/8177/":   "posting_8177.html",
		"https://www.python.org/jobs/8176/":   "posting_8176.html",
		"https://www.python.org/jobs/8109/":   "posting_8109.html",
	}}
}

func pythonorgByID(jobs []Job) map[string]Job {
	out := map[string]Job{}
	for _, j := range jobs {
		out[j.ExternalID] = j
	}
	return out
}

func TestPythonOrgCardsReadIdentityFromTheListing(t *testing.T) {
	f := newPythonOrgFake(t)
	root, _ := f.GetHTML(context.Background(), "https://www.python.org/jobs/?page=2")
	cards := parsePythonOrgCards(root)
	if len(cards) != 3 {
		t.Fatalf("cards = %d, want 3: %+v", len(cards), cards)
	}
	c := cards[0]
	if c.ID != "8109" || c.URL != "https://www.python.org/jobs/8109/" ||
		c.Title != "Senior Backend Python Engineer" ||
		c.Company != "mindIT HR Agency (on behalf of a confidential client)" ||
		c.Location != "Remote, Argentina" || c.Posted != "2026-07-15T14:40:16.433072+00:00" {
		t.Errorf("first card = %+v", c)
	}
	// The "New" badge sits in the same span as the title and the employer on page 1.
	root, _ = f.GetHTML(context.Background(), "https://www.python.org/jobs/")
	if got := parsePythonOrgCards(root)[0]; got.Company != "3Gimbals" || got.Title != "Software Engineer - Full Stack" {
		t.Errorf("badged card = %+v", got)
	}
}

func TestPythonOrgFetchWalksToThe404AndHydrates(t *testing.T) {
	f := newPythonOrgFake(t)
	jobs, err := NewPythonOrg(f).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatal(err)
	}
	if want := "https://www.python.org/jobs/?page=3"; !strings.Contains(strings.Join(f.got, " "), want) {
		t.Errorf("walk never reached the page past the end: %v", f.got)
	}
	// Six listed; three posting pages are served, the other three 404 and are dropped.
	if len(jobs) != 3 {
		t.Fatalf("jobs = %d, want 3", len(jobs))
	}
	j := pythonorgByID(jobs)["8177"]
	if j.Title != "Software Engineer - Full Stack" || j.Company != "3Gimbals" ||
		j.Location != "Remote, United States" || !j.IsTechHint {
		t.Errorf("identity: %+v", j)
	}
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2026, 10, 14, 20, 52, 19, 360265000, time.UTC)) {
		t.Errorf("posted = %v", j.PostedAt)
	}
	if !strings.Contains(j.Description, "OMEN") || !strings.Contains(j.Description, "Contact Info") {
		t.Errorf("body: %.300s", j.Description)
	}
	if strings.Contains(j.Description, "Job Title") {
		t.Errorf("the repeated Job Title section was kept: %.200s", j.Description)
	}
	if j.URL != "https://www.python.org/jobs/8177/" {
		t.Errorf("url = %s", j.URL)
	}
}

func TestPythonOrgSeenPostingCostsNoDetail(t *testing.T) {
	f := newPythonOrgFake(t)
	jobs, err := NewPythonOrg(f).(HydratingSource).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 6 {
		t.Fatalf("jobs = %d, want 6", len(jobs))
	}
	for _, j := range jobs {
		if !j.SeenRefresh || j.Description != "" {
			t.Errorf("seen posting hydrated: %+v", j)
		}
	}
	for _, u := range f.got {
		if pythonorgPostingRE.MatchString(strings.TrimPrefix(u, pythonorgBase)) {
			t.Errorf("seen posting fetched: %s", u)
		}
	}
}

func TestPythonOrgFirstPageFailureFailsTheCrawl(t *testing.T) {
	f := newPythonOrgFake(t)
	f.errs = map[string]error{"https://www.python.org/jobs/": errors.New("boom")}
	if _, err := NewPythonOrg(f).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("want an error")
	}
}

func TestPythonOrgReadingNoPostingPageFailsTheCrawl(t *testing.T) {
	f := newPythonOrgFake(t)
	for _, id := range []string{"8177", "8176", "8109"} {
		delete(f.pages, "https://www.python.org/jobs/"+id+"/")
	}
	if _, err := NewPythonOrg(f).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("want an error when no new posting page could be read")
	}
}
