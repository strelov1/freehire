package sources

import (
	"context"
	"strconv"
	"strings"
	"testing"
)

func TestGeekHunterProvider(t *testing.T) {
	if got := NewGeekHunter(nil).Provider(); got != "geekhunter" {
		t.Errorf("Provider() = %q, want %q", got, "geekhunter")
	}
}

// GeekHunter earns fullBoardListing because the /jobs page inlines every open posting in one
// ItemList ld+json block, so a listing failure aborts the whole Fetch rather than returning a
// partial board.
func TestGeekHunterMarkers(t *testing.T) {
	s := NewGeekHunter(nil)
	if _, ok := s.(fullBoardListing); !ok {
		t.Error("geekhunter should implement the fullBoardListing marker")
	}
}

func TestGeekHunterRegisteredAsFullBoardListing(t *testing.T) {
	if !FullBoardListingProviders(All(nil))["geekhunter"] {
		t.Error("FullBoardListingProviders(All(nil)) should include geekhunter")
	}
}

func TestGeekHunterRegisteredInAll(t *testing.T) {
	s, ok := All(nil)["geekhunter"]
	if !ok {
		t.Fatal("All() missing provider geekhunter")
	}
	if s.Provider() != "geekhunter" {
		t.Errorf("All()[geekhunter].Provider() = %q", s.Provider())
	}
}

// A listing fetch failure must abort the whole Fetch, never return a partial result as
// success — the property TestGeekHunterMarkers' fullBoardListing claim rests on.
func TestGeekHunterFetchPropagatesAListingError(t *testing.T) {
	fake := &routedHTTP{}
	if _, err := NewGeekHunter(fake).Fetch(context.Background(), CompanyEntry{Board: "acme"}); err == nil {
		t.Fatal("Fetch succeeded despite a listing error")
	}
}

// itemListHTML is a board listing page carrying the schema.org ItemList ld+json block
// GeekHunter server-renders, naming the board's open postings.
func itemListHTML(name string, urls ...string) string {
	var items []string
	for i, u := range urls {
		items = append(items, `{"@type":"ListItem","position":`+strconv.Itoa(i+1)+`,"url":"`+u+`","name":"job"}`)
	}
	return `<html><head><script id="itemList" type="application/ld+json">` +
		`{"@context":"https://schema.org","@type":"ItemList","name":"` + name +
		`","numberOfItems":` + strconv.Itoa(len(urls)) + `,"itemListElement":[` + strings.Join(items, ",") + `]}` +
		`</script></head><body>page</body></html>`
}

// geekhunterJobPostingHTML is a posting page carrying the schema.org JobPosting ld+json block
// GeekHunter server-renders.
func geekhunterJobPostingHTML(title, desc, company, datePosted, locality string, employmentType []string) string {
	loc := ""
	if locality != "" {
		loc = `,"jobLocation":[{"@type":"Place","address":{"@type":"PostalAddress","addressLocality":"` +
			locality + `","addressCountry":"BRA"}}]`
	}
	et := ""
	if len(employmentType) > 0 {
		et = `,"employmentType":["` + strings.Join(employmentType, `","`) + `"]`
	}
	return `<html><head><script type="application/ld+json">` +
		`{"@context":"https://schema.org","@type":"JobPosting","title":"` + title +
		`","description":"` + desc + `","datePosted":"` + datePosted +
		`","hiringOrganization":{"@type":"Organization","name":"` + company + `"}` +
		et + loc + `}` +
		`</script></head><body>page</body></html>`
}

func TestGeekHunterFetchListsAndFetchesDetail(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/pt/acme/jobs/backend-engineer", geekhunterJobPostingHTML(
			"Backend Engineer", "<p>Build things.</p>", "Acme Inc",
			"2026-04-02", "Florianópolis", []string{"FULL_TIME"})).
		route("/pt/acme/jobs/remote-designer", geekhunterJobPostingHTML(
			"Designer Remoto", "<p>Design things.</p>", "Acme Inc",
			"2026-05-01", "", nil)).
		route("/pt/acme/jobs", itemListHTML("Vagas tech em Acme",
			"https://www.geekhunter.com/pt/acme/jobs/backend-engineer",
			"https://www.geekhunter.com/pt/acme/jobs/remote-designer"))

	jobs, err := NewGeekHunter(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "geekhunter", Board: "acme",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2", len(jobs))
	}

	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	j, ok := byID["backend-engineer"]
	if !ok {
		t.Fatal("job backend-engineer missing")
	}
	if j.Title != "Backend Engineer" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "Acme Inc" {
		t.Errorf("Company = %q, want the hiringOrganization name from the ld+json", j.Company)
	}
	if j.Location != "Florianópolis, BRA" {
		t.Errorf("Location = %q, want the jobLocation address joined", j.Location)
	}
	if j.Remote {
		t.Error("Remote = true, want false: a jobLocation is present")
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q, want full_time", j.EmploymentType)
	}
	if !strings.Contains(j.Description, "Build things.") {
		t.Errorf("Description = %q, want the JSON-LD description", j.Description)
	}
	if j.PostedAt == nil || j.PostedAt.UTC().Year() != 2026 {
		t.Errorf("PostedAt = %v, want parsed datePosted (2026)", j.PostedAt)
	}

	r, ok := byID["remote-designer"]
	if !ok {
		t.Fatal("job remote-designer missing")
	}
	if !r.Remote {
		t.Error("Remote = false, want true: no jobLocation at all is GeekHunter's remote signal")
	}
	if r.WorkMode != "remote" {
		t.Errorf("WorkMode = %q, want remote", r.WorkMode)
	}
	if r.Location != "" {
		t.Errorf("Location = %q, want empty (no jobLocation)", r.Location)
	}
}

// A page that answered but carries no JobPosting yields no description, so the posting is
// still not stored — but it is MARKED rather than dropped, using the URL-derived slug as its
// id since the ld+json identifier never became available.
func TestGeekHunterPostingWithoutJobPostingIsMarkedNotDropped(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/pt/acme/jobs/ok", geekhunterJobPostingHTML(
			"OK Job", "<p>Real work.</p>", "Acme Inc", "2026-01-14", "Recife", nil)).
		route("/pt/acme/jobs/broken", `<html><head></head><body>no job posting here</body></html>`).
		route("/pt/acme/jobs", itemListHTML("Vagas tech em Acme",
			"https://www.geekhunter.com/pt/acme/jobs/ok",
			"https://www.geekhunter.com/pt/acme/jobs/broken"))

	jobs, err := NewGeekHunter(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "geekhunter", Board: "acme",
	})
	if err != nil {
		t.Fatalf("Fetch should not abort the board on one unreadable detail: %v", err)
	}
	read := readPostings(jobs)
	if len(read) != 1 || read[0].ExternalID != "ok" {
		t.Fatalf("read = %v, want only ok", read)
	}
	markers := unreadableMarkers(jobs)
	if len(markers) != 1 || markers[0].ExternalID != "broken" {
		t.Fatalf("unreadable markers = %v, want one for broken", markers)
	}
}

func TestGeekHunterListingURLsHandlesAnEmptyBoard(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/pt/empty/jobs", itemListHTML("Vagas tech em Empty"))

	jobs, err := NewGeekHunter(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Empty", Provider: "geekhunter", Board: "empty",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("len(jobs) = %d, want 0 for a board with no open postings", len(jobs))
	}
}
