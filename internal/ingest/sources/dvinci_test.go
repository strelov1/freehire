package sources

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"
)

func dvinciFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/dvinci/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestDvinciProvider(t *testing.T) {
	if got := NewDvinci(nil).Provider(); got != "dvinci" {
		t.Errorf("Provider() = %q, want dvinci", got)
	}
}

// TestDvinciFetchMapsBoardDocument reads the real CADENAS board captured 2026-10-25 (recruiter
// contact and images scrubbed): 11 publications over 6 openings, all in German.
func TestDvinciFetchMapsBoardDocument(t *testing.T) {
	fake := (&routedHTTP{}).route("cadenas.dvinci-hr.com/jobPublication/list.json", dvinciFixture(t, "cadenas.json"))
	jobs, err := NewDvinci(fake).Fetch(context.Background(), CompanyEntry{Provider: "dvinci", Board: "cadenas.dvinci-hr.com", Company: "CADENAS"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	// One opening published under several titles in ONE language is several postings.
	if len(jobs) != 11 {
		t.Fatalf("got %d jobs, want all 11 publications", len(jobs))
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	if _, ok := byID["20287"]; !ok {
		t.Error("DevOps Engineer (20287) was dropped; it shares an opening with 20267 but is its own posting")
	}

	j := byID["20291"]
	if j.URL != "https://cadenas.dvinci-hr.com/de/jobs/20291/junior-genai-engineer-mwd-cadenas-augsburg" {
		t.Errorf("URL = %q", j.URL)
	}
	if j.Title != "(Junior) GenAI Engineer (m/w/d)" || j.Company != "CADENAS GmbH" {
		t.Errorf("Title/Company = %q/%q", j.Title, j.Company)
	}
	if j.Location != "Augsburg (onsite), Germany" || len(j.Countries) != 1 || j.Countries[0] != "de" {
		t.Errorf("Location/Countries = %q/%v", j.Location, j.Countries)
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q, want full_time", j.EmploymentType)
	}
	if deref(j.SalaryMin) != 65000 || deref(j.SalaryMax) != 65000 || j.SalaryCurrency != "EUR" || j.SalaryPeriod != "year" {
		t.Errorf("salary = %v-%v %s/%s", deref(j.SalaryMin), deref(j.SalaryMax), j.SalaryCurrency, j.SalaryPeriod)
	}
	// The publication's start date, not the opening's creation.
	if j.PostedAt == nil || j.PostedAt.Format("2006-01-02T15:04") != "2026-10-01T22:00" {
		t.Errorf("PostedAt = %v", j.PostedAt)
	}
	// Rendered sections in page order with their headlines; the header and contact are not body.
	for _, want := range []string{"Your Tasks", "Your Profile", "Come as you are!"} {
		if !strings.Contains(j.Description, want) {
			t.Errorf("Description lacks %q", want)
		}
	}
	if strings.Contains(j.Description, "Recruiting team") {
		t.Error("Description carries the contact section")
	}
	if strings.Index(j.Description, "Your Tasks") > strings.Index(j.Description, "Your Profile") {
		t.Error("sections out of page order")
	}

	// Several places: each named with its country, joined.
	if got := byID["20257"].Location; got != "Augsburg (onsite), Germany; Essen (onsite), Germany; Wolfsburg, Germany" {
		t.Errorf("multi-place Location = %q", got)
	}
	// Two working times stated: neither is the posting's.
	if got := byID["20257"].EmploymentType; got != "" {
		t.Errorf("FULL_TIME+PART_TIME EmploymentType = %q, want empty", got)
	}
}

// One opening published once per language is one job: the lowest publication id is kept.
func TestDvinciCollapsesTranslations(t *testing.T) {
	fake := (&routedHTTP{}).route("weig.dvinci-easy.com/jobPublication/list.json", dvinciFixture(t, "weig.json"))
	jobs, err := NewDvinci(fake).Fetch(context.Background(), CompanyEntry{Provider: "dvinci", Board: "weig.dvinci-easy.com", Company: "WEIG"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	var ids []string
	for _, j := range jobs {
		ids = append(ids, j.ExternalID)
	}
	if strings.Join(ids, ",") != "40927,40926" {
		t.Errorf("ids = %v, want the German publications 40927,40926 (the lower id of each pair)", ids)
	}
}

// A tenant on the plain template (aekno, captured 2026-10-25) has no renderedContent: the raw
// fields are the body.
func TestDvinciPlainTemplate(t *testing.T) {
	fake := (&routedHTTP{}).route("aekno-karriere.dvinci-easy.com/jobPublication/list.json", dvinciFixture(t, "aekno.json"))
	jobs, err := NewDvinci(fake).Fetch(context.Background(), CompanyEntry{Provider: "dvinci", Board: "aekno-karriere.dvinci-easy.com", Company: "Ärztekammer Nordrhein"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.Description == "" {
		t.Error("Description empty; the raw tasks/profile fields should fill it")
	}
	// No company on the opening: the catalogue's name stands.
	if j.Company != "Ärztekammer Nordrhein" {
		t.Errorf("Company = %q", j.Company)
	}
	// A place with no country: named, but no structured country is invented.
	if j.Location != "Düsseldorf (Hauptstelle)" || j.Countries != nil {
		t.Errorf("Location/Countries = %q/%v", j.Location, j.Countries)
	}
}

func TestDvinciEmptyBoard(t *testing.T) {
	fake := (&routedHTTP{}).route("x.dvinci-hr.com/jobPublication/list.json", "[]")
	jobs, err := NewDvinci(fake).Fetch(context.Background(), CompanyEntry{Board: "x.dvinci-hr.com"})
	if err != nil || len(jobs) != 0 {
		t.Fatalf("jobs/err = %v/%v, want an empty board", jobs, err)
	}
}

func TestDvinciNullDocumentFails(t *testing.T) {
	fake := (&routedHTTP{}).route("x.dvinci-hr.com/jobPublication/list.json", "null")
	if _, err := NewDvinci(fake).Fetch(context.Background(), CompanyEntry{Board: "x.dvinci-hr.com"}); err == nil {
		t.Fatal("a null document read as an empty board")
	}
}

func TestDvinciNotFoundIsBoardGone(t *testing.T) {
	fake := (&routedHTTP{}).routeErr("x.dvinci-hr.com", &StatusError{Code: http.StatusNotFound})
	_, err := NewDvinci(fake).Fetch(context.Background(), CompanyEntry{Board: "x.dvinci-hr.com"})
	if !errors.Is(err, ErrBoardGone) {
		t.Fatalf("err = %v, want ErrBoardGone", err)
	}
}

func TestDvinciRejectsNonHostBoard(t *testing.T) {
	for _, b := range []string{"cadenas", "cadenas.dvinci-hr.com/de", "https://cadenas.dvinci-hr.com", ""} {
		if _, err := NewDvinci(&routedHTTP{}).Fetch(context.Background(), CompanyEntry{Board: b}); err == nil {
			t.Errorf("board %q accepted", b)
		}
	}
}
