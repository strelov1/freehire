package sources

import (
	"context"
	"errors"
	"slices"
	"testing"
)

func TestHimalayasProvider(t *testing.T) {
	if got := NewHimalayas(nil).Provider(); got != "himalayas" {
		t.Errorf("Provider() = %q, want himalayas", got)
	}
}

func TestHimalayasIsBoardlessAggregator(t *testing.T) {
	s := NewHimalayas(nil)
	if _, ok := s.(boardless); !ok {
		t.Error("himalayas should implement the boardless marker")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("himalayas should implement the aggregator marker")
	}
}

func TestHimalayasRegisteredAndFilterable(t *testing.T) {
	if _, ok := All(nil)["himalayas"]; !ok {
		t.Error("All() should register provider himalayas")
	}
	if !slices.Contains(FilterableProviders(), "himalayas") {
		t.Error("FilterableProviders() should include himalayas")
	}
}

func TestHimalayasBoardFileValidates(t *testing.T) {
	cfg, err := LoadConfig("../../sources/himalayas.yml")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if err := cfg.Validate(All(nil)); err != nil {
		t.Fatalf("sources/himalayas.yml fails validation: %v", err)
	}
}

func TestHimalayasFetchPaginatesAndMaps(t *testing.T) {
	// totalCount (3) exceeds the first page, so the adapter must fetch a second offset page.
	// The offset advances by the count actually returned (page1 has 2 postings → next
	// offset is 2), not by the requested limit — Himalayas caps the page size below it.
	page1 := `{"totalCount":3,"jobs":[
{"title":"Web Engineer","companyName":"KraftPixel","applicationLink":"https://himalayas.app/companies/kraftpixel/jobs/web-engineer","guid":"https://himalayas.app/companies/kraftpixel/jobs/web-engineer","locationRestrictions":["United States","Canada"],"description":"<p>Build web.</p>","pubDate":1747699200},
{"title":"NoGUID drop","companyName":"Ghost","guid":""}
]}`
	page2 := `{"totalCount":3,"jobs":[
{"title":"Data Analyst","companyName":"Peroptyx","applicationLink":"https://himalayas.app/companies/peroptyx/jobs/data-analyst","guid":"https://himalayas.app/companies/peroptyx/jobs/data-analyst","locationRestrictions":["Ireland"],"description":"<p>Analyze.</p>","pubDate":1781725000}
]}`
	fake := (&routedHTTP{}).route("offset=2", page2).route("offset=0", page1)
	jobs, err := NewHimalayas(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if fake.calls != 2 {
		t.Errorf("made %d requests, want 2 (one per offset page)", fake.calls)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2 (empty-guid dropped)", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "https://himalayas.app/companies/kraftpixel/jobs/web-engineer" {
		t.Errorf("ExternalID = %q, want the guid", j.ExternalID)
	}
	if j.Company != "KraftPixel" || j.Title != "Web Engineer" {
		t.Errorf("bad mapping: %+v", j)
	}
	if j.URL != "https://himalayas.app/companies/kraftpixel/jobs/web-engineer" {
		t.Errorf("URL = %q, want applicationLink", j.URL)
	}
	if j.Location != "United States, Canada" {
		t.Errorf("Location = %q, want joined locationRestrictions", j.Location)
	}
	if !j.Remote || j.WorkMode != "remote" {
		t.Errorf("Remote=%v WorkMode=%q, want remote (himalayas is remote-only)", j.Remote, j.WorkMode)
	}
	if j.PostedAt == nil {
		t.Error("PostedAt nil, want parsed epoch seconds")
	}
}

func TestHimalayasCompanyNameSentinelFallsBackToCompanySlug(t *testing.T) {
	// Himalayas' feed renders companyName as the literal "name" for some postings (an
	// unresolved template field on their end). companySlug, a separate JSON field, stays
	// correct even then, so toJob recovers the company from it instead of storing the
	// literal sentinel. Confirmed live: guid also carries a stray leading ":" on these rows
	// (a second, correlated glitch), which is exactly why companySlug — not guid parsing —
	// is the fallback.
	p := himalayasPosting{
		Title:       "Channel Sales Manager (East)",
		CompanyName: himalayasCompanyNameSentinel,
		CompanySlug: "freshworks",
		GUID:        ":https://himalayas.app/companies/freshworks/jobs/channel-sales-manager-east-9054304068",
	}
	j, ok := p.toJob()
	if !ok {
		t.Fatal("toJob() ok = false, want true (companySlug yields a recoverable company)")
	}
	if j.Company != "freshworks" {
		t.Errorf("Company = %q, want %q (recovered from companySlug)", j.Company, "freshworks")
	}
}

func TestHimalayasCompanyNameSentinelDropsWhenCompanySlugAlsoMissing(t *testing.T) {
	// When companySlug is empty too, there is nothing usable to fall back to — drop the
	// posting rather than store the literal "name" as the company.
	p := himalayasPosting{
		Title:       "Operations Manager",
		CompanyName: himalayasCompanyNameSentinel,
		GUID:        "https://himalayas.app/companies/somewhere/jobs/operations-manager",
	}
	if _, ok := p.toJob(); ok {
		t.Error("toJob() ok = true, want false (no companySlug to recover the company from)")
	}
}

func TestHimalayasReturnsPartialOnPageError(t *testing.T) {
	// Himalayas rate-limits (429) mid-crawl. A page failure after we have already collected
	// jobs must return the partial result, not discard everything: the first page succeeds,
	// the second (offset=2) has no route → the fake errors, and Fetch returns page 1's job.
	page1 := `{"totalCount":999,"jobs":[
{"title":"Web Engineer","companyName":"KraftPixel","applicationLink":"https://himalayas.app/x","guid":"https://himalayas.app/x","pubDate":1747699200},
{"title":"Data Analyst","companyName":"Peroptyx","applicationLink":"https://himalayas.app/y","guid":"https://himalayas.app/y","pubDate":1747699200}
]}`
	fake := (&routedHTTP{}).route("offset=0", page1) // offset=2 has no route → error
	jobs, err := NewHimalayas(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch should swallow a mid-crawl page error, got: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2 (partial result from the first page before the error)", len(jobs))
	}
}

func TestHimalayasErrorsWhenFirstPageFails(t *testing.T) {
	// A failure on the very first page yields no jobs at all, so it is a genuine board error
	// (not a partial result to keep).
	fake := &fakeHTTP{err: errors.New("boom")}
	if _, err := NewHimalayas(fake).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Error("Fetch should return an error when the first page fails")
	}
}

