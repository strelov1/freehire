package sources

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

// sgListingHTML builds a softgarden root listing page linking each given job href (the live
// markup emits relative "../job/…" hrefs).
func sgListingHTML(jobHrefs ...string) string {
	var b strings.Builder
	b.WriteString(`<html><body><div class="jobList">`)
	for _, h := range jobHrefs {
		b.WriteString(`<div class="matchValue title"><a href="` + h + `" target="_blank">A job</a></div>`)
	}
	b.WriteString(`</div></body></html>`)
	return b.String()
}

// sgDetailHTML builds a softgarden job page carrying a schema.org JobPosting ld+json block.
// jobLocation is a single Place object and there is no remote flag.
func sgDetailHTML(title, encodedDescription, datePosted, locality, country string) string {
	return `<html><head><script type="application/ld+json">` +
		`{"@context":"http://schema.org/","@type":"JobPosting",` +
		`"title":"` + title + `",` +
		`"description":"` + encodedDescription + `",` +
		`"datePosted":"` + datePosted + `",` +
		`"employmentType":["FULL_TIME"],` +
		`"hiringOrganization":{"@type":"Organization","name":"Bundesdruckerei"},` +
		`"jobLocation":{"@type":"Place","address":{"addressLocality":"` + locality + `","addressCountry":"` + country + `"}}}` +
		`</script></head><body></body></html>`
}

var sgNoFeed = &StatusError{Method: "GET", Code: 404, URL: "https://x.career.softgarden.de/jobs.feed.json"}

func TestSoftgardenProvider(t *testing.T) {
	if got := NewSoftgarden(nil).Provider(); got != "softgarden" {
		t.Errorf("Provider() = %q, want %q", got, "softgarden")
	}
}

func TestSGJobID(t *testing.T) {
	cases := map[string]string{
		"https://bundesdruckerei.softgarden.io/job/66221618/creative-producer?jobDbPVId=279489843&l=en": "66221618",
		"../job/66221618/creative-producer?jobDbPVId=279489843":                                         "66221618",
		"https://b.softgarden.io/job/12345":                                                             "12345", // bare id, no slug/query
		"https://b.softgarden.io/":                                                                      "",      // listing root, no id
		"https://b.softgarden.io/imprint":                                                               "",
		"https://b.softgarden.io/job/abc/x":                                                             "", // non-numeric id
	}
	for u, want := range cases {
		if got := sgJobID(u); got != want {
			t.Errorf("sgJobID(%q) = %q, want %q", u, got, want)
		}
	}
}

func TestSGJobLinksResolvesRelativeHrefs(t *testing.T) {
	// The live listing emits relative "../job/…" hrefs; each must resolve to an absolute,
	// fetchable URL against the root, and a repeated job de-dups to one.
	h := sgListingHTML(
		"../job/66221618/creative-producer?jobDbPVId=279489843&l=en",
		"../job/66221618/creative-producer?jobDbPVId=279489843&l=en",
		"../job/70000001/data-engineer?jobDbPVId=280000000",
	) + `<a href="../imprint">Imprint</a>`
	base := mustURL(t, "https://bundesdruckerei.softgarden.io/")
	got := jobLinks(base, parseHTML(t, h), func(href string) bool { return sgJobID(href) != "" })
	want := []string{
		"https://bundesdruckerei.softgarden.io/job/66221618/creative-producer?jobDbPVId=279489843&l=en",
		"https://bundesdruckerei.softgarden.io/job/70000001/data-engineer?jobDbPVId=280000000",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("jobLinks() = %v, want %v", got, want)
	}
}

