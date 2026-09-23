package sources

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
)

// jcListingHTML builds the careers listing, which links each vacancy by slug. The decorative
// href-less references are deliberate: the real page styles its view switcher with
// background-image: url('/vacancies/grid_icon.svg'), so a listing scan that matches the path
// anywhere rather than in an anchor collects icons as vacancies.
func jcListingHTML(slugs ...string) string {
	var b strings.Builder
	b.WriteString(`<html><head><style>` +
		`.grid{background-image:url('/vacancies/grid_icon_active.svg')}` +
		`.list{background-image:url('/vacancies/list_icon.svg')}` +
		`</style></head><body><ul>`)
	for _, s := range slugs {
		b.WriteString(`<li><a href="/vacancies/` + s + `">A role</a></li>`)
	}
	b.WriteString(`</ul></body></html>`)
	return b.String()
}

// jcDetailHTML builds a vacancy page carrying the Apollo cache Next.js embeds. content is the
// Editor.js block array the site stores a description as, passed verbatim.
func jcDetailHTML(id int, name, slug, content string) string {
	state := map[string]any{
		"Category:19": map[string]any{"__typename": "Category"},
		"Vacancy:" + strconv.Itoa(id): map[string]any{
			"__typename": "Vacancy",
			"id":         id,
			"name":       name,
			"slug":       slug,
			"content":    content,
		},
	}
	return jcPageHTML(state)
}

// jcPageHTML wraps an Apollo cache in the page shape the adapter reads.
func jcPageHTML(state map[string]any) string {
	payload, err := json.Marshal(map[string]any{
		"props": map[string]any{"pageProps": map[string]any{"initialApolloState": state}},
	})
	if err != nil {
		panic(err)
	}
	return `<html><body><script id="__NEXT_DATA__" type="application/json">` +
		string(payload) + `</script></body></html>`
}

// jcBlocks renders an Editor.js block array as the site stores it: a JSON STRING holding JSON.
func jcBlocks(blocks ...map[string]any) string {
	raw, err := json.Marshal(blocks)
	if err != nil {
		panic(err)
	}
	return string(raw)
}

func jcParagraph(text string) map[string]any {
	return map[string]any{"type": "paragraph", "data": map[string]any{"text": text}}
}

