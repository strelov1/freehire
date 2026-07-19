package sources

import (
	"context"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestISmartRecruitProvider(t *testing.T) {
	if got := NewISmartRecruit(nil).Provider(); got != "ismartrecruit" {
		t.Errorf("Provider() = %q, want %q", got, "ismartrecruit")
	}
}

// TestISmartRecruitListingURL pins the tenant-token wrapper: a readable board id must rebuild the
// exact widget URL observed live (E7p + rawbase64 + Q9e), so an encoding regression is caught.
func TestISmartRecruitListingURL(t *testing.T) {
	got := ismartRecruitListingURL("caglobalint.com_8oi10Y11UJ")
	want := "https://app.ismartrecruit.com/openJobWebsite?tenantId=E7pY2FnbG9iYWxpbnQuY29tXzhvaTEwWTExVUoQ9e&view=grid&col=2&lang=en"
	if got != want {
		t.Errorf("listing URL =\n %q\nwant\n %q", got, want)
	}
}

// TestISmartRecruitInhouseToken pins the second seeded board's wrap to its live widget token, so
// both shipped tenants stay reproducible from their readable board ids.
func TestISmartRecruitInhouseToken(t *testing.T) {
	got := ismartRecruitListingURL("inhousehiring.com_BUAVSM0UMK")
	if !strings.Contains(got, "tenantId=E7paW5ob3VzZWhpcmluZy5jb21fQlVBVlNNMFVNSwQ9e&") {
		t.Errorf("inhouse listing URL = %q, want the live tenantId token", got)
	}
}

// TestISmartRecruitBoardFileValidates loads the shipped board file and validates every entry
// against the registry, catching a missing registration or a malformed seed entry.
func TestISmartRecruitBoardFileValidates(t *testing.T) {
	cfg, err := LoadConfig("../../sources/ismartrecruit.yml")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if err := cfg.Validate(All(nil)); err != nil {
		t.Fatalf("sources/ismartrecruit.yml fails validation: %v", err)
	}
}

// TestISmartRecruitCardsFixture parses a real listing grid and asserts each posting card: the
// numeric external id decoded from the detail href, the title (en-dash entity decoded), the
// location cell, and the detail URL.
func TestISmartRecruitCardsFixture(t *testing.T) {
	root := parseFixture(t, "testdata/ismartrecruit/listing.html")
	cards := ismartRecruitCards(root)
	if len(cards) != 2 {
		t.Fatalf("got %d cards, want 2", len(cards))
	}
	c := cards[0]
	if c.ExternalID != "14208623" {
		t.Errorf("ExternalID = %q, want the numeric posting id 14208623", c.ExternalID)
	}
	if !strings.HasPrefix(c.Title, "On-Site Mining Engineer") || strings.Contains(c.Title, "&#8211;") {
		t.Errorf("Title = %q, want the decoded role text", c.Title)
	}
	if c.Location != "Namibia" {
		t.Errorf("Location = %q, want %q", c.Location, "Namibia")
	}
	if !strings.Contains(c.URL, "jobDescription?x=") {
		t.Errorf("URL = %q, want the jobDescription detail link", c.URL)
	}
	if cards[1].ExternalID != "14208625" {
		t.Errorf("second card ExternalID = %q, want 14208625", cards[1].ExternalID)
	}
}

// TestISmartRecruitEnrichFixture folds a real detail page onto a card-derived job and asserts the
// description is taken from og:description with its double-encoded entities fully decoded.
func TestISmartRecruitEnrichFixture(t *testing.T) {
	root := parseFixture(t, "testdata/ismartrecruit/detail.html")
	job := ismartRecruitEnrich(Job{ExternalID: "14208623", Title: "On-Site Mining Engineer"}, root)

	if !strings.Contains(job.Description, "open pit copper project") {
		t.Errorf("Description missing the role body: %.120q", job.Description)
	}
	// The stored description is sanitized HTML, so a literal "&" is the entity "&amp;"; what must
	// NOT survive is the SOURCE's double-encoding ("&amp;amp;"), proving one decode layer was
	// stripped when the detail attribute was parsed.
	for _, doubled := range []string{"amp;amp;", "amp;#39;", "amp;#8211;"} {
		if strings.Contains(job.Description, doubled) {
			t.Errorf("Description kept a double-encoded entity %q: %.160q", doubled, job.Description)
		}
	}
}

// TestISmartRecruitEnrichTitleFallback asserts og:title fills in only when the card rendered no
// title, never overwriting the card's own.
func TestISmartRecruitEnrichTitleFallback(t *testing.T) {
	root := parseFixture(t, "testdata/ismartrecruit/detail.html")

	if got := ismartRecruitEnrich(Job{}, root).Title; !strings.HasPrefix(got, "On-Site Mining Engineer") {
		t.Errorf("empty-title job: Title = %q, want og:title fallback", got)
	}
	if got := ismartRecruitEnrich(Job{Title: "Kept"}, root).Title; got != "Kept" {
		t.Errorf("Title = %q, want the card's own title preserved", got)
	}
}

func TestISmartRecruitExternalID(t *testing.T) {
	cases := map[string]string{
		"https://app.ismartrecruit.com/jobDescription?x=E7pY2FnbG9iYWxpbnQuY29tXzE0MjA4NjIzX1dfZW4=Q9e&view=grid": "14208623",
		"https://app.ismartrecruit.com/openJobWebsite?tenantId=E7pfooQ9e&view=grid":                               "",
		"https://app.ismartrecruit.com/jobDescription?x=notatoken":                                                "",
	}
	for href, want := range cases {
		if got := ismartRecruitExternalID(href); got != want {
			t.Errorf("externalID(%q) = %q, want %q", href, got, want)
		}
	}
}

// ismartRecruitFake routes the listing and detail GETs to their fixtures by URL shape.
type ismartRecruitFake struct{ listing, detail *html.Node }

func (f ismartRecruitFake) GetHTML(_ context.Context, u string) (*html.Node, error) {
	if strings.Contains(u, "openJobWebsite") {
		return f.listing, nil
	}
	return f.detail, nil
}

// TestISmartRecruitFetch drives the whole adapter over fixtures: the listing yields both postings
// and each is enriched from the detail page into a fully-populated Job.
func TestISmartRecruitFetch(t *testing.T) {
	fake := ismartRecruitFake{
		listing: parseFixture(t, "testdata/ismartrecruit/listing.html"),
		detail:  parseFixture(t, "testdata/ismartrecruit/detail.html"),
	}
	jobs, err := NewISmartRecruit(fake).Fetch(context.Background(), CompanyEntry{Company: "CA Global"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2", len(jobs))
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	j, ok := byID["14208623"]
	if !ok {
		t.Fatalf("posting 14208623 missing; got ids %v", byID)
	}
	if j.Company != "CA Global" {
		t.Errorf("Company = %q, want the configured entry name", j.Company)
	}
	if j.Location != "Namibia" || j.Remote {
		t.Errorf("Location/Remote = %q/%v, want Namibia/false", j.Location, j.Remote)
	}
	if !strings.Contains(j.Description, "open pit copper project") {
		t.Errorf("Description not folded from detail: %.80q", j.Description)
	}
}
