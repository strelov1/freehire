package sources

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"
)

// btListingHTML builds a Betterteam root listing page linking each given job href.
func btListingHTML(jobHrefs ...string) string {
	var b strings.Builder
	b.WriteString(`<html><body><ul class="jobs">`)
	for _, h := range jobHrefs {
		b.WriteString(`<li><a href="` + h + `">A job</a></li>`)
	}
	b.WriteString(`</ul></body></html>`)
	return b.String()
}

// btDetailHTML builds a Betterteam job page carrying a schema.org JobPosting ld+json block.
// jobLocation is a single Place object and there is no remote flag.
func btDetailHTML(title, encodedDescription, datePosted, locality, country string) string {
	return `<html><head><script type="application/ld+json">` +
		`{"@context":"https://schema.org/","@type":"JobPosting",` +
		`"title":"` + title + `",` +
		`"description":"` + encodedDescription + `",` +
		`"datePosted":"` + datePosted + `",` +
		`"employmentType":"FULL_TIME",` +
		`"hiringOrganization":{"@type":"Organization","name":"110 Grill"},` +
		`"jobLocation":{"@type":"Place","address":{"addressLocality":"` + locality + `","addressCountry":"` + country + `"}}}` +
		`</script></head><body></body></html>`
}

func TestBetterteamProvider(t *testing.T) {
	if got := NewBetterteam(nil).Provider(); got != "betterteam" {
		t.Errorf("Provider() = %q, want %q", got, "betterteam")
	}
}

func TestBTJobID(t *testing.T) {
	cases := map[string]string{
		"https://110grill.betterteam.com/bartender-21": "bartender-21",
		"/culinary-manager-39":                         "culinary-manager-39",
		"https://x.betterteam.com/line-cook-113":       "line-cook-113",
		"https://x.betterteam.com/":                    "", // listing root, no id
		"/about-us":                                    "", // no trailing numeric id
		"/careers":                                     "", // no trailing id
		"https://x.betterteam.com/a/b-12":              "", // not a single segment
	}
	for u, want := range cases {
		if got := btJobID(u); got != want {
			t.Errorf("btJobID(%q) = %q, want %q", u, got, want)
		}
	}
}

func TestBTJobLinksResolvesRelativeHrefs(t *testing.T) {
	// The live listing emits relative "/<slug>-<id>" hrefs; each must resolve to an
	// absolute, fetchable URL against the root, and a repeated job de-dups to one.
	h := btListingHTML("/bartender-21", "/bartender-21", "/culinary-manager-39") +
		`<a href="/about-us">About</a>`
	base := mustURL(t, "https://110grill.betterteam.com/")
	got := jobLinks(base, parseHTML(t, h), func(href string) bool { return btJobID(href) != "" })
	want := []string{
		"https://110grill.betterteam.com/bartender-21",
		"https://110grill.betterteam.com/culinary-manager-39",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("jobLinks() = %v, want %v", got, want)
	}
}

func TestBetterteamFetchListingThenDetailAndMaps(t *testing.T) {
	jobURL := "https://110grill.betterteam.com/bartender-21"
	detail := btDetailHTML(
		"Bartender",
		"&lt;div&gt;Pour &lt;b&gt;drinks&lt;/b&gt;.&lt;/div&gt;&lt;script&gt;alert(1)&lt;/script&gt;",
		"2026-06-09T15:07:41.914471Z", "North Conway", "US")
	fake := (&routedHTTP{}).
		route("/bartender-21", detail). // detail route first: the listing root has no slug-id
		route("110grill.betterteam.com/", btListingHTML("/bartender-21"))

	jobs, err := NewBetterteam(fake).Fetch(context.Background(), CompanyEntry{
		Company: "110 Grill", Provider: "betterteam", Board: "110grill",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "bartender-21" {
		t.Errorf("ExternalID = %q, want bartender-21", j.ExternalID)
	}
	if j.URL != jobURL {
		t.Errorf("URL = %q, want %q", j.URL, jobURL)
	}
	if j.Title != "Bartender" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "110 Grill" {
		t.Errorf("Company = %q, want '110 Grill'", j.Company)
	}
	if j.Location != "North Conway, US" {
		t.Errorf("Location = %q, want 'North Conway, US'", j.Location)
	}
	if strings.Contains(j.Description, "<script>") ||
		!strings.Contains(j.Description, "<div>") || !strings.Contains(j.Description, "<b>drinks</b>") {
		t.Errorf("Description not unescaped/sanitized: %q", j.Description)
	}
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2026, 6, 9, 15, 7, 41, 914471000, time.UTC)) {
		t.Errorf("PostedAt = %v, want 2026-06-09T15:07:41.914471Z", j.PostedAt)
	}
}

func TestBetterteamDropsDetailWithoutJobPosting(t *testing.T) {
	// A job page carrying no schema.org JobPosting drops just that posting; the rest of the
	// board still ingests.
	d := btDetailHTML("Cook", "&lt;div&gt;x&lt;/div&gt;", "2026-06-09T15:07:41Z", "Boston", "US")
	fake := (&routedHTTP{}).
		route("/line-cook-113", d).
		route("/line-cook-114", `<html><body>no ld+json here</body></html>`).
		route("110grill.betterteam.com/", btListingHTML("/line-cook-113", "/line-cook-114"))

	jobs, err := NewBetterteam(fake).Fetch(context.Background(), CompanyEntry{
		Company: "110 Grill", Board: "110grill",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (one detail dropped)", len(jobs))
	}
}
