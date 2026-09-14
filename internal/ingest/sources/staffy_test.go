package sources

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
)

// staffyListingHTML builds a /vacantes fixture with the given (slug, title) job cards and
// a declared total — mirrors the live page's shape (job-card articles + a
// section-kicker-title count), verified live against boldbusiness... err, wearestaffy.com.
func staffyListingHTML(declaredTotal int, jobs ...[2]string) string {
	var cards strings.Builder
	for _, j := range jobs {
		slug, title := j[0], j[1]
		cards.WriteString(`<article class="job-card"><div><p class="eyebrow">LATAM · Remoto</p><h2>` +
			title + `</h2><p>desc</p></div><a class="button button-secondary" href="/positions/` +
			slug + `">Ver vacante</a></article>`)
	}
	return `<html><body><header><nav><a href="/vacantes">Vacantes</a></nav></header><main>` +
		`<section class="page-hero compact"><h1>Todas las busquedas activas.</h1></section>` +
		`<section class="section"><div class="section-heading minimal">` +
		`<h2 class="section-kicker-title accent">` + strconv.Itoa(declaredTotal) + ` activas</h2></div>` +
		`<div class="job-list">` + cards.String() + `</div></section></main></body></html>`
}

// staffyDetailHTML mirrors a live detail page's structure: an <h1> title, a three-span
// .metadata block (location, work arrangement, seniority), and <h2>-delimited prose.
func staffyDetailHTML(title, location, workArrangement, seniority, description string) string {
	return `<html><body><main><section class="position-layout"><article class="position-content">` +
		`<a class="back-link" href="/">Back to jobs</a><p class="eyebrow">New opportunity</p>` +
		`<h1>` + title + `</h1>` +
		`<div class="metadata"><span>` + location + `</span><span>` + workArrangement + `</span><span>` + seniority + `</span></div>` +
		`<h2>About the role</h2><p>` + description + `</p>` +
		`</article></section></main></body></html>`
}

func TestStaffyProvider(t *testing.T) {
	if got := NewStaffy(nil).Provider(); got != "staffy" {
		t.Errorf("Provider() = %q, want %q", got, "staffy")
	}
}

func TestStaffyMarkers(t *testing.T) {
	s := NewStaffy(nil)
	if _, ok := s.(boardless); !ok {
		t.Error("staffy should implement the boardless marker")
	}
	if _, ok := s.(fullBoardListing); !ok {
		t.Error("staffy should implement the fullBoardListing marker")
	}
}

func TestStaffyRegisteredAsFullBoardListing(t *testing.T) {
	if !FullBoardListingProviders(All(nil))["staffy"] {
		t.Error("FullBoardListingProviders(All(nil)) should include staffy")
	}
}

func TestStaffyRegisteredInAll(t *testing.T) {
	s, ok := All(nil)["staffy"]
	if !ok {
		t.Fatal("All() missing provider staffy")
	}
	if s.Provider() != "staffy" {
		t.Errorf("All()[staffy].Provider() = %q", s.Provider())
	}
}

