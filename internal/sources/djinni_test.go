package sources

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// djinniListingHTML is a djinni.co-shaped listing page: one application/ld+json block whose
// payload is an ARRAY of JobPosting objects (as the real listing emits), alongside the
// Organization/BreadcrumbList blocks the real page also carries — which LDJobPostings must
// ignore. The first posting is a fully-populated remote UA role; the second is a minimal but
// usable posting; the third is unusable (no company) and must be dropped.
const djinniListingHTML = `<html><head>
<script type="application/ld+json">{"@type":"Organization","name":"Djinni"}</script>
<script type="application/ld+json">
[{"@context":"https://schema.org/","@type":"JobPosting",
"title":"Senior Backend Go Engineer",
"url":"https://djinni.co/jobs/837549-senior-backend-go-engineer/",
"description":"<p>Build &amp; ship services.</p>",
"datePosted":"2026-07-06T04:43:20.486231",
"employmentType":"FULL_TIME",
"jobLocationType":"TELECOMMUTE",
"identifier":837549,
"hiringOrganization":{"@type":"Organization","name":"Acme","sameAs":"https://acme.example/"},
"applicantLocationRequirements":{"@type":"AdministrativeArea","address":{"@type":"PostalAddress","addressCountry":"UA"}},
"validThrough":"2026-08-15T04:43:20.486231"},
{"@context":"https://schema.org/","@type":"JobPosting",
"title":"Recruiter",
"url":"https://djinni.co/jobs/837547-recruiter/",
"description":"Hire people.",
"employmentType":"FULL_TIME",
"identifier":837547,
"hiringOrganization":{"@type":"Organization","name":"Beta"}},
{"@context":"https://schema.org/","@type":"JobPosting",
"title":"Orphan role",
"url":"https://djinni.co/jobs/1-orphan/",
"identifier":1,
"hiringOrganization":{"@type":"Organization"}}]
</script>
</head><body></body></html>`

// djinniSingleHTML carries one JobPosting as a lone object (not wrapped in an array), which the
// adapter must still map.
const djinniSingleHTML = `<html><head>
<script type="application/ld+json">
{"@context":"https://schema.org/","@type":"JobPosting",
"title":"Lone Posting","url":"https://djinni.co/jobs/900-lone/","identifier":900,
"hiringOrganization":{"@type":"Organization","name":"Solo"}}
</script>
</head><body></body></html>`

const djinniEmptyHTML = `<html><head></head><body></body></html>`

func init() { djinniPageDelay = 0 } // don't pace the crawl in unit tests

// djinniPagedHTTP models djinni.co's listing pagination. A page present in bodies is served
// directly (final URL == requested URL, no redirect). A page listed in errPages fails with a 403
// (modelling Djinni rate-limiting a fast datacenter crawl). A page ABSENT from both models the
// real past-the-end behavior: djinni 302s to the bare listing (/jobs/) and serves page 1's
// content — so the resolved final URL loses the page marker, the adapter's end-of-feed signal.
type djinniPagedHTTP struct {
	bodies   map[int]string
	errPages map[int]bool
	gotURLs  []string
}

func (f *djinniPagedHTTP) GetHTMLResolved(_ context.Context, url string) (*html.Node, string, error) {
	f.gotURLs = append(f.gotURLs, url)
	page := djinniPageOf(url)
	if f.errPages[page] {
		return nil, "", fmt.Errorf("GET %s: status 403", url)
	}
	if body, ok := f.bodies[page]; ok {
		node, err := html.Parse(strings.NewReader(body))
		return node, url, err // served directly, no redirect
	}
	// Past the end: 302 → /jobs/, re-serving page 1's content under a page-less final URL.
	node, err := html.Parse(strings.NewReader(f.bodies[1]))
	return node, "https://djinni.co/jobs/", err
}

// djinniPageOf extracts N from a "…?page=N" URL, or 0 when absent.
func djinniPageOf(url string) int {
	_, after, ok := strings.Cut(url, "page=")
	if !ok {
		return 0
	}
	n, _ := strconv.Atoi(after)
	return n
}

func TestDjinniProvider(t *testing.T) {
	if got := NewDjinni(nil).Provider(); got != "djinni" {
		t.Errorf("Provider() = %q, want %q", got, "djinni")
	}
}