func TestJettyCloudFetch(t *testing.T) {
	body := jcBlocks(
		jcParagraph("We are looking for an experienced SRE."),
		map[string]any{"type": "header", "data": map[string]any{"text": "What you will do", "level": 2}},
		map[string]any{"type": "list", "data": map[string]any{"items": []string{"Run things", "Fix things"}}},
	)
	routes := (&routedHTTP{}).
		route("/vacancies?", jcListingHTML("site-reliability-engineer-telco-team")).
		route("/vacancies/site-reliability-engineer-telco-team",
			jcDetailHTML(490, "Site Reliability Engineer (Telco team)", "site-reliability-engineer-telco-team", body))

	jobs, err := NewJettyCloud(routes).Fetch(context.Background(), CompanyEntry{Company: "JettyCloud"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs (%v), want 1", len(jobs), jobs)
	}
	j := jobs[0]
	if j.ExternalID != "490" {
		t.Errorf("ExternalID = %q, want the numeric vacancy id", j.ExternalID)
	}
	if j.Title != "Site Reliability Engineer (Telco team)" {
		t.Errorf("Title = %q", j.Title)
	}
	if want := "https://www.jettycloud.com/vacancies/site-reliability-engineer-telco-team"; j.URL != want {
		t.Errorf("URL = %q, want %q", j.URL, want)
	}
	if j.Company != "JettyCloud" {
		t.Errorf("Company = %q", j.Company)
	}
	for _, want := range []string{"experienced SRE", "What you will do", "Run things", "Fix things"} {
		if !strings.Contains(j.Description, want) {
			t.Errorf("Description missing %q; got %q", want, j.Description)
		}
	}
}

// A vacancy page's Apollo cache holds its RELATED vacancies too, so "the Vacancy entry" is not
// a thing — there are several, and Go's map iteration order is deliberately random, so picking
// the first one makes the adapter return a different posting run to run. Found on the live site:
// two of the three pages resolved to the same id, one of them with an empty description.
// The page's own slug is what identifies its posting.
func TestJettyCloudPicksThePageOwnVacancyNotARelatedOne(t *testing.T) {
	state := map[string]any{
		"Vacancy:367": map[string]any{
			"__typename": "Vacancy", "id": 367, "name": "AppSec Automation Engineer", "slug": "appsec-automation-engineer",
			// A related vacancy is a teaser: the cache carries its identity, not its body.
			"content": "",
		},
		"Vacancy:490": map[string]any{
			"__typename": "Vacancy", "id": 490, "name": "Site Reliability Engineer", "slug": "site-reliability-engineer",
			"content": jcBlocks(jcParagraph("The body of the posting this page is about.")),
		},
	}
	routes := (&routedHTTP{}).
		route("/vacancies?", jcListingHTML("site-reliability-engineer")).
		route("/vacancies/site-reliability-engineer", jcPageHTML(state))

	jobs, err := NewJettyCloud(routes).Fetch(context.Background(), CompanyEntry{Company: "JettyCloud"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs (%v), want 1", len(jobs), jobs)
	}
	if jobs[0].ExternalID != "490" {
		t.Fatalf("got id %q, want 490 — the page's own vacancy, not a related one", jobs[0].ExternalID)
	}
	if !strings.Contains(jobs[0].Description, "The body of the posting") {
		t.Errorf("Description = %q, want the page's own posting body", jobs[0].Description)
	}
}

// A closed vacancy is the trap this source sets: its page answers 200 with the ordinary layout
// and an Apollo cache holding no Vacancy at all. Reading that as a posting would invent a job
// with no id and no title; reading it as a failure would fail every crawl that happens to race
// a closure. It is neither — the listing is the source of truth, so the page is skipped.
func TestJettyCloudSkipsAClosedVacancy(t *testing.T) {
	routes := (&routedHTTP{}).
		route("/vacancies?", jcListingHTML("go-developer", "incident-response-lead")).
		route("/vacancies/go-developer", jcPageHTML(map[string]any{"Category:19": map[string]any{"__typename": "Category"}})).
		route("/vacancies/incident-response-lead",
			jcDetailHTML(491, "Incident Response Lead", "incident-response-lead", jcBlocks(jcParagraph("Lead it."))))

	jobs, err := NewJettyCloud(routes).Fetch(context.Background(), CompanyEntry{Company: "JettyCloud"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "491" {
		t.Fatalf("got %v, want only the vacancy that still carries data", jobs)
	}
}

// An unknown Editor.js block must cost its own text, never the whole description: the site
// authors these by hand and a new block type is a content decision, not a crawl failure.
func TestJettyCloudKeepsTheDescriptionAroundAnUnknownBlock(t *testing.T) {
	body := jcBlocks(
		jcParagraph("Before."),
		map[string]any{"type": "carousel", "data": map[string]any{"images": []string{"a.png"}}},
		jcParagraph("After."),
	)
	routes := (&routedHTTP{}).
		route("/vacancies?", jcListingHTML("role")).
		route("/vacancies/role", jcDetailHTML(7, "Role", "role", body))

	jobs, err := NewJettyCloud(routes).Fetch(context.Background(), CompanyEntry{Company: "JettyCloud"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	for _, want := range []string{"Before.", "After."} {
		if !strings.Contains(jobs[0].Description, want) {
			t.Errorf("Description missing %q; got %q", want, jobs[0].Description)
		}
	}
}

// A listing that links nothing is a re-templated page or a block, not an employer who closed
// every role — and read as the latter it would retire the whole company from the catalogue.
func TestJettyCloudEmptyListingIsAnError(t *testing.T) {
	routes := (&routedHTTP{}).route("/vacancies?", jcListingHTML())

	if _, err := NewJettyCloud(routes).Fetch(context.Background(), CompanyEntry{Company: "JettyCloud"}); err == nil {
		t.Fatal("Fetch succeeded on a listing carrying no vacancies, want an error")
	}
}

// The adapter is single-company: it has no board to be keyed by.
func TestJettyCloudIsBoardless(t *testing.T) {
	if _, ok := NewJettyCloud(&routedHTTP{}).(boardless); !ok {
		t.Fatal("jettycloud must be boardless — it serves one employer and its config carries no board")
	}
}
