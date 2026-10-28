package sources

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	"golang.org/x/time/rate"
)

// talentclueFixture reads a capture of Henneo's client (2026-10-27): four entries of its API
// listing — one marked archived by hand — and the page of its hybrid Hiberus posting with scripts
// and styles stripped.
func talentclueFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/talentclue/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}

const talentclueHenneoClient = "4c5d0b2e8a1f4e6b9c3d7a1e5f2b8c90"

var talentclueHenneoEntry = CompanyEntry{Company: "Henneo", Provider: "talentclue", Board: talentclueHenneoClient}

func talentclueHenneoHTTP(t *testing.T) *routedHTTP {
	return (&routedHTTP{}).
		route("/jswidget/jobs/"+talentclueHenneoClient+"/", talentclueFixture(t, "list_henneo.json")).
		route("/node/119990106/", talentclueFixture(t, "job_119990106.html"))
}

// newTestTalentclue is the adapter without its crawl-delay pacing.
func newTestTalentclue(c talentclueHTTP) talentclue {
	s := NewTalentclue(c).(talentclue)
	s.apiLimiter = rate.NewLimiter(rate.Inf, 1)
	s.pageDelay = 0
	return s
}

func TestTalentclueProvider(t *testing.T) {
	s, ok := All(nil)["talentclue"]
	if !ok || s.Provider() != "talentclue" {
		t.Fatal("All() must register talentclue")
	}
	if _, ok := s.(HydratingSource); !ok {
		t.Error("talentclue must be a HydratingSource: a body costs a page request")
	}
}

func TestTalentclueFilterIsTheWidgetDefault(t *testing.T) {
	b, err := base64.StdEncoding.DecodeString(talentclueFilter)
	if err != nil {
		t.Fatalf("filter is not base64: %v", err)
	}
	var f map[string]any
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatalf("filter is not JSON: %v", err)
	}
	if f["subs"] != "1" || f["includeOnlyClosedJobs"] != false || f["showArchivedJobs"] != false {
		t.Errorf("filter = %v, want open jobs of the client and its subgroups", f)
	}
}

func TestTalentclueListsAndRefreshesSeenPostings(t *testing.T) {
	fake := talentclueHenneoHTTP(t)
	jobs, err := newTestTalentclue(fake).FetchNew(context.Background(), talentclueHenneoEntry, func(string) bool { return true })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	// Four listed, one archived: three refreshes from the one API call.
	if len(jobs) != 3 || fake.calls != 1 {
		t.Fatalf("jobs/requests = %d/%d, want 3 postings from one request", len(jobs), fake.calls)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		if !j.SeenRefresh || j.Description != "" {
			t.Errorf("job %s = %+v, want a listing-only SeenRefresh", j.ExternalID, j)
		}
		byID[j.ExternalID] = j
	}
	if _, ok := byID["95046578"]; ok {
		t.Error("an archived posting must not be listed")
	}
	ar := byID["104637340"]
	if ar.WorkMode != "remote" || !ar.Remote || len(ar.Countries) != 1 || ar.Countries[0] != "ar" {
		t.Errorf("remote Argentine posting = %+v", ar)
	}
	if ar.Location != "Ciudad de Buenos Aires, Buenos Aires, Argentina" {
		t.Errorf("Location = %q", ar.Location)
	}
	// "Únete al equipo" carries no country code and no place.
	open := byID["43069965"]
	if open.WorkMode != "onsite" || open.Countries != nil || open.Location != "" || open.Company != "Henneo" {
		t.Errorf("placeless posting = %+v", open)
	}
}

func TestTalentclueHydratesANewPosting(t *testing.T) {
	fake := talentclueHenneoHTTP(t)
	jobs, err := newTestTalentclue(fake).FetchNew(context.Background(), talentclueHenneoEntry,
		func(id string) bool { return id != "119990106" })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	var j Job
	for _, x := range jobs {
		if x.ExternalID == "119990106" {
			j = x
		}
	}
	if j.SeenRefresh || j.Unreadable || j.Title != "¿Te unes al universo hiberus?" {
		t.Fatalf("job = %+v, want it hydrated", j)
	}
	// The subgroup is the employer: Henneo's client lists Hiberus's openings.
	if j.Company != "Hiberus" || j.URL != "https://henneo.talentclue.com/es/node/119990106/4590" {
		t.Errorf("Company/URL = %q/%q", j.Company, j.URL)
	}
	if j.WorkMode != "hybrid" || j.Remote || j.EmploymentType != "full_time" || j.Location != "Pamplona, Navarra, España" {
		t.Errorf("WorkMode/EmploymentType/Location = %q/%q/%q", j.WorkMode, j.EmploymentType, j.Location)
	}
	if len(j.Countries) != 1 || j.Countries[0] != "es" || j.PostedAt == nil || j.PostedAt.Format("2006-01-02") != "2025-10-07" {
		t.Errorf("Countries/PostedAt = %v/%v", j.Countries, j.PostedAt)
	}
	if !strings.Contains(j.Description, "Hiberus") || strings.Contains(j.Description, "<script") {
		t.Errorf("Description = %.200q", j.Description)
	}
}

