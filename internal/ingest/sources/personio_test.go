package sources

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestPersonioProvider(t *testing.T) {
	if got := NewPersonio(nil).Provider(); got != "personio" {
		t.Errorf("Provider() = %q, want %q", got, "personio")
	}
}

// Personio earns fullBoardListing because Fetch's default-locale feed carries every open
// position in one document (no pagination), and its fetch failing aborts the whole Fetch —
// the English-feed and detail-page fallbacks are best-effort and only affect body richness,
// never whether a position is counted.
func TestPersonioMarkers(t *testing.T) {
	s := NewPersonio(nil)
	if _, ok := s.(fullBoardListing); !ok {
		t.Error("personio should implement the fullBoardListing marker")
	}
}

func TestPersonioRegisteredAsFullBoardListing(t *testing.T) {
	if !FullBoardListingProviders(All(nil))["personio"] {
		t.Error("FullBoardListingProviders(All(nil)) should include personio")
	}
}

// A listing fetch failure must abort the whole Fetch, never return a partial result as
// success — the property TestPersonioMarkers' fullBoardListing claim rests on.
func TestPersonioFetchPropagatesAListingError(t *testing.T) {
	fake := &fakeHTTP{err: errors.New("boom")}
	if _, err := NewPersonio(fake).Fetch(context.Background(), CompanyEntry{Board: "acme"}); err == nil {
		t.Fatal("Fetch succeeded despite a listing error")
	}
}