func TestSoftgardenFetchListingThenDetailAndMaps(t *testing.T) {
	jobURL := "https://bundesdruckerei.softgarden.io/job/66221618/creative-producer?jobDbPVId=279489843"
	detail := sgDetailHTML(
		"Creative Producer",
		"&lt;p&gt;Build &lt;b&gt;it&lt;/b&gt;.&lt;/p&gt;&lt;script&gt;alert(1)&lt;/script&gt;",
		"2026-07-21T12:43:08.524+02:00", "Berlin", "Deutschland")
	fake := (&routedHTTP{}).
		routeErr("career.softgarden.de", sgNoFeed). // no feed for this tenant → legacy path
		route("/job/66221618", detail).             // detail route first: the listing root has no /job/
		route("bundesdruckerei.softgarden.io/", sgListingHTML(
			"../job/66221618/creative-producer?jobDbPVId=279489843"))

	jobs, err := NewSoftgarden(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Bundesdruckerei", Provider: "softgarden", Board: "bundesdruckerei",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "66221618" {
		t.Errorf("ExternalID = %q, want 66221618", j.ExternalID)
	}
	if j.URL != jobURL {
		t.Errorf("URL = %q, want %q", j.URL, jobURL)
	}
	if j.Title != "Creative Producer" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "Bundesdruckerei" {
		t.Errorf("Company = %q, want Bundesdruckerei", j.Company)
	}
	if j.Location != "Berlin, Deutschland" {
		t.Errorf("Location = %q, want 'Berlin, Deutschland'", j.Location)
	}
	if strings.Contains(j.Description, "<script>") ||
		!strings.Contains(j.Description, "<p>") || !strings.Contains(j.Description, "<b>it</b>") {
		t.Errorf("Description not unescaped/sanitized: %q", j.Description)
	}
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2026, 7, 21, 10, 43, 8, 524_000_000, time.UTC)) {
		t.Errorf("PostedAt = %v, want 2026-07-21T12:43:08.524+02:00", j.PostedAt)
	}
}

