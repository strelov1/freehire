package sources

import (
	"context"
	"errors"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"
)

// easycruitFixture reads an export captured verbatim from jeugdzorg.easycruit.com (2026-10-25):
// the list (three open vacancies, one published under two departments) and each vacancy's
// detail; list_empty.xml is vre.easycruit.com's, a site with no openings.
func easycruitFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/easycruit/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}

const easycruitJeugdzorg = "jeugdzorg.easycruit.com/export/xml/vacancy/"

func easycruitJeugdzorgHTTP(t *testing.T) *routedHTTP {
	r := &routedHTTP{}
	for _, id := range []string{"3643611", "3643615", "3667721"} {
		r.route(easycruitJeugdzorg+id+".xml", easycruitFixture(t, "detail_jeugdzorg_"+id+".xml"))
	}
	return r.route(easycruitJeugdzorg+"list.xml", easycruitFixture(t, "list_jeugdzorg.xml"))
}

var easycruitJeugdzorgEntry = CompanyEntry{Company: "Jeugdbescherming Rotterdam Rijnmond", Provider: "easycruit", Board: "jeugdzorg"}

func TestEasyCruitProvider(t *testing.T) {
	s, ok := All(nil)["easycruit"]
	if !ok || s.Provider() != "easycruit" {
		t.Fatal("All() must register easycruit")
	}
	if _, ok := s.(HydratingSource); !ok {
		t.Error("easycruit must be a HydratingSource: the body costs a detail request")
	}
	if !FullBoardListingProviders(All(nil))["easycruit"] {
		t.Error("easycruit's list export is one unpaginated document: it must be fullBoardListing")
	}
}

func TestEasyCruitListRefreshesSeenVacancies(t *testing.T) {
	fake := easycruitJeugdzorgHTTP(t)
	jobs, err := NewEasyCruit(fake).(HydratingSource).FetchNew(context.Background(), easycruitJeugdzorgEntry,
		func(string) bool { return true })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 3 || fake.calls != 1 {
		t.Fatalf("jobs/requests = %d/%d, want the list's 3 vacancies from one request", len(jobs), fake.calls)
	}
	for _, j := range jobs {
		if !j.SeenRefresh || j.ExternalID == "" || !strings.HasPrefix(j.URL, "https://jeugdzorg.easycruit.com/vacancy/"+j.ExternalID+"/") {
			t.Errorf("job = %+v, want an identity-only SeenRefresh", j)
		}
	}
}

func TestEasyCruitMapsADetail(t *testing.T) {
	jobs, err := NewEasyCruit(easycruitJeugdzorgHTTP(t)).Fetch(context.Background(), easycruitJeugdzorgEntry)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	if len(byID) != 3 {
		t.Fatalf("jobs = %d, want 3", len(byID))
	}
	// Published under two departments: one vacancy, the lower department id's page.
	c := byID["3643611"]
	if c.URL != "https://jeugdzorg.easycruit.com/vacancy/3643611/49012" ||
		c.Title != "Medewerker Crisisinterventieteam Jeugdbescherming Rotterdam" {
		t.Errorf("URL/Title = %q/%q", c.URL, c.Title)
	}
	if c.Company != "Jeugdbescherming Rotterdam Rijnmond" || c.Location != "Rotterdam, Zuid-Holland" {
		t.Errorf("Company/Location = %q/%q", c.Company, c.Location)
	}
	if !strings.Contains(c.Description, "Crisisinterventieteam") || strings.Contains(c.Description, "&lt;") {
		t.Errorf("Description = %.200q, want the decoded body", c.Description)
	}
	if c.PostedAt == nil || c.PostedAt.Format("2006-01-02") != "2026-08-03" {
		t.Errorf("PostedAt = %v", c.PostedAt)
	}
	// A fixed-term full-time vacancy is a contract; a placement is an internship.
	if got := byID["3643615"].EmploymentType; got != "contract" {
		t.Errorf("fixed-term EmploymentType = %q, want contract", got)
	}
	s := byID["3667721"]
	if s.EmploymentType != "internship" || len(s.Countries) != 0 || s.Location != "Rotterdam" {
		t.Errorf("internship EmploymentType/Countries/Location = %q/%v/%q", s.EmploymentType, s.Countries, s.Location)
	}
}