func TestTalentclueBoundsPagesPerRun(t *testing.T) {
	fake := talentclueHenneoHTTP(t)
	s := newTestTalentclue(fake)
	s.maxNewPerRun = 1
	jobs, err := s.FetchNew(context.Background(), talentclueHenneoEntry, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 1 || fake.calls != 2 {
		t.Fatalf("jobs/requests = %d/%d, want one page bought and the rest deferred", len(jobs), fake.calls)
	}
}

func TestTalentclueDetailFailures(t *testing.T) {
	gone := (&routedHTTP{}).
		route("/jswidget/jobs/", talentclueFixture(t, "list_henneo.json")).
		routeErr("/node/", &StatusError{Method: "GET", Code: http.StatusNotFound})
	jobs, err := newTestTalentclue(gone).FetchNew(context.Background(), talentclueHenneoEntry,
		func(id string) bool { return id != "119990106" })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 2 {
		t.Errorf("a 404 page must drop its posting: got %d jobs, want 2", len(jobs))
	}

	refused := (&routedHTTP{}).
		route("/jswidget/jobs/", talentclueFixture(t, "list_henneo.json")).
		routeErr("/node/", &StatusError{Method: "GET", Code: http.StatusForbidden})
	jobs, err = newTestTalentclue(refused).FetchNew(context.Background(), talentclueHenneoEntry,
		func(id string) bool { return id != "119990106" })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	marked := false
	for _, j := range jobs {
		if j.ExternalID == "119990106" {
			marked = j.Unreadable && j.Company == "Hiberus"
		}
	}
	if !marked {
		t.Error("a refused page must yield an unreadable marker under the posting's employer")
	}
}

func TestTalentclueListingShapes(t *testing.T) {
	empty := (&routedHTTP{}).route("/jswidget/jobs/", `{"jobs":[]}`)
	jobs, err := newTestTalentclue(empty).Fetch(context.Background(), talentclueHenneoEntry)
	if err != nil || len(jobs) != 0 {
		t.Errorf("an empty client = %d jobs, %v; want none and no error", len(jobs), err)
	}

	for name, body := range map[string]string{
		"no job list":   `{"error":{"errorcode":400,"description":"Invalid clientId"}}`,
		"foreign link":  `{"jobs":{"1":{"url":"https://evil.example/es/node/1/4590","title":"x","is_archived":"0"}}}`,
		"non-node key":  `{"jobs":{"abc":{"url":"https://x.talentclue.com/es/node/1/4590","title":"x","is_archived":"0"}}}`,
		"not a listing": `{"jobs":"nope"}`,
	} {
		r := (&routedHTTP{}).route("/jswidget/jobs/", body)
		if _, err := newTestTalentclue(r).Fetch(context.Background(), talentclueHenneoEntry); err == nil {
			t.Errorf("%s: want the crawl to fail", name)
		}
	}

	bad := CompanyEntry{Company: "Henneo", Provider: "talentclue", Board: "henneo"}
	if _, err := newTestTalentclue(&routedHTTP{}).Fetch(context.Background(), bad); err == nil {
		t.Error("a board that is not a client id must fail the crawl")
	}
}

func TestTalentclueEmploymentType(t *testing.T) {
	for _, c := range []struct{ contract, shift, want string }{
		{"Beca", "Jornada completa", "internship"},
		{"Formativo de Prácticas", "", "internship"},
		{"Contratista/Freelance", "", "contract"},
		{"Indefinido", "Jornada parcial", "part_time"},
		{"Temporal", "", ""},
	} {
		if got := talentclueEmploymentType(c.contract, c.shift); got != c.want {
			t.Errorf("talentclueEmploymentType(%q, %q) = %q, want %q", c.contract, c.shift, got, c.want)
		}
	}
}
