package sources

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
)

// hrofficeFixture reads an answer captured verbatim from middendelfland.inhroffice.com
// (2026-10-25): the listing (three open vacancies) and each vacancy's page.
func hrofficeFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/hroffice/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}

const hrofficeMDF = "middendelfland.inhroffice.com"

func hrofficeMDFHTTP(t *testing.T) *routedHTTP {
	r := &routedHTTP{}
	for _, id := range []string{"391727", "391710", "317503"} {
		r.route(hrofficeMDF+"/nl/job/"+id, hrofficeFixture(t, "job_middendelfland_"+id+".html"))
	}
	return r.route(hrofficeMDF+"/api/nl/vacancies", hrofficeFixture(t, "list_middendelfland.json"))
}

var hrofficeMDFEntry = CompanyEntry{Company: "Gemeente Midden-Delfland", Provider: "hroffice", Board: "middendelfland"}

func TestHROfficeProvider(t *testing.T) {
	s, ok := All(nil)["hroffice"]
	if !ok || s.Provider() != "hroffice" {
		t.Fatal("All() must register hroffice")
	}
	if _, ok := s.(HydratingSource); !ok {
		t.Error("hroffice must be a HydratingSource: the body costs a page request")
	}
	if !FullBoardListingProviders(All(nil))["hroffice"] {
		t.Error("hroffice's listing is one unpaginated answer: it must be fullBoardListing")
	}
}

func TestHROfficeListingRefreshesSeenVacancies(t *testing.T) {
	fake := hrofficeMDFHTTP(t)
	jobs, err := NewHROffice(fake).(HydratingSource).FetchNew(context.Background(), hrofficeMDFEntry,
		func(string) bool { return true })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 3 || fake.calls != 1 {
		t.Fatalf("jobs/requests = %d/%d, want the listing's 3 vacancies from one request", len(jobs), fake.calls)
	}
	for _, j := range jobs {
		if !j.SeenRefresh || j.URL != "https://middendelfland.inhroffice.com/nl/job/"+j.ExternalID {
			t.Errorf("job = %+v, want an identity-only SeenRefresh", j)
		}
	}
}

func TestHROfficeMapsAPage(t *testing.T) {
	jobs, err := NewHROffice(hrofficeMDFHTTP(t)).Fetch(context.Background(), hrofficeMDFEntry)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	g := byID["391727"]
	if g.Title != "Medewerker groen en bomen – werken in de buitenruimte" || g.Company != "Gemeente Midden-Delfland" {
		t.Errorf("Title/Company = %q/%q", g.Title, g.Company)
	}
	if g.Location != "Maasland, Nederland" {
		t.Errorf("Location = %q", g.Location)
	}
	if !strings.Contains(g.Description, "groenonderhoud") || strings.Contains(g.Description, "Solliciteren") {
		t.Errorf("Description = %.200q, want the main column only", g.Description)
	}
	if g.PostedAt == nil || g.PostedAt.Format("2006-01-02") != "2026-10-23" {
		t.Errorf("PostedAt = %v", g.PostedAt)
	}
}

func TestHROfficeFailures(t *testing.T) {
	fake := (&routedHTTP{}).
		routeErr(hrofficeMDF+"/nl/job/391727", &StatusError{Method: http.MethodGet, Code: http.StatusBadGateway}).
		routeErr(hrofficeMDF+"/nl/job/391710", &StatusError{Method: http.MethodGet, Code: http.StatusNotFound}).
		route(hrofficeMDF+"/nl/job/317503", `<html><body><p>maintenance</p></body></html>`).
		route(hrofficeMDF+"/api/nl/vacancies", hrofficeFixture(t, "list_middendelfland.json"))
	jobs, err := NewHROffice(fake).Fetch(context.Background(), hrofficeMDFEntry)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	if !byID["391727"].Unreadable || !byID["317503"].Unreadable {
		t.Error("a 502 and a page without its main column must be unreadable markers")
	}
	if _, ok := byID["391710"]; ok {
		t.Error("a 404 on the page must drop the vacancy")
	}

	reshaped := (&routedHTTP{}).route("x.inhroffice.com/api/nl/vacancies", `{"type":"MiniJobPortalNotFoundException"}`)
	if _, err := NewHROffice(reshaped).Fetch(context.Background(), CompanyEntry{Provider: "hroffice", Board: "x"}); err == nil {
		t.Error("an answer that is not the array must fail, not read as an empty site")
	}
	empty := (&routedHTTP{}).route("quiet.inhroffice.com/api/nl/vacancies", `[]`)
	if jobs, err := NewHROffice(empty).Fetch(context.Background(), CompanyEntry{Provider: "hroffice", Board: "quiet"}); err != nil || len(jobs) != 0 {
		t.Errorf("empty listing: jobs/err = %d/%v", len(jobs), err)
	}
}
