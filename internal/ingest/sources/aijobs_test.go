package sources

import (
	"context"
	"fmt"
	neturl "net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/html"
)

// aijobsListingFixture is a trimmed real listing-page fragment (captured live): two job
// cards plus a company-profile link and a skill-filter link, neither of which is a job
// posting, to guard against over-matching.
const aijobsListingFixture = `<html><body><ul>
<li><a class="font-monospace fw-bold stretched-link" href="/job/data-specialist-petah-tikva-center-district-il-268449/">Data Specialist</a></li>
<li><a class="font-monospace fw-bold stretched-link" href="/job/lead-ai-engineer-tel-aviv-yafo-tel-aviv-district-il-268513/">Lead AI Engineer</a></li>
<li><a href="/company/medison-pharma-16767/">Medison Pharma</a></li>
<li><a href="/jobs/skill-python/">Python</a></li>
</ul></body></html>`

func TestAijobsListingLinksMatchesOnlyJobPostings(t *testing.T) {
	root, err := html.Parse(strings.NewReader(aijobsListingFixture))
	if err != nil {
		t.Fatalf("html.Parse: %v", err)
	}
	got := aijobsListingLinks(root)
	want := []string{
		"/job/data-specialist-petah-tikva-center-district-il-268449/",
		"/job/lead-ai-engineer-tel-aviv-yafo-tel-aviv-district-il-268513/",
	}
	if !slices.Equal(got, want) {
		t.Errorf("aijobsListingLinks = %v, want %v", got, want)
	}
}

// aijobsListingPage renders a listing page whose only content is one job card per id.
func aijobsListingPage(ids ...string) string {
	var b strings.Builder
	b.WriteString("<html><body><ul>")
	for _, id := range ids {
		b.WriteString(`<li><a href="/job/role-` + id + `/">role</a></li>`)
	}
	b.WriteString("</ul></body></html>")
	return b.String()
}

