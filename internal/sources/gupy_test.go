package sources

import (
	"context"
	"strings"
	"testing"
)

func TestGupyProvider(t *testing.T) {
	if got := NewGupy(nil).Provider(); got != "gupy" {
		t.Errorf("Provider() = %q, want %q", got, "gupy")
	}
}

func TestGupyFetchMapsFieldsAndPaginates(t *testing.T) {
	// Page 1 is full (gupyPageLimit items) so the walk continues; page 2 is short so it
	// stops there. pagination.total is deliberately wrong (clamped to the limit) to prove
	// the adapter ignores it and pages by short-page instead.
	var page1 strings.Builder
	page1.WriteString(`{"pagination":{"total":100,"limit":100,"offset":0},"data":[`)
	for i := 0; i < gupyPageLimit; i++ {
		if i > 0 {
			page1.WriteString(",")
		}
		page1.WriteString(`{"id":`)
		page1.WriteString(itoa(1000 + i))
		page1.WriteString(`,"name":"Role","jobUrl":"https://creditas.gupy.io/job/`)
		page1.WriteString(itoa(1000 + i))
		page1.WriteString(`","workplaceType":"on-site"}`)
	}
	page1.WriteString(`]}`)

	page2 := `{"pagination":{"total":100,"limit":100,"offset":100},"data":[
		{"id":2001,"name":"  Backend Engineer  ","jobUrl":"https://creditas.gupy.io/job/2001",
		 "description":"<p>Build things</p><script>evil()</script>","city":"São Paulo","state":"SP","country":"Brasil",
		 "isRemoteWork":false,"workplaceType":"hybrid","publishedDate":"2026-06-11T11:34:10.829Z"},
		{"id":2002,"name":"Remote SRE","jobUrl":"https://creditas.gupy.io/job/2002",
		 "city":"","state":"","country":"Brasil","isRemoteWork":true,"workplaceType":"remote"},
		{"id":2003,"name":"No URL Role","jobUrl":""}
	]}`

	fake := (&routedHTTP{}).
		route("offset=0", page1.String()).
		route("offset=100", page2)

	jobs, err := NewGupy(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Creditas", Provider: "gupy", Board: "85606",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	// 100 from page 1 + 2 from page 2 (the empty-jobUrl posting is dropped) = 102.
	if len(jobs) != gupyPageLimit+2 {
		t.Fatalf("len(jobs) = %d, want %d across two pages with one URL-less posting dropped", len(jobs), gupyPageLimit+2)
	}

	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	be, ok := byID["2001"]
	if !ok {
		t.Fatal("posting 2001 missing")
	}
	if be.Title != "Backend Engineer" {
		t.Errorf("Title = %q, want trimmed name", be.Title)
	}
	if be.URL != "https://creditas.gupy.io/job/2001" {
		t.Errorf("URL = %q, want jobUrl", be.URL)
	}
	if be.Company != "Creditas" {
		t.Errorf("Company = %q, want configured company", be.Company)
	}
	if be.Location != "São Paulo, SP, Brasil" {
		t.Errorf("Location = %q, want city/state/country joined", be.Location)
	}
	if be.WorkMode != "hybrid" {
		t.Errorf("WorkMode = %q, want hybrid from workplaceType", be.WorkMode)
	}
	if !strings.Contains(be.Description, "Build things") {
		t.Errorf("Description missing body, got %q", be.Description)
	}
	if strings.Contains(be.Description, "evil") {
		t.Errorf("Description must be sanitized, got %q", be.Description)
	}
	if be.PostedAt == nil || be.PostedAt.UTC().Year() != 2026 {
		t.Errorf("PostedAt = %v, want parsed publishedDate (2026)", be.PostedAt)
	}

	sre, ok := byID["2002"]
	if !ok {
		t.Fatal("posting 2002 missing")
	}
	if !sre.Remote {
		t.Error("Remote = false, want true from isRemoteWork")
	}
	if sre.WorkMode != "remote" {
		t.Errorf("WorkMode = %q, want remote", sre.WorkMode)
	}
	if sre.Location != "Brasil" {
		t.Errorf("Location = %q, want country only when city/state are blank", sre.Location)
	}

	if _, dropped := byID["2003"]; dropped {
		t.Error("posting 2003 has no jobUrl and must be dropped")
	}
}

// itoa is provided by ozon_test.go in this package.

func TestGupyFetchAssemblesRichDescription(t *testing.T) {
	// The portal listing flattens a posting's sections into one tag-less blob; the richer
	// public detail endpoint returns them as separate HTML fields. Fetch must pull the
	// detail and assemble the sections into structured HTML, not serve the flat listing text.
	fake := (&routedHTTP{}).
		route("offset=0", `{"data":[
			{"id":500,"name":"Backend Engineer","jobUrl":"https://acme.gupy.io/job/500",
			 "description":"Do stuff;Ship it;Be a Go expert","workplaceType":"remote"}
		]}`).
		route("job-publication/public/jobs/500", `{
			"description":"<p>.</p>",
			"responsibilities":"<p>O profissional:</p><ul><li>Ship it;</li></ul>",
			"prerequisites":"<ul><li>Be a Go expert</li></ul>",
			"relevantExperiences":"<p>Free lunch</p>"
		}`)

	jobs, err := NewGupy(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "gupy", Board: "42",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1", len(jobs))
	}
	desc := jobs[0].Description

	// Structure from the detail is preserved: list markup survives sanitization.
	if !strings.Contains(desc, "<li>") || !strings.Contains(desc, "<ul>") {
		t.Errorf("Description dropped list structure, got %q", desc)
	}
	// Every section's body is present.
	for _, want := range []string{"Ship it", "Go expert", "Free lunch"} {
		if !strings.Contains(desc, want) {
			t.Errorf("Description missing %q section body, got %q", want, desc)
		}
	}
	// The empty "<p>.</p>" intro placeholder must not leak a stray leading dot.
	if strings.HasPrefix(strings.TrimSpace(desc), ".") {
		t.Errorf("Description leaked the placeholder intro, got %q", desc)
	}
}

func TestGupyFetchFallsBackToFlatDescription(t *testing.T) {
	// When the detail endpoint is unreachable (no route → GetJSON error), Fetch degrades to
	// the listing's own description rather than dropping the body.
	fake := (&routedHTTP{}).
		route("offset=0", `{"data":[
			{"id":600,"name":"Role","jobUrl":"https://acme.gupy.io/job/600",
			 "description":"<p>flat body</p>","workplaceType":"remote"}
		]}`)

	jobs, err := NewGupy(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "gupy", Board: "42",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1", len(jobs))
	}
	if !strings.Contains(jobs[0].Description, "flat body") {
		t.Errorf("Description = %q, want fallback to flat listing body", jobs[0].Description)
	}
}

func TestGupyFetchStopsOnEmptyPage(t *testing.T) {
	// A short first page ends the walk immediately — no offset=100 request is made.
	fake := (&routedHTTP{}).
		route("offset=0", `{"pagination":{"total":1,"limit":100,"offset":0},"data":[
			{"id":1,"name":"Only Role","jobUrl":"https://x.gupy.io/job/1","workplaceType":"remote"}
		]}`)

	jobs, err := NewGupy(fake).Fetch(context.Background(), CompanyEntry{
		Company: "X", Provider: "gupy", Board: "999",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1 from the single short page", len(jobs))
	}
	// One listing call (the short page stops the walk — no offset=100 request) plus one
	// per-posting detail call for the single posting.
	if fake.calls != 2 {
		t.Errorf("made %d HTTP calls, want 2 (one listing that stops the walk + one detail)", fake.calls)
	}
}

// itoa is provided by ozon_test.go in this package.