func TestStaffyFetchListsAndHydrates(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/positions/data-engineer-junior-1967", staffyDetailHTML("Data Engineer", "Argentina", "Remoto", "Junior", "Build pipelines.")).
		route("/positions/cloud-solutions-architect-2011", staffyDetailHTML("Cloud Solutions Architect", "AR (Bs As)", "Hibrido (2 veces por semana)", "Semi senior", "Design clouds.")).
		route(staffyListingURL, staffyListingHTML(2,
			[2]string{"data-engineer-junior-1967", "Data Engineer"},
			[2]string{"cloud-solutions-architect-2011", "Cloud Solutions Architect"}))

	jobs, err := NewStaffy(fake).Fetch(context.Background(), CompanyEntry{Company: "Staffy"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2", len(jobs))
	}

	j1 := jobs[0]
	if j1.ExternalID != "data-engineer-junior-1967" {
		t.Errorf("ExternalID = %q", j1.ExternalID)
	}
	if j1.Title != "Data Engineer" {
		t.Errorf("Title = %q", j1.Title)
	}
	if j1.Company != "Staffy" {
		t.Errorf("Company = %q, want the configured CompanyEntry.Company", j1.Company)
	}
	if j1.Location != "Argentina" {
		t.Errorf("Location = %q", j1.Location)
	}
	if j1.Seniority != "junior" {
		t.Errorf("Seniority = %q, want junior", j1.Seniority)
	}
	if j1.WorkMode != "remote" {
		t.Errorf("WorkMode = %q, want remote (Remoto)", j1.WorkMode)
	}
	if !j1.Remote {
		t.Errorf("Remote = false, want true")
	}
	if j1.Description != "<h2>About the role</h2><p>Build pipelines.</p>" {
		t.Errorf("Description = %q", j1.Description)
	}
	if j1.URL != "https://jobs.wearestaffy.com/positions/data-engineer-junior-1967" {
		t.Errorf("URL = %q", j1.URL)
	}

	j2 := jobs[1]
	if j2.Seniority != "middle" {
		t.Errorf("Seniority = %q, want middle (Semi senior)", j2.Seniority)
	}
	if j2.WorkMode != "hybrid" {
		t.Errorf("WorkMode = %q, want hybrid (Hibrido)", j2.WorkMode)
	}
	if j2.Remote {
		t.Errorf("Remote = true, want false (hybrid, not remote)")
	}
}

func TestStaffyEmptyBoardYieldsNoJobsNoError(t *testing.T) {
	fake := (&routedHTTP{}).route(staffyListingURL, staffyListingHTML(0))
	jobs, err := NewStaffy(fake).Fetch(context.Background(), CompanyEntry{Company: "Staffy"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs, want 0", len(jobs))
	}
}

func TestStaffyFetchFailsWholeBoardOnListingError(t *testing.T) {
	fake := (&routedHTTP{}).routeErr(staffyListingURL, errors.New("boom"))
	if _, err := NewStaffy(fake).Fetch(context.Background(), CompanyEntry{Company: "Staffy"}); err == nil {
		t.Fatal("Fetch succeeded despite a listing error")
	}
}

func TestStaffyFetchFailsWhenDeclaredTotalDisagreesWithLinkCount(t *testing.T) {
	fake := (&routedHTTP{}).route(staffyListingURL, staffyListingHTML(5,
		[2]string{"data-engineer-junior-1967", "Data Engineer"}))
	if _, err := NewStaffy(fake).Fetch(context.Background(), CompanyEntry{Company: "Staffy"}); err == nil {
		t.Fatal("Fetch succeeded despite the declared total disagreeing with the link count")
	}
}

func TestStaffyUnreadableDetailIsMarkedNotDropped(t *testing.T) {
	fake := (&routedHTTP{}).
		routeErr("/positions/data-engineer-junior-1967", errors.New("timeout")).
		route(staffyListingURL, staffyListingHTML(1,
			[2]string{"data-engineer-junior-1967", "Data Engineer"}))

	jobs, err := NewStaffy(fake).Fetch(context.Background(), CompanyEntry{Company: "Staffy"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (an unreadable marker, not a drop)", len(jobs))
	}
	if jobs[0].ExternalID != "data-engineer-junior-1967" {
		t.Errorf("ExternalID = %q, want data-engineer-junior-1967", jobs[0].ExternalID)
	}
}

func TestStaffySeniorityMapping(t *testing.T) {
	cases := []struct{ label, want string }{
		{"Junior", "junior"},
		{"Semi senior", "middle"},
		{"Senior", "senior"},
		{"Sr", "senior"},
		{"Lead", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := staffySeniority(c.label); got != c.want {
			t.Errorf("staffySeniority(%q) = %q, want %q", c.label, got, c.want)
		}
	}
}

func TestStaffyWorkModeMapping(t *testing.T) {
	cases := []struct{ text, want string }{
		{"Remoto", "remote"},
		{"Hibrido - 2 veces por semana", "hybrid"},
		{"Híbrido (Puerto Madero)", "hybrid"},
		{"Presencial", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := staffyWorkMode(c.text); got != c.want {
			t.Errorf("staffyWorkMode(%q) = %q, want %q", c.text, got, c.want)
		}
	}
}