func seenSet(ids ...string) func(string) bool {
	set := make(map[string]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	return func(id string) bool { return set[id] }
}

// aijobsPagedFake serves a fixed listing body per page number (matched on the "page=N"
// query parameter) and a plain session-bootstrap GET, so a test can shape exactly what
// crawlListing discovers per page without a real HTTP round trip.
func aijobsPagedFake(pages map[int]string) aijobsGetPostFake {
	return aijobsGetPostFake{postForm: func(url string) (*html.Node, error) {
		for page, body := range pages {
			if strings.Contains(url, fmt.Sprintf("page=%d", page)) {
				return html.Parse(strings.NewReader(body))
			}
		}
		return nil, fmt.Errorf("aijobsPagedFake: no route for %s", url)
	}}
}

func TestAijobsCrawlListingStopsWhenAPageIsFullySeen(t *testing.T) {
	fake := aijobsPagedFake(map[int]string{
		1: aijobsListingPage("1", "2"),
		2: aijobsListingPage("3"),
		3: aijobsListingPage("4", "5"), // already-seen page: caught up
		4: aijobsListingPage("99"),     // poison: must never be fetched
	})

	unseen, refreshed, err := aijobs{http: fake}.crawlListing(context.Background(), seenSet("4", "5"), 0)
	if err != nil {
		t.Fatalf("crawlListing: %v", err)
	}
	want := []string{"/job/role-1/", "/job/role-2/", "/job/role-3/"}
	if !slices.Equal(unseen, want) {
		t.Errorf("unseen = %v, want %v", unseen, want)
	}
	// The page that stopped the walk is still-live postings, not garbage: both of its ids
	// must come back as a liveness refresh, or a still-open aijobs.net posting would never
	// have its last_seen_at advanced past its first crawl.
	gotIDs := make([]string, len(refreshed))
	for i, j := range refreshed {
		gotIDs[i] = j.ExternalID
		if !j.SeenRefresh {
			t.Errorf("refreshed[%d].SeenRefresh = false, want true", i)
		}
	}
	if wantIDs := []string{"4", "5"}; !slices.Equal(gotIDs, wantIDs) {
		t.Errorf("refreshed ids = %v, want %v", gotIDs, wantIDs)
	}
}

func TestAijobsCrawlListingStopsAtNewBudget(t *testing.T) {
	fake := aijobsPagedFake(map[int]string{
		1: aijobsListingPage("1", "2", "3"),
		2: aijobsListingPage("99"), // poison: must never be fetched
	})

	unseen, _, err := aijobs{http: fake}.crawlListing(context.Background(), seenSet(), 2)
	if err != nil {
		t.Fatalf("crawlListing: %v", err)
	}
	want := []string{"/job/role-1/", "/job/role-2/"}
	if !slices.Equal(unseen, want) {
		t.Errorf("unseen = %v, want %v (budget-capped)", unseen, want)
	}
}

// aijobsGetPostFake lets a test control the GET (session bootstrap) and POST (listing
// page) paths independently. routedHTTP can't: its routes match by URL substring only,
// and the bootstrap GET's URL is always a prefix of every paginated POST's URL (both hit
// the same "https://aijobs.net/[...]" host), so a route meant to fail only the POST would
// also swallow the GET, or vice versa.
type aijobsGetPostFake struct {
	postForm func(url string) (*html.Node, error)
	// getHTML overrides GetHTML's response (detail-page tests); nil means the empty page
	// every pagination test relies on for the session-bootstrap GET.
	getHTML func(url string) (*html.Node, error)
}

func (f aijobsGetPostFake) GetHTML(_ context.Context, url string) (*html.Node, error) {
	if f.getHTML == nil {
		return html.Parse(strings.NewReader("<html></html>"))
	}
	return f.getHTML(url)
}

func (f aijobsGetPostFake) PostFormWithHeaders(_ context.Context, url string, _ map[string]string, _ neturl.Values) (*html.Node, error) {
	return f.postForm(url)
}

func (f aijobsGetPostFake) CookieValue(string, string) string { return "test-csrf-token" }

func TestAijobsCrawlListingFirstPageFailureErrors(t *testing.T) {
	fake := aijobsGetPostFake{postForm: func(url string) (*html.Node, error) {
		return nil, fmt.Errorf("no route for %s", url) // bootstrap succeeds, page 1 fails
	}}
	if _, _, err := (aijobs{http: fake}).crawlListing(context.Background(), seenSet(), 0); err == nil {
		t.Error("expected an error when the first listing page fails, got nil")
	}
}

// TestAijobsCrawlListingStopsAtHardPageCap covers the "hard cap regardless of seen state"
// spec scenario: a feed that never runs dry and never repeats an already-seen posting
// (so the seen-based stop never triggers) must still stop at aijobsMaxPages. It shrinks
// the package var for the duration of the test rather than paginating to the real ~1200.
func TestAijobsCrawlListingStopsAtHardPageCap(t *testing.T) {
	orig := aijobsMaxPages
	aijobsMaxPages = 3
	defer func() { aijobsMaxPages = orig }()

	var requests int
	fake := aijobsGetPostFake{postForm: func(string) (*html.Node, error) {
		requests++
		// A fresh, never-seen numeric id every call (aijobsJobIDPattern requires a
		// trailing digit run): the seen-based stop can never trigger, so only the hard
		// cap can end the walk.
		return html.Parse(strings.NewReader(aijobsListingPage(fmt.Sprintf("%d", 1000+requests))))
	}}

	unseen, _, err := aijobs{http: fake}.crawlListing(context.Background(), seenSet(), 0)
	if err != nil {
		t.Fatalf("crawlListing: %v", err)
	}
	if requests != aijobsMaxPages {
		t.Errorf("requested %d pages, want exactly %d (stop at the hard cap)", requests, aijobsMaxPages)
	}
	if len(unseen) != aijobsMaxPages {
		t.Errorf("got %d unseen postings, want %d (one per page up to the cap)", len(unseen), aijobsMaxPages)
	}
}

func TestAijobsCrawlListingLaterPageFailureKeepsWhatWasGathered(t *testing.T) {
	fake := aijobsPagedFake(map[int]string{1: aijobsListingPage("1")}) // no page=2: page 2 fails

	unseen, _, err := aijobs{http: fake}.crawlListing(context.Background(), seenSet(), 0)
	if err != nil {
		t.Fatalf("crawlListing: %v", err)
	}
	if want := []string{"/job/role-1/"}; !slices.Equal(unseen, want) {
		t.Errorf("unseen = %v, want %v", unseen, want)
	}
}

// aijobsDetailFixture is a trimmed real detail-page fragment (captured live), including
// the "@ M..." masked company name (the page's own visible label is paywalled — the
// adapter must derive the name from the /company/ href instead, never that text) and the
// site's own structured Tasks/Perks/Skills sections in place of free-text prose.
const aijobsDetailFixture = `<html><body><main>
<h1 class="font-monospace fs-2 py-3">Data Specialist</h1>
<div class="row g-3 align-items-start flex-nowrap">
    <div class="col">
        <strong>Petah Tikva, Center District, IL</strong>
        <span class="text-bg-success px-1 rounded">R</span>
        <p class="py-4">
            <span class="text-bg-secondary px-1 rounded">ILS 139K-151K (estimate)</span>
            <span class="text-bg-warning px-1 rounded">Mid-level</span>
        </p>
    </div>
    <div class="col-6 col-md-5 col-lg-4 text-end">
        <div class="fw-bold mb-2">
            <a class="text-decoration-none d-block text-break" href="/company/medison-pharma-16767/">
                @ M...
            </a>
        </div>
        <span class="d-block text-muted py-2">
            Found 8h ago
        </span>
    </div>
</div>

<h5>Tasks</h5>
<ul>
    <li>Build AI powered data solutions</li>
    <li>Design data pipelines</li>
</ul>

<h5>Perks/Benefits</h5>
<ul>
    <li>N/A</li>
</ul>

<h5>Skills/Tech-stack</h5>
<p>
    <a href="/jobs/skill-python/">Python</a> |
    <a href="/jobs/skill-sql/">SQL</a>
</p>
</main></body></html>`

func TestAijobsDetailMapsCoreFields(t *testing.T) {
	fake := aijobsGetPostFake{
		getHTML: func(string) (*html.Node, error) {
			return html.Parse(strings.NewReader(aijobsDetailFixture))
		},
	}
	job, ok := aijobs{http: fake}.detail(context.Background(), "/job/data-specialist-268449/")
	if !ok {
		t.Fatal("detail() = false, want a valid posting")
	}
	if job.ExternalID != "268449" {
		t.Errorf("ExternalID = %q, want %q", job.ExternalID, "268449")
	}
	if job.URL != "https://aijobs.net/job/data-specialist-268449/" {
		t.Errorf("URL = %q, want the absolute detail page URL", job.URL)
	}
	if job.Title != "Data Specialist" {
		t.Errorf("Title = %q, want %q", job.Title, "Data Specialist")
	}
	if job.Company != "Medison Pharma" {
		t.Errorf("Company = %q, want %q (derived from the /company/ slug, not the masked label)", job.Company, "Medison Pharma")
	}
	if job.Location != "Petah Tikva, Center District, IL" {
		t.Errorf("Location = %q, want %q", job.Location, "Petah Tikva, Center District, IL")
	}
	if !job.Remote {
		t.Error("Remote = false, want true (the R badge)")
	}
	if job.WorkMode != "remote" {
		t.Errorf("WorkMode = %q, want %q", job.WorkMode, "remote")
	}
	if !strings.Contains(job.Description, "Build AI powered data solutions") || !strings.Contains(job.Description, "Design data pipelines") {
		t.Errorf("Description = %q, want it to contain both task items", job.Description)
	}
	if strings.Contains(job.Description, "N/A") {
		t.Errorf("Description = %q, want the placeholder Perks/Benefits (N/A) section omitted", job.Description)
	}
	if want := []string{"Python", "SQL"}; !slices.Equal(job.Skills, want) {
		t.Errorf("Skills = %v, want %v", job.Skills, want)
	}
}

func TestAijobsDetailDropsPostingWithNoCompanyLink(t *testing.T) {
	fake := aijobsGetPostFake{
		getHTML: func(string) (*html.Node, error) {
			return html.Parse(strings.NewReader(`<html><body><main><h1>Some Role</h1></main></body></html>`))
		},
	}
	if _, ok := (aijobs{http: fake}).detail(context.Background(), "/job/some-role-1/"); ok {
		t.Error("detail() = true, want false for a page with no /company/ link")
	}
}

func TestAijobsDetailIncludesRealPerks(t *testing.T) {
	const withPerks = `<html><body><main>
<h1>Role</h1>
<a href="/company/acme-1/">@ A...</a>
<h5>Tasks</h5><ul><li>Do the work</li></ul>
<h5>Perks/Benefits</h5><ul><li>Health insurance</li><li>Remote stipend</li></ul>
</main></body></html>`
	fake := aijobsGetPostFake{getHTML: func(string) (*html.Node, error) {
		return html.Parse(strings.NewReader(withPerks))
	}}
	job, ok := aijobs{http: fake}.detail(context.Background(), "/job/role-2/")
	if !ok {
		t.Fatal("detail() = false, want a valid posting")
	}
	if !strings.Contains(job.Description, "Health insurance") || !strings.Contains(job.Description, "Remote stipend") {
		t.Errorf("Description = %q, want real perks included", job.Description)
	}
}

// TestAijobsPostedTextRecognizesBothLabels guards a real-site discrepancy found only by
// live smoke-testing task 6's ingest run: a recently-crawled posting reads "Found X ago"
// (as every fixture elsewhere in this file uses), but an older one — confirmed live
// against https://aijobs.net/job/statistics-expert-phd-remote-238344/ — reads
// "Published Xd ago" instead. Both must yield the same relative-time text.
func TestAijobsPostedTextRecognizesBothLabels(t *testing.T) {
	cases := map[string]string{
		`<span>Found 8h ago</span>`:      "8h ago",
		`<span>Published 15d ago</span>`: "15d ago",
	}
	for markup, want := range cases {
		root, err := html.Parse(strings.NewReader("<html><body>" + markup + "</body></html>"))
		if err != nil {
			t.Fatalf("html.Parse: %v", err)
		}
		if got := aijobsPostedText(root); got != want {
			t.Errorf("aijobsPostedText(%q) = %q, want %q", markup, got, want)
		}
	}
}

func TestAijobsCompanyNameFromSlug(t *testing.T) {
	cases := map[string]string{
		"/company/medison-pharma-16767/": "Medison Pharma",
		"/company/acme-1/":               "Acme",
		"/company/multi-word-co-42/":     "Multi Word Co",
	}
	for href, want := range cases {
		root, err := html.Parse(strings.NewReader(`<html><body><a href="` + href + `">@ x</a></body></html>`))
		if err != nil {
			t.Fatalf("html.Parse: %v", err)
		}
		if got := aijobsCompanyName(root); got != want {
			t.Errorf("aijobsCompanyName(%q) = %q, want %q", href, got, want)
		}
	}
}

func TestAijobsParsePostedAtHoursDaysWeeks(t *testing.T) {
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		text string
		want time.Time
	}{
		{"8h ago", now.Add(-8 * time.Hour)},
		{"3d ago", now.AddDate(0, 0, -3)},
		{"2w ago", now.AddDate(0, 0, -14)},
	}
	for _, c := range cases {
		got := aijobsParsePostedAt(c.text, now)
		if got == nil {
			t.Errorf("aijobsParsePostedAt(%q) = nil, want %v", c.text, c.want)
			continue
		}
		if !got.Equal(c.want) {
			t.Errorf("aijobsParsePostedAt(%q) = %v, want %v", c.text, got, c.want)
		}
	}
}

