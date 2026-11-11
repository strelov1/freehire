package sources

import (
	"context"
	"encoding/json"
	"fmt"
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

func btNativeListingHTML(slugs ...string) string {
	nodes := []any{map[string]int{"_1": 2}, "loaderData", map[string]int{"_3": 4}, "routes/CareersPage", map[string]int{"_5": 6}, "jobs", nil, "slug"}
	refs := make([]int, 0, len(slugs))
	for _, slug := range slugs {
		refs = append(refs, len(nodes))
		nodes = append(nodes, map[string]int{"_7": len(nodes) + 1}, slug)
	}
	nodes[6] = refs
	payload, _ := json.Marshal(nodes)
	encoded, _ := json.Marshal(string(payload))
	return `<script>window.__reactRouterContext.streamController.enqueue(` + string(encoded) + `);</script>`
}

func TestBetterteamNativeListingPreservesPlatformSlugs(t *testing.T) {
	detail := btDetailHTML("Expert", "<p>Expert work.</p>", "", "", "")
	fake := (&routedHTTP{}).route("/remote-expert-", detail).route("/markets-expert", detail).
		route("b.betterteam.com/", btNativeListingHTML("remote-expert-(md-do)-44", "markets-expert"))
	jobs, err := NewBetterteam(fake).Fetch(context.Background(), CompanyEntry{Board: "b"})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 || jobs[0].ExternalID != "remote-expert-(md-do)-44" || jobs[1].ExternalID != "markets-expert" {
		t.Fatalf("got %+v, want the two native slugs unchanged", jobs)
	}
}

func TestBetterteamRejectsNativeCeilingAndMalformedState(t *testing.T) {
	slugs := make([]string, 300)
	for i := range slugs {
		slugs[i] = fmt.Sprintf("expert-%d", i)
	}
	for _, body := range []string{btNativeListingHTML(slugs...), `<script>window.__reactRouterContext.streamController.enqueue("[]");</script>`, `<script>window.__reactRouterContext.streamController.enqueue("{}");</script>`, `<script>window.__reactRouterContext.streamController.enqueue(null);</script>`} {
		fake := (&routedHTTP{}).route("b.betterteam.com/", body)
		if _, err := NewBetterteam(fake).Fetch(context.Background(), CompanyEntry{Board: "b"}); err == nil {
			t.Fatal("a capped or unproven native listing was accepted")
		}
	}
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
		"/remote-expert-%28md-do%29-44":                "remote-expert-(md-do)-44",
		"/specialty%2Fradiology-321":                   "specialty/radiology-321",
	}
	for u, want := range cases {
		if got := btJobID(u); got != want {
			t.Errorf("btJobID(%q) = %q, want %q", u, got, want)
		}
	}
}

func TestBetterteamRejectsKnownListingCeiling(t *testing.T) {
	hrefs := make([]string, 300)
	for i := range hrefs {
		hrefs[i] = fmt.Sprintf("/remote-expert-%%28md-do%%29-%d", i)
	}
	fake := (&routedHTTP{}).route("b.betterteam.com/", btListingHTML(hrefs...))
	if _, err := NewBetterteam(fake).Fetch(context.Background(), CompanyEntry{Board: "b"}); err == nil {
		t.Fatal("the known 300-job ceiling was accepted as complete")
	}
	if fake.calls != 1 {
		t.Fatalf("made %d requests, want only the capped listing", fake.calls)
	}
	if _, ok := NewBetterteam(nil).(fullBoardListing); ok {
		t.Fatal("Betterteam must not claim complete listing support")
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
