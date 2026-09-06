package sources

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// jazzhrDetailHTML is a JazzHR job page: server-rendered HTML whose only payload we read is
// the schema.org JobPosting ld+json. jobLocation is a single Place (not an array). The
// description embeds a <script> (written as <\/script>) that sanitizeHTML must strip.
const jazzhrDetailHTML = `<html><head></head><body>
<script type="application/ld+json">
{"@context":"http://schema.org/","@type":"JobPosting",
"url":"https://proautomated.applytojob.com/apply/nfHu9c2Sxz/Field-Service-Engineer",
"title":"Field Service Engineer",
"description":"<p>Fix machines.</p><script>alert(1)<\/script>",
"datePosted":"2026-06-16",
"hiringOrganization":{"@type":"Organization","name":"ProAutomated"},
"jobLocation":{"@type":"Place","address":{"@type":"PostalAddress","addressLocality":"Dover","addressRegion":"DE"}}}
</script>
</body></html>`

// jazzhrListingHTML is the /apply listing: anchors to /apply/<token>/<slug> job pages,
// plus noise links the adapter must ignore.
const jazzhrListingHTML = `<html><body>
<a href="/about">About</a>
<a href="/apply/nfHu9c2Sxz/Field-Service-Engineer">Field Service Engineer</a>
<a href="/apply/nfHu9c2Sxz/Field-Service-Engineer">Apply</a>
</body></html>`

func TestJazzHRProvider(t *testing.T) {
	if got := NewJazzHR(nil).Provider(); got != "jazzhr" {
		t.Errorf("Provider() = %q, want %q", got, "jazzhr")
	}
}

// JazzHR earns fullBoardListing because the /apply listing is a single page linking every
// open posting (no pagination), so a listing failure aborts the whole Fetch.
func TestJazzHRMarkers(t *testing.T) {
	s := NewJazzHR(nil)
	if _, ok := s.(fullBoardListing); !ok {
		t.Error("jazzhr should implement the fullBoardListing marker")
	}
}

func TestJazzHRRegisteredAsFullBoardListing(t *testing.T) {
	if !FullBoardListingProviders(All(nil))["jazzhr"] {
		t.Error("FullBoardListingProviders(All(nil)) should include jazzhr")
	}
}

// A listing fetch failure must abort the whole Fetch, never return a partial result as
// success — the property TestJazzHRMarkers' fullBoardListing claim rests on.
func TestJazzHRFetchPropagatesAListingError(t *testing.T) {
	fake := &fakeHTTP{err: errors.New("boom")}
	if _, err := NewJazzHR(fake).Fetch(context.Background(), CompanyEntry{Board: "acme"}); err == nil {
		t.Fatal("Fetch succeeded despite a listing error")
	}
}

func TestJazzHRJobID(t *testing.T) {
	cases := map[string]string{
		"https://proautomated.applytojob.com/apply/nfHu9c2Sxz/Field-Service-Engineer": "nfHu9c2Sxz",
		"/apply/bhCE7nHkv6/Some-Role":               "bhCE7nHkv6",
		"https://proautomated.applytojob.com/about": "",
	}
	for u, want := range cases {
		if got := jazzhrJobID(u); got != want {
			t.Errorf("jazzhrJobID(%q) = %q, want %q", u, got, want)
		}
	}
}

func TestJazzHRFetchListingThenDetailAndMaps(t *testing.T) {
	job := "https://proautomated.applytojob.com/apply/nfHu9c2Sxz/Field-Service-Engineer"
	// The listing emits a relative href, so the test fails unless the adapter resolves it
	// against the board host before fetching the detail.
	fake := (&routedHTTP{}).
		route("/apply/nfHu9c2Sxz/Field-Service-Engineer", jazzhrDetailHTML).
		route("/apply", jazzhrListingHTML)

	jobs, err := NewJazzHR(fake).Fetch(context.Background(), CompanyEntry{
		Company: "ProAutomated", Provider: "jazzhr", Board: "proautomated",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (the title+apply anchors de-duped)", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "nfHu9c2Sxz" {
		t.Errorf("ExternalID = %q, want nfHu9c2Sxz", j.ExternalID)
	}
	if j.URL != job {
		t.Errorf("URL = %q, want %q", j.URL, job)
	}
	if j.Title != "Field Service Engineer" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "ProAutomated" {
		t.Errorf("Company = %q, want hiringOrganization name", j.Company)
	}
	if j.Location != "Dover, DE" {
		t.Errorf("Location = %q, want %q", j.Location, "Dover, DE")
	}
	if strings.Contains(j.Description, "<script>") || strings.Contains(j.Description, "alert(1)") {
		t.Errorf("Description not sanitized: %q", j.Description)
	}
	if !strings.Contains(j.Description, "Fix machines") {
		t.Errorf("Description lost real content: %q", j.Description)
	}
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2026, 6, 16, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("PostedAt = %v, want 2026-06-16", j.PostedAt)
	}
}