func TestAijobsParsePostedAtUnrecognizedTextLeavesNil(t *testing.T) {
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	for _, text := range []string{"", "just now", "8 hours ago", "8x ago"} {
		if got := aijobsParsePostedAt(text, now); got != nil {
			t.Errorf("aijobsParsePostedAt(%q) = %v, want nil", text, got)
		}
	}
}

// TestAijobsParsePostedAtMonthsYears locks in tasks.md 5.1's decision: a flat 30/365-day
// approximation, not calendar-aware AddDate(0, -n, 0) — PostedAt is a freshness signal,
// not a legal date.
func TestAijobsParsePostedAtMonthsYears(t *testing.T) {
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		text string
		want time.Time
	}{
		{"5mo ago", now.AddDate(0, 0, -5*30)},
		{"1y ago", now.AddDate(0, 0, -365)},
	}
	for _, c := range cases {
		got := aijobsParsePostedAt(c.text, now)
		if got == nil {
			t.Errorf("aijobsParsePostedAt(%q) = nil, want %v", c.text, c.want)
			continue
		}
		if !got.Equal(c.want) {
			t.Errorf("aijobsParsePostedAt(%q) = %v, want %v", c.text, got, c.want)
		}
	}
}

func TestAijobsJobID(t *testing.T) {
	cases := map[string]string{
		"/job/data-specialist-petah-tikva-center-district-il-268449/": "268449",
		"/job/lead-ai-engineer-268513/":                               "268513",
		"/company/medison-pharma-16767/":                              "",
		"/jobs/skill-python/":                                         "",
		"/job/no-trailing-id/":                                        "",
	}
	for href, want := range cases {
		if got := aijobsJobID(href); got != want {
			t.Errorf("aijobsJobID(%q) = %q, want %q", href, got, want)
		}
	}
}

