package sources

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"
)

// flatchrNotFoundPage is what a switched-off careers site serves at its .json address, under
// HTTP 200 (captured 2026-10-24 from careers.flatchr.io/company/accessite.json, trimmed).
const flatchrNotFoundPage = `<section class="notFound">
  <div class="illustration row">
    <div class="illustration_content col-12 col-sm-7">
      <div class="illustration_content_title">
        Votre recherche est tombée à l’eau.
      </div>
    </div>
  </div>
</section>`

func TestFlatchrProvider(t *testing.T) {
	if got := NewFlatchr(nil).Provider(); got != "flatchr" {
		t.Errorf("Provider() = %q, want flatchr", got)
	}
}

// TestFlatchrFetchMapsBoardDocument reads the real Mila board captured 2026-10-24 (company
// prose trimmed): six vacancies spanning all three telework values.
func TestFlatchrFetchMapsBoardDocument(t *testing.T) {
	doc, err := os.ReadFile("testdata/flatchr/mila.json")
	if err != nil {
		t.Fatal(err)
	}
	fake := (&routedHTTP{}).route("careers.flatchr.io/company/mila.json", string(doc))
	jobs, err := NewFlatchr(fake).Fetch(context.Background(), CompanyEntry{Provider: "flatchr", Board: "mila", Company: "Mila"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 6 {
		t.Fatalf("got %d jobs, want 6", len(jobs))
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	remote := byID["D3NVZdqRPMYnMyb6"]
	if remote.URL != "https://careers.flatchr.io/fr/company/mila/vacancy/d3nvzdqrpmynmyb6-100-teletravail-gestionnaire-senior-sinistres-mri-mrh-f-h/" {
		t.Errorf("URL = %q", remote.URL)
	}
	if remote.WorkMode != "remote" || !remote.Remote {
		t.Errorf("WorkMode/Remote = %q/%v, want remote/true", remote.WorkMode, remote.Remote)
	}
	if remote.Company != "Mila" || remote.Location != "1 Parv. de la Défense, 92800 Puteaux, France" {
		t.Errorf("Company/Location = %q/%q", remote.Company, remote.Location)
	}
	if remote.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q, want full_time for a CDI not flagged part-time", remote.EmploymentType)
	}
	// The body is the three sections the careers site renders, in order.
	if !strings.Contains(remote.Description, "100% télétravail") || !strings.Contains(remote.Description, "Mila") {
		t.Errorf("Description = %q", remote.Description)
	}
	if remote.PostedAt == nil || remote.PostedAt.Format("2006-01-02") != "2026-10-01" {
		t.Errorf("PostedAt = %v", remote.PostedAt)
	}

	if got := byID["xlV0D9a66LZpWELk"].WorkMode; got != "hybrid" {
		t.Errorf("parttime telework = %q, want hybrid", got)
	}
	if got := byID["ADYjo9mkyD7dkR0r"].WorkMode; got != "onsite" {
		t.Errorf("no telework = %q, want onsite", got)
	}
}

// A holding's board lists its subsidiaries' vacancies: the employer is the vacancy's own.
func TestFlatchrGroupBoardNamesEachEmployer(t *testing.T) {
	fake := (&routedHTTP{}).route("company/additi.json", `{"items":[{"status":"published","published":true,
		"created_at":"2026-10-01T13:16:24.668Z","vacancy":{"id":"oLm5NdDxKEOdAGbZ",
		"slug":"olm5nddxkeodagbz-assistant-marketing-et-communication-h-f","title":"Assistant Marketing H/F ",
		"description":"<p>Medialex</p>","language":"fr_FR","remote":"notime","contract_type":"Stage",
		"company":{"name":"Medialex ","slug":"medialex"}}},
		{"status":"draft","published":false,"vacancy":{"id":"x","slug":"x-y","title":"Hidden"}}]}`)
	jobs, err := NewFlatchr(fake).Fetch(context.Background(), CompanyEntry{Board: "additi", Company: "Additi"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (an unpublished item is skipped)", len(jobs))
	}
	if jobs[0].Company != "Medialex" || jobs[0].EmploymentType != "internship" {
		t.Errorf("Company/EmploymentType = %q/%q, want Medialex/internship", jobs[0].Company, jobs[0].EmploymentType)
	}
	if !strings.HasPrefix(jobs[0].URL, "https://careers.flatchr.io/fr/company/additi/vacancy/") {
		t.Errorf("URL = %q, want the address on the board that listed it", jobs[0].URL)
	}
}

func TestFlatchrBoardGone(t *testing.T) {
	for name, fake := range map[string]*routedHTTP{
		// The decode of an HTML page fails; the text read then shows the platform's verdict.
		"switched-off site": (&routedHTTP{}).route("company/acme.json", flatchrNotFoundPage),
		"unknown slug":      (&routedHTTP{}).routeErr("company/acme.json", &StatusError{Code: http.StatusNotFound}),
	} {
		_, err := NewFlatchr(fake).Fetch(context.Background(), CompanyEntry{Board: "acme", Company: "Acme"})
		if !errors.Is(err, ErrBoardGone) {
			t.Errorf("%s: err = %v, want ErrBoardGone", name, err)
		}
	}
}

func TestFlatchrUnreadableDocumentIsNotGone(t *testing.T) {
	fake := (&routedHTTP{}).route("company/acme.json", `<html>some other page</html>`)
	_, err := NewFlatchr(fake).Fetch(context.Background(), CompanyEntry{Board: "acme", Company: "Acme"})
	if err == nil || errors.Is(err, ErrBoardGone) {
		t.Fatalf("err = %v, want a failure that does not claim the board is gone", err)
	}
}

func TestFlatchrDocumentWithoutItemsFails(t *testing.T) {
	fake := (&routedHTTP{}).route("company/acme.json", `{"statusCode":200}`)
	if _, err := NewFlatchr(fake).Fetch(context.Background(), CompanyEntry{Board: "acme", Company: "Acme"}); err == nil {
		t.Fatal("a document without items is a reshaped answer, not an empty board")
	}
}

func TestFlatchrEmploymentType(t *testing.T) {
	for _, c := range []struct {
		contract string
		partial  bool
		want     string
	}{
		{"CDI", false, "full_time"},
		{"CDI", true, "part_time"},
		{"CDD", false, "contract"},
		{"Freelance", false, "contract"},
		{"Stage", false, "internship"},
		{"Alternance", false, ""},
		{"Statutaire", true, "part_time"},
		{"VIE", false, ""},
	} {
		if got := flatchrEmploymentType(c.contract, c.partial); got != c.want {
			t.Errorf("flatchrEmploymentType(%q, %v) = %q, want %q", c.contract, c.partial, got, c.want)
		}
	}
}