func TestStripHimalayasSelfPromo(t *testing.T) {
	// The trailer reaches this function in three shapes: unwrapped (what sanitizeHTML now
	// hands the adapter), and still linked with or without the rel="nofollow" the sanitizer
	// used to add — which is how the catalogue rows stored before the link strip look.
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "trailer as sanitized today (link already unwrapped)",
			in:   `<p>Apply today.</p><p>Originally posted on Himalayas</p>`,
			want: `<p>Apply today.</p>`,
		},
		{
			name: "trailer as stored before the link strip (rel=nofollow)",
			in:   `<p>Apply today.</p><p>Originally posted on <a href="https://himalayas.app" rel="nofollow">Himalayas</a></p>`,
			want: `<p>Apply today.</p>`,
		},
		{
			name: "trailer as served (raw feed, no rel)",
			in:   `<p>Apply today.</p><p>Originally posted on <a href="https://himalayas.app">Himalayas</a></p>`,
			want: `<p>Apply today.</p>`,
		},
		{
			name: "a body that merely names the mountains is not a promo trailer",
			in:   `<p>Lead treks across the Himalayas.</p>`,
			want: `<p>Lead treks across the Himalayas.</p>`,
		},
		{
			name: "a clean body is returned byte-for-byte",
			in:   `<p>Build web.</p>`,
			want: `<p>Build web.</p>`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := StripHimalayasSelfPromo(tc.in); got != tc.want {
				t.Errorf("StripHimalayasSelfPromo():\n got %q\nwant %q", got, tc.want)
			}
		})
	}
}

func TestStripHimalayasSelfPromoIsIdempotent(t *testing.T) {
	// The backfill re-runs over rows a previous run already cleaned, so a second pass must
	// rewrite nothing.
	in := `<p>At PTC, we build.</p><p>Originally posted on <a href="https://himalayas.app" rel="nofollow">Himalayas</a></p>`
	once := StripHimalayasSelfPromo(in)
	if twice := StripHimalayasSelfPromo(once); twice != once {
		t.Errorf("second pass changed the body:\n got %q\nwant %q", twice, once)
	}
}

func TestHimalayasFetchStripsSelfPromo(t *testing.T) {
	// The adapter must yield an already-clean body, so a re-crawl writes the same text the
	// backfill leaves behind.
	page := `{"totalCount":1,"jobs":[
{"title":"Web Engineer","companyName":"KraftPixel","applicationLink":"https://himalayas.app/x","guid":"https://himalayas.app/x","description":"<p>Join <a href=\"https://himalayas.app/companies/kraftpixel\">KraftPixel</a>.</p><p>Originally posted on <a href=\"https://himalayas.app\">Himalayas</a></p>","pubDate":1747699200}
]}`
	jobs, err := NewHimalayas((&routedHTTP{}).route("offset=0", page)).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	if want := `<p>Join KraftPixel.</p>`; jobs[0].Description != want {
		t.Errorf("Description = %q, want %q", jobs[0].Description, want)
	}
}