const aijobsMinimalDetailHTML = `<html><body><main>
<h1>Role</h1>
<a href="/company/acme-1/">@ A...</a>
<h5>Tasks</h5><ul><li>Do the thing</li></ul>
</main></body></html>`

func TestAijobsImplementsHydratingSource(t *testing.T) {
	var _ HydratingSource = aijobs{}
}

func TestAijobsFetchNewHydratesUnseenAndRefreshesSeenPostings(t *testing.T) {
	var mu sync.Mutex
	requestedSeenPosting := false
	fake := aijobsGetPostFake{
		postForm: func(url string) (*html.Node, error) {
			if strings.Contains(url, "page=1") {
				return html.Parse(strings.NewReader(aijobsListingPage("1", "2")))
			}
			return nil, fmt.Errorf("no route for %s", url)
		},
		getHTML: func(url string) (*html.Node, error) {
			if strings.Contains(url, "role-1") {
				mu.Lock()
				requestedSeenPosting = true
				mu.Unlock()
			}
			if strings.Contains(url, "/job/role-") {
				return html.Parse(strings.NewReader(aijobsMinimalDetailHTML))
			}
			return html.Parse(strings.NewReader("<html></html>")) // session bootstrap
		},
	}

	jobs, err := (aijobs{http: fake, maxNewPerRun: 500}).FetchNew(context.Background(), CompanyEntry{}, seenSet("1"))
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}

	mu.Lock()
	poisoned := requestedSeenPosting
	mu.Unlock()
	if poisoned {
		t.Error("detail fetched for an already-seen posting (role-1)")
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2 (the unseen posting hydrated, the seen one refreshed)", len(jobs))
	}
	var hydrated, refreshed *Job
	for i := range jobs {
		if jobs[i].SeenRefresh {
			refreshed = &jobs[i]
		} else {
			hydrated = &jobs[i]
		}
	}
	if hydrated == nil || hydrated.ExternalID != "2" {
		t.Fatalf("hydrated job = %+v, want ExternalID %q", hydrated, "2")
	}
	if hydrated.Company != "Acme" {
		t.Errorf("Company = %q, want %q", hydrated.Company, "Acme")
	}
	// Still-open posting "1" was re-listed but not re-fetched: a SeenRefresh Job, not
	// silence, or its last_seen_at would never advance and the company-scoped sweep would
	// eventually close it as unseen with no way back in.
	if refreshed == nil || refreshed.ExternalID != "1" {
		t.Fatalf("refreshed job = %+v, want ExternalID %q", refreshed, "1")
	}
}