// The region names its country in the site's language; it goes through the shared country
// dictionary, and EasyCruit's own numeric id is not read.
func TestEasyCruitCountries(t *testing.T) {
	var v easycruitVersion
	for _, name := range []string{"Norge", "Sverige", "Norge", "Atlantis"} {
		v.Countries = append(v.Countries, struct {
			Name     string   `xml:"name,attr"`
			Counties []string `xml:"County"`
		}{Name: name})
	}
	if got := easycruitCountries(v); !slices.Equal(got, []string{"no", "se"}) {
		t.Errorf("countries = %v, want [no se]", got)
	}
}

func TestEasyCruitEmploymentType(t *testing.T) {
	items := func(pairs ...string) []easycruitCategory {
		var out []easycruitCategory
		for i := 0; i < len(pairs); i += 2 {
			out = append(out, easycruitCategory{Type: pairs[i], ID: pairs[i+1]})
		}
		return out
	}
	tests := []struct {
		name string
		in   []easycruitCategory
		want string
	}{
		{"permanent full time", items("duration", "permanent", "extent", "fulltime"), "full_time"},
		{"permanent part time", items("duration", "permanent", "extent", "parttime"), "part_time"},
		{"both extents state neither", items("duration", "permanent", "extent", "fulltime", "extent", "parttime"), ""},
		{"temporary cover", items("duration", "temporary", "extent", "fulltime"), "contract"},
		{"trainee", items("duration", "trainee"), "internship"},
		{"casual work alone", items("extent", "casual-work"), ""},
		{"nothing", nil, ""},
	}
	for _, tt := range tests {
		if got := easycruitEmploymentType(tt.in); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestEasyCruitEmptySite(t *testing.T) {
	fake := (&routedHTTP{}).route("vre.easycruit.com/export/xml/vacancy/list.xml", easycruitFixture(t, "list_empty.xml"))
	jobs, err := NewEasyCruit(fake).Fetch(context.Background(), CompanyEntry{Provider: "easycruit", Board: "vre"})
	if err != nil || len(jobs) != 0 {
		t.Fatalf("jobs/err = %d/%v, want an empty board", len(jobs), err)
	}
}

func TestEasyCruitUnknownSiteIsGone(t *testing.T) {
	fake := (&routedHTTP{}).routeErr("nope.easycruit.com", &StatusError{Method: http.MethodGet, Code: http.StatusNotFound})
	_, err := NewEasyCruit(fake).Fetch(context.Background(), CompanyEntry{Provider: "easycruit", Board: "nope"})
	if !errors.Is(err, ErrBoardGone) {
		t.Fatalf("err = %v, want ErrBoardGone", err)
	}
}

func TestEasyCruitReshapedListFails(t *testing.T) {
	fake := (&routedHTTP{}).route("x.easycruit.com", `<html><body>maintenance</body></html>`)
	if _, err := NewEasyCruit(fake).Fetch(context.Background(), CompanyEntry{Provider: "easycruit", Board: "x"}); err == nil {
		t.Fatal("an answer that is not the list export must fail, not read as an empty site")
	}
}

func TestEasyCruitUnreadableDetailIsAMarker(t *testing.T) {
	fake := (&routedHTTP{}).
		routeErr(easycruitJeugdzorg+"3643611.xml", &StatusError{Method: http.MethodGet, Code: http.StatusBadGateway}).
		routeErr(easycruitJeugdzorg+"3643615.xml", &StatusError{Method: http.MethodGet, Code: http.StatusNotFound}).
		route(easycruitJeugdzorg+"3667721.xml", `<Vacancy id="1"><Versions><Version><Title>x</Title></Version></Versions></Vacancy>`).
		route(easycruitJeugdzorg+"list.xml", easycruitFixture(t, "list_jeugdzorg.xml"))
	jobs, err := NewEasyCruit(fake).Fetch(context.Background(), easycruitJeugdzorgEntry)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	if !byID["3643611"].Unreadable {
		t.Error("a 502 on the detail must be an unreadable marker")
	}
	if _, ok := byID["3643615"]; ok {
		t.Error("a 404 on the detail is the platform's own answer and must drop the vacancy")
	}
	if !byID["3667721"].Unreadable {
		t.Error("a detail that is another vacancy must be an unreadable marker")
	}
}