func TestDjinniFetchMapsListingArray(t *testing.T) {
	fake := &djinniPagedHTTP{bodies: map[int]string{1: djinniListingHTML}}

	jobs, err := NewDjinni(fake).Fetch(context.Background(), CompanyEntry{Provider: "djinni"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	// Page 1 has three JobPostings; the third is unusable (no company) and is dropped.
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want the 2 usable postings", len(jobs))
	}

	j := jobs[0]
	if j.ExternalID != "837549" {
		t.Errorf("ExternalID = %q, want the numeric identifier as string", j.ExternalID)
	}
	if j.URL != "https://djinni.co/jobs/837549-senior-backend-go-engineer/" {
		t.Errorf("URL = %q", j.URL)
	}
	if j.Title != "Senior Backend Go Engineer" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "Acme" {
		t.Errorf("Company = %q, want hiringOrganization.name", j.Company)
	}
	if j.Location != "UA" {
		t.Errorf("Location = %q, want addressCountry", j.Location)
	}
	// sanitizeHTML keeps structural markup (bluemonday allows <p>) and re-encodes bare &,
	// so a description round-trips as safe HTML rather than being flattened to plain text.
	if j.Description != "<p>Build &amp; ship services.</p>" {
		t.Errorf("Description = %q, want sanitized description HTML", j.Description)
	}
	if !j.Remote || j.WorkMode != "remote" {
		t.Errorf("Remote=%v WorkMode=%q, want true/remote for TELECOMMUTE", j.Remote, j.WorkMode)
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q, want full_time", j.EmploymentType)
	}
	if j.PostedAt == nil {
		t.Errorf("PostedAt = nil, want the zone-less datePosted parsed")
	}
}

func TestDjinniDropsUnusablePosting(t *testing.T) {
	fake := &djinniPagedHTTP{bodies: map[int]string{1: djinniListingHTML}}
	jobs, err := NewDjinni(fake).Fetch(context.Background(), CompanyEntry{Provider: "djinni"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	for _, j := range jobs {
		if j.Title == "Orphan role" {
			t.Fatalf("mapped the company-less posting %q, want it dropped", j.Title)
		}
	}
}

func TestDjinniAcceptsSingleObjectBlock(t *testing.T) {
	fake := &djinniPagedHTTP{bodies: map[int]string{1: djinniSingleHTML}}
	jobs, err := NewDjinni(fake).Fetch(context.Background(), CompanyEntry{Provider: "djinni"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "900" {
		t.Fatalf("got %#v, want the lone-object posting mapped", jobs)
	}
}

// TestDjinniStopsAtPastEndRedirect is the regression guard for the redirect trap: a past-the-end
// page 302s to /jobs/ and re-serves page 1 (which is NOT empty), so a naive "stop on empty page"
// would loop, re-ingesting page 1 up to the page cap. The adapter must instead stop on the
// redirect (final URL lost the page marker) after requesting exactly page 1 then page 2.
func TestDjinniStopsAtPastEndRedirect(t *testing.T) {
	fake := &djinniPagedHTTP{bodies: map[int]string{1: djinniListingHTML}}
	jobs, err := NewDjinni(fake).Fetch(context.Background(), CompanyEntry{Provider: "djinni"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want only page 1's 2 usable postings (no re-ingested duplicates)", len(jobs))
	}
	if len(fake.gotURLs) != 2 {
		t.Fatalf("requested %d pages (%v), want page 1 then the redirected page 2 only", len(fake.gotURLs), fake.gotURLs)
	}
}

// TestDjinniStopsOnEmptyNonRedirectedPage covers the secondary stop: a page that returns 200
// with no JobPosting (not a redirect) also ends the crawl.
func TestDjinniStopsOnEmptyNonRedirectedPage(t *testing.T) {
	fake := &djinniPagedHTTP{bodies: map[int]string{1: djinniListingHTML, 2: djinniEmptyHTML}}
	jobs, err := NewDjinni(fake).Fetch(context.Background(), CompanyEntry{Provider: "djinni"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want page 1's 2 postings then a stop at the empty page 2", len(jobs))
	}
	if len(fake.gotURLs) != 2 {
		t.Fatalf("requested %d pages, want page 1 then the empty page 2", len(fake.gotURLs))
	}
}

// TestDjinniPartialOnMidCrawlError covers the datacenter rate-limit case: page 1 maps, then
// page 2 403s. The adapter must keep page 1's jobs and return no error (a partial crawl of the
// freshest pages beats losing everything and dropping the board into cooldown).
func TestDjinniPartialOnMidCrawlError(t *testing.T) {
	fake := &djinniPagedHTTP{
		bodies:   map[int]string{1: djinniListingHTML},
		errPages: map[int]bool{2: true},
	}
	jobs, err := NewDjinni(fake).Fetch(context.Background(), CompanyEntry{Provider: "djinni"})
	if err != nil {
		t.Fatalf("Fetch returned error %v, want nil (partial success on a mid-crawl 403)", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want page 1's 2 usable postings kept despite the page-2 403", len(jobs))
	}
}

// TestDjinniFailsHardWhenFirstPageErrors guards the sweep: an error on page 1 (zero jobs
// collected) must surface as a board failure, never an empty successful crawl — otherwise the
// unseen-sweep would close the whole Djinni catalogue.
func TestDjinniFailsHardWhenFirstPageErrors(t *testing.T) {
	fake := &djinniPagedHTTP{
		bodies:   map[int]string{1: djinniListingHTML},
		errPages: map[int]bool{1: true},
	}
	jobs, err := NewDjinni(fake).Fetch(context.Background(), CompanyEntry{Provider: "djinni"})
	if err == nil {
		t.Fatalf("Fetch returned nil error with %d jobs, want a board failure when page 1 fails", len(jobs))
	}
}

func TestDjinniIsAggregator(t *testing.T) {
	reg := All(nil)
	if got := ProviderKind(reg, "djinni"); got != KindAggregator {
		t.Errorf("ProviderKind(djinni) = %q, want %q", got, KindAggregator)
	}
	found := false
	for _, p := range AggregatorProviders(reg) {
		if p == "djinni" {
			found = true
		}
	}
	if !found {
		t.Error("djinni missing from AggregatorProviders — it would not inherit ATS suppression")
	}
}