func TestAijobsFetchHydratesEverythingWithoutASeenPredicate(t *testing.T) {
	fake := aijobsGetPostFake{
		postForm: func(url string) (*html.Node, error) {
			if strings.Contains(url, "page=1") {
				return html.Parse(strings.NewReader(aijobsListingPage("1")))
			}
			return nil, fmt.Errorf("no route for %s", url)
		},
		getHTML: func(url string) (*html.Node, error) {
			if strings.Contains(url, "/job/role-") {
				return html.Parse(strings.NewReader(aijobsMinimalDetailHTML))
			}
			return html.Parse(strings.NewReader("<html></html>"))
		},
	}
	jobs, err := (aijobs{http: fake, maxNewPerRun: 500}).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "1" {
		t.Errorf("jobs = %+v, want exactly the one posting hydrated", jobs)
	}
}

func TestAijobsMaxNewPerRunFromEnv(t *testing.T) {
	cases := map[string]int{
		"":     aijobsDefaultMaxNewPerRun,
		"0":    aijobsDefaultMaxNewPerRun,
		"-5":   aijobsDefaultMaxNewPerRun,
		"abc":  aijobsDefaultMaxNewPerRun,
		"1234": 1234,
	}
	for env, want := range cases {
		t.Setenv("AIJOBS_MAX_NEW_PER_RUN", env)
		if got := aijobsMaxNewPerRunFromEnv(); got != want {
			t.Errorf("AIJOBS_MAX_NEW_PER_RUN=%q: got %d, want %d", env, got, want)
		}
	}
}

func TestAijobsNewAijobsDefaultsNonPositiveBudget(t *testing.T) {
	for _, n := range []int{0, -1} {
		a := NewAijobs(nil, n).(aijobs)
		if a.maxNewPerRun != aijobsDefaultMaxNewPerRun {
			t.Errorf("NewAijobs(nil, %d).maxNewPerRun = %d, want %d", n, a.maxNewPerRun, aijobsDefaultMaxNewPerRun)
		}
	}
}

func TestAijobsProvider(t *testing.T) {
	if got := NewAijobs(nil, 0).Provider(); got != "aijobs" {
		t.Errorf("Provider() = %q, want %q", got, "aijobs")
	}
}

func TestAijobsIsBoardlessAggregator(t *testing.T) {
	s := NewAijobs(nil, 0)
	if _, ok := s.(boardless); !ok {
		t.Error("aijobs should implement the boardless marker")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("aijobs should implement the aggregator marker")
	}
}

func TestAijobsRegisteredAndFilterable(t *testing.T) {
	if _, ok := All(nil)["aijobs"]; !ok {
		t.Error("All() should register provider aijobs")
	}
	if !slices.Contains(FilterableProviders(), "aijobs") {
		t.Error("FilterableProviders() should include aijobs")
	}
}