func TestPersonioFetch(t *testing.T) {
	fake := &fakeHTTP{body: `<?xml version="1.0" encoding="UTF-8"?>
<workzag-jobs>
  <position>
    <id>2596159</id>
    <subcompany>Acme Handwerk GmbH</subcompany>
    <office>Hamburg</office>
    <department>Engineering</department>
    <recruitingCategory>Tech</recruitingCategory>
    <name>Senior Go Engineer (m/w/d)</name>
    <jobDescriptions>
      <jobDescription><name>Tasks</name><value><![CDATA[<p>Build the pipeline.</p>]]></value></jobDescription>
      <jobDescription><name>Requirements</name><value><![CDATA[<ul><li>Go</li></ul>]]></value></jobDescription>
    </jobDescriptions>
    <createdAt>2026-04-09T20:28:25+00:00</createdAt>
  </position>
  <position>
    <id>2596160</id>
    <office>Remote</office>
    <name>Platform Engineer</name>
    <jobDescriptions>
      <jobDescription><name>Tasks</name><value><![CDATA[<p>Work from anywhere.</p>]]></value></jobDescription>
    </jobDescriptions>
    <createdAt>2026-04-09T20:28:25+00:00</createdAt>
  </position>
</workzag-jobs>`}

	jobs, err := NewPersonio(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "personio", Board: "acme",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !strings.Contains(fake.gotURL, "acme.jobs.personio.com") || !strings.Contains(fake.gotURL, "/xml") {
		t.Errorf("requested URL %q should target the board xml feed", fake.gotURL)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2", len(jobs))
	}

	j := jobs[0]
	if j.ExternalID != "2596159" {
		t.Errorf("ExternalID = %q, want the position id", j.ExternalID)
	}
	if j.Title != "Senior Go Engineer (m/w/d)" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Location != "Hamburg" {
		t.Errorf("Location = %q, want the office", j.Location)
	}
	// The feed has no posting URL — the adapter builds one from board + id.
	if j.URL != "https://acme.jobs.personio.com/job/2596159" {
		t.Errorf("URL = %q, want a constructed job URL", j.URL)
	}
	// Description concatenates the jobDescription values, sanitized.
	for _, want := range []string{"Build the pipeline.", "Go"} {
		if !strings.Contains(j.Description, want) {
			t.Errorf("Description missing %q, got %q", want, j.Description)
		}
	}
	if j.Remote {
		t.Error("Remote = true for a Hamburg office, want false")
	}
	if j.PostedAt == nil || j.PostedAt.UTC().Year() != 2026 {
		t.Errorf("PostedAt = %v, want parsed createdAt (2026)", j.PostedAt)
	}

	// The feed carries no remote flag — remote is inferred from the office text.
	if !jobs[1].Remote {
		t.Error("second job office=Remote should infer Remote = true")
	}
}

func TestPersonioFetchFallsBackToDetailWhenFeedDescriptionEmpty(t *testing.T) {
	// The board's default-locale feed omits this posting's body (it is published in
	// another locale), so jobDescriptions is empty. The detail page still server-renders
	// the body as a schema.org JobPosting, and the adapter falls back to it.
	feed := `<?xml version="1.0" encoding="UTF-8"?>
<workzag-jobs>
  <position>
    <id>2514154</id>
    <office>Munich (Hybrid)</office>
    <name>Marketing Intern (f/m/x)</name>
    <jobDescriptions></jobDescriptions>
    <createdAt>2026-01-30T16:17:59+00:00</createdAt>
  </position>
</workzag-jobs>`
	detail := `<html><head><script type="application/ld+json">` +
		`{"@context":"http://schema.org/","@type":"JobPosting",` +
		`"description":"&lt;p&gt;Your mission as our Marketing Intern.&lt;/p&gt;"}` +
		`</script></head><body></body></html>`

	fake := (&routedHTTP{}).
		route("/xml", feed).
		route("/job/2514154", detail)

	jobs, err := NewPersonio(fake).Fetch(context.Background(), CompanyEntry{
		Company: "4screen", Provider: "personio", Board: "4screen",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1", len(jobs))
	}
	if !strings.Contains(jobs[0].Description, "Your mission as our Marketing Intern.") {
		t.Errorf("Description = %q, want the detail-page JobPosting body", jobs[0].Description)
	}
}

func TestPersonioFetchFallsBackToEnglishFeedWhenDefaultLocaleEmpty(t *testing.T) {
	// The default-locale feed omits this posting's body, and its detail page carries no
	// JobPosting ld+json — but the ?language=en feed does have the body. The adapter
	// fetches the English feed once and fills the gap from it.
	defaultFeed := `<?xml version="1.0" encoding="UTF-8"?>
<workzag-jobs>
  <position>
    <id>2632015</id>
    <office>Hamburg</office>
    <name>Game Designer</name>
    <jobDescriptions></jobDescriptions>
    <createdAt>2026-01-30T16:17:59+00:00</createdAt>
  </position>
</workzag-jobs>`
	enFeed := `<?xml version="1.0" encoding="UTF-8"?>
<workzag-jobs>
  <position>
    <id>2632015</id>
    <office>Hamburg</office>
    <name>Game Designer</name>
    <jobDescriptions>
      <jobDescription><name>Tasks</name><value><![CDATA[<p>Design the game.</p>]]></value></jobDescription>
    </jobDescriptions>
    <createdAt>2026-01-30T16:17:59+00:00</createdAt>
  </position>
</workzag-jobs>`

	// Route the English feed before the default one: the ?language=en URL also contains
	// "/xml", so the more specific match must come first.
	fake := (&routedHTTP{}).
		route("language=en", enFeed).
		route("/xml", defaultFeed)

	jobs, err := NewPersonio(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Tonies", Provider: "personio", Board: "tonies",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1", len(jobs))
	}
	if !strings.Contains(jobs[0].Description, "Design the game.") {
		t.Errorf("Description = %q, want the English-feed body", jobs[0].Description)
	}
}

func TestPersonioFetchEmptyFeed(t *testing.T) {
	fake := &fakeHTTP{body: `<?xml version="1.0" encoding="UTF-8"?><workzag-jobs></workzag-jobs>`}
	jobs, err := NewPersonio(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "personio", Board: "acme",
	})
	if err != nil {
		t.Fatalf("Fetch on empty feed should not error: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("len(jobs) = %d, want 0 for an empty feed", len(jobs))
	}
}

// The fixtures under testdata/personio were recorded from the live boards on 2026-10-25.
// Both tenants answer /xml with 404 — their XML feed is switched off — while search.json
// still lists every open position. search.json carries a body only when asked for a
// locale the posting is published in: aconium's postings are German-only, factworks'
// English-only, and the bare search.json carries no body for either.
func personioFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/personio/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func personioXMLDisabled(board string) error {
	return &StatusError{Method: "GET", Code: 404, URL: "https://" + board + ".jobs.personio.com/xml"}
}

func TestPersonioFetchFallsBackToSearchJSONWhenXMLFeedIsDisabled(t *testing.T) {
	fake := (&routedHTTP{}).
		routeErr("/xml", personioXMLDisabled("aconium")).
		route("search.json?language=de", personioFixture(t, "aconium_search_de.json")).
		route("search.json?language=en", personioFixture(t, "aconium_search_en.json"))

	jobs, err := NewPersonio(fake).Fetch(context.Background(), CompanyEntry{
		Company: "aconium", Provider: "personio", Board: "aconium",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2", len(jobs))
	}
	j := jobs[0]
	// The same posting id the XML feed carries, so a tenant that turns its feed back on (or
	// off) keeps the identity (source, external_id) of every posting it already has.
	if j.ExternalID != "1589998" {
		t.Errorf("ExternalID = %q, want the posting id", j.ExternalID)
	}
	if j.URL != "https://aconium.jobs.personio.com/job/1589998" {
		t.Errorf("URL = %q, want the same constructed job URL as the XML path", j.URL)
	}
	if !strings.HasPrefix(j.Title, "Informationssicherheitsbeauftragte:r") {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Location != "Berlin - Invalidenstraße" {
		t.Errorf("Location = %q, want the office", j.Location)
	}
	for _, want := range []string{"ISO 27001", "Für unser Berliner Büro"} {
		if !strings.Contains(j.Description, want) {
			t.Errorf("Description missing %q, got %q", want, j.Description)
		}
	}
	if j.Remote {
		t.Error("Remote = true for a Berlin office")
	}
}

func TestPersonioSearchJSONFillsEnglishOnlyPostingsFromTheEnglishLocale(t *testing.T) {
	fake := (&routedHTTP{}).
		routeErr("/xml", personioXMLDisabled("factworks-gmbh")).
		route("search.json?language=de", personioFixture(t, "factworks_search_de.json")).
		route("search.json?language=en", personioFixture(t, "factworks_search_en.json"))

	jobs, err := NewPersonio(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Factworks", Provider: "personio", Board: "factworks-gmbh",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("len(jobs) = %d, want 3", len(jobs))
	}
	for _, j := range jobs {
		if j.Description == "" {
			t.Errorf("job %s (%s) has no description; the English locale carries it", j.ExternalID, j.Title)
		}
	}
	if jobs[2].ExternalID != "2826679" || jobs[2].Title != "Senior IT System Administrator (m/f/d)" {
		t.Errorf("jobs[2] = %s %q", jobs[2].ExternalID, jobs[2].Title)
	}
}

// The fallback listing is the board's only listing once the feed is off, so its failure
// must fail the Fetch exactly as the XML feed's does — fullBoardListing rests on it.
func TestPersonioSearchJSONListingErrorFailsTheFetch(t *testing.T) {
	fake := (&routedHTTP{}).
		routeErr("/xml", personioXMLDisabled("acme")).
		routeErr("search.json", errors.New("boom"))
	if _, err := NewPersonio(fake).Fetch(context.Background(), CompanyEntry{Board: "acme"}); err == nil {
		t.Fatal("Fetch succeeded although both listings failed")
	}
}

// Only a 404 means "the feed is switched off". A 5xx or a timeout on /xml is a transient
// failure of the primary listing and must surface as one, not be papered over by a second
// listing that would then be read as the board's whole state.
func TestPersonioDoesNotFallBackOnATransientXMLFailure(t *testing.T) {
	fake := (&routedHTTP{}).
		routeErr("/xml", &StatusError{Method: "GET", Code: 503, URL: "https://acme.jobs.personio.com/xml"}).
		route("search.json", `[{"id":1,"name":"x","office":"Berlin","description":""}]`)
	if _, err := NewPersonio(fake).Fetch(context.Background(), CompanyEntry{Board: "acme"}); err == nil {
		t.Fatal("Fetch succeeded on a 503 from the XML feed")
	}
}