func TestJazzHRCompanyFallsBackToEntry(t *testing.T) {
	detail := `<html><body><script type="application/ld+json">
{"@type":"JobPosting","title":"Role","datePosted":"2026-06-16",
"hiringOrganization":{"name":""},
"jobLocation":{"address":{"addressLocality":"Austin","addressCountry":"US"}}}
</script></body></html>`
	fake := (&routedHTTP{}).
		route("/apply/abc123XYZ0/role", detail).
		route("/apply", `<html><body><a href="/apply/abc123XYZ0/role">Role</a></body></html>`)

	jobs, err := NewJazzHR(fake).Fetch(context.Background(), CompanyEntry{Company: "Acme Corp", Board: "acme"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].Company != "Acme Corp" {
		t.Fatalf("Company = %q, want fallback %q", jobs[0].Company, "Acme Corp")
	}
}

const jazzhrTwoPostingListing = `<html><body>
<a href="/apply/keptKept11/kept">kept</a>
<a href="/apply/lostLost22/lost">lost</a>
</body></html>`

// A detail request the crawl could not READ must not look like a posting that is not there.
// The detail page is jazzhr's only source for a posting, and it is re-fetched on every run, so
// a dropped one leaves the posting missing from a crawl that reported no failure at all — and
// the stale-job sweep closes a live vacancy once the grace window elapses.
func TestJazzHRUnreadableDetailIsMarkedNotDropped(t *testing.T) {
	// routeErr, not an absent route: `route("/apply", ...)` matches by SUBSTRING, so it also
	// answers /apply/lostLost22/lost — with the listing HTML, which carries no ld+json. That
	// drives the parse branch and leaves the transport one, the branch this test is named for,
	// unexercised. The two are different failures and the adapter treats them the same on
	// purpose; each still needs its own cover.
	fake := (&routedHTTP{}).
		routeErr("/apply/lostLost22", errors.New("connection reset by peer")).
		route("/apply/keptKept11/kept", jazzhrDetailHTML).
		route("/apply", jazzhrTwoPostingListing)

	jobs, err := NewJazzHR(fake).Fetch(context.Background(), CompanyEntry{Company: "Acme Corp", Board: "acme"})
	if err != nil {
		t.Fatalf("Fetch should not abort the board on one failed detail: %v", err)
	}
	read := readPostings(jobs)
	if len(read) != 1 || read[0].ExternalID != "keptKept11" {
		t.Fatalf("read = %v, want only the posting whose detail answered", read)
	}
	markers := unreadableMarkers(jobs)
	if len(markers) != 1 || markers[0].ExternalID != "lostLost22" {
		t.Fatalf("unreadable markers = %v, want one for the posting whose detail did not", markers)
	}
	if markers[0].Company != "Acme Corp" {
		t.Errorf("marker Company = %q, want the board's employer — it names the close scope the run withholds", markers[0].Company)
	}
}

// The other half of the distinction: 404 is the platform's own answer that the posting is
// gone, so the crawl drops it and the board's evidence stays complete — otherwise a board
// whose listing has gone stale could never retire anything.
func TestJazzHRGoneDetailDropsThePosting(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/apply/keptKept11/kept", jazzhrDetailHTML).
		routeErr("/apply/lostLost22/lost", &StatusError{Method: "GET", Code: 404, URL: "https://acme.applytojob.com/apply/lostLost22/lost"}).
		route("/apply", jazzhrTwoPostingListing)

	jobs, err := NewJazzHR(fake).Fetch(context.Background(), CompanyEntry{Company: "Acme Corp", Board: "acme"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "keptKept11" {
		t.Fatalf("got %v, want only the kept posting — a 404 is evidence, not a hole", jobs)
	}
}