func TestSoftgardenDropsDetailWithoutJobPosting(t *testing.T) {
	// A job page carrying no schema.org JobPosting drops just that posting; the rest of the
	// board still ingests.
	d := sgDetailHTML("Role", "&lt;p&gt;x&lt;/p&gt;", "2026-07-21T12:43:08.524+02:00", "Oslo", "Norway")
	fake := (&routedHTTP{}).
		routeErr("career.softgarden.de", sgNoFeed).
		route("/job/111", d).                                           // first posting resolves
		route("/job/222", `<html><body>no ld+json here</body></html>`). // second has no JobPosting → drops
		route("bundesdruckerei.softgarden.io/", sgListingHTML(
			"../job/111/role-a",
			"../job/222/role-b"))

	jobs, err := NewSoftgarden(fake).Fetch(context.Background(), CompanyEntry{
		Company: "B", Board: "bundesdruckerei",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (one detail dropped)", len(jobs))
	}
}

const sgFeedJSON = `{"@type":"DataFeed","numberOfItems":2,"dataFeedElement":[
 {"@type":"DataFeedItem","item":{"@type":"JobPosting","title":"Senior Account Executive",
  "url":"https://acme.career.softgarden.de/jobs/62349294/Senior-Account-Executive/",
  "datePosted":"2026-09-23T10:37:06.121+02:00","identifier":{"@type":"PropertyValue","value":62349294},
  "description":"<b>Your role</b><ul><li>Sell</li></ul><script>alert(1)</script>",
  "jobLocation":{"@type":"Place","address":{"addressLocality":"Berlin","addressCountry":"Deutschland"}}}},
 {"@type":"DataFeedItem","item":{"@type":"JobPosting","title":"Engineer",
  "url":"https://acme.career.softgarden.de/jobs/63710444/Engineer/",
  "datePosted":"2026-10-01T08:00:00.000+02:00","identifier":{"@type":"PropertyValue","value":63710444},
  "description":"<p>Build</p>",
  "jobLocation":[{"address":{"addressLocality":"Hamburg","addressCountry":"Deutschland"}},
                 {"address":{"addressLocality":"Remote","addressCountry":"Deutschland"}},
                 {"address":{"addressLocality":"Hamburg","addressCountry":"Deutschland"}}]}}
]}`

func TestSoftgardenFetchesWholeBoardFromTheFeed(t *testing.T) {
	fake := (&routedHTTP{}).route("acme.career.softgarden.de/jobs.feed.json", sgFeedJSON)

	jobs, err := NewSoftgarden(fake).Fetch(context.Background(), CompanyEntry{Company: "Acme", Board: "acme"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if fake.calls != 1 {
		t.Errorf("requests = %d, want 1 (the feed carries the bodies)", fake.calls)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "62349294" || j.Title != "Senior Account Executive" || j.Company != "Acme" {
		t.Errorf("job = %+v", j)
	}
	if j.URL != "https://acme.career.softgarden.de/jobs/62349294/Senior-Account-Executive/" {
		t.Errorf("URL = %q", j.URL)
	}
	if j.Location != "Berlin, Deutschland" {
		t.Errorf("Location = %q", j.Location)
	}
	if strings.Contains(j.Description, "<script>") || !strings.Contains(j.Description, "<li>Sell</li>") {
		t.Errorf("Description not sanitized: %q", j.Description)
	}
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2026, 9, 23, 8, 37, 6, 121_000_000, time.UTC)) {
		t.Errorf("PostedAt = %v", j.PostedAt)
	}
	// An array jobLocation joins its distinct places.
	if jobs[1].Location != "Hamburg, Deutschland; Remote, Deutschland" {
		t.Errorf("multi-location = %q", jobs[1].Location)
	}
}

func TestSoftgardenFeedErrorOtherThanMissingDoesNotFallBack(t *testing.T) {
	// A 500 on the feed is a failed crawl, not "this tenant has no feed": falling back would
	// mask an outage behind a truncated one-page listing.
	fake := (&routedHTTP{}).routeErr("career.softgarden.de", &StatusError{Method: "GET", Code: 500})

	_, err := NewSoftgarden(fake).Fetch(context.Background(), CompanyEntry{Board: "acme"})
	var se *StatusError
	if !errors.As(err, &se) || se.Code != 500 {
		t.Fatalf("err = %v, want the 500 StatusError", err)
	}
}

func TestSoftgardenRejectsBoardThatIsNotAHostLabel(t *testing.T) {
	if _, err := NewSoftgarden(&routedHTTP{}).Fetch(context.Background(), CompanyEntry{Board: "evil.com/x"}); err == nil {
		t.Fatal("want an error for a board that cannot be a hostname label")
	}
}

func TestSoftgardenFeedPostingWithoutIDFailsTheRead(t *testing.T) {
	// A partial list returned as a success would let the unseen-sweep close the posting the
	// id-less item stands for.
	feed := `{"dataFeedElement":[
	 {"item":{"title":"Fine","url":"https://acme.career.softgarden.de/jobs/1/","identifier":{"value":1}}},
	 {"item":{"title":"No id","url":"https://acme.career.softgarden.de/jobs/x/","identifier":{}}}]}`
	fake := (&routedHTTP{}).route("career.softgarden.de", feed)

	if jobs, err := NewSoftgarden(fake).Fetch(context.Background(), CompanyEntry{Board: "acme"}); err == nil {
		t.Fatalf("got %d jobs and no error, want the read to fail", len(jobs))
	}
}

func TestSoftgardenFeedAcceptsAnUppercaseBoard(t *testing.T) {
	fake := (&routedHTTP{}).route("acme.career.softgarden.de/jobs.feed.json", sgFeedJSON)

	jobs, err := NewSoftgarden(fake).Fetch(context.Background(), CompanyEntry{Board: "Acme"})
	if err != nil || len(jobs) != 2 {
		t.Fatalf("jobs=%d err=%v, want 2 jobs: DNS labels are case-insensitive", len(jobs), err)
	}
}
