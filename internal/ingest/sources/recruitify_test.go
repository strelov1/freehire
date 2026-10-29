package sources

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"
)

// recruitifyFixture reads an answer captured verbatim from api.recruitify.ai for the motife
// portal (2026-10-28): the listing's two pages and the empty third that ends it, and three
// details — one in Kraków flagged remote, one remote naming no place, one in two cities.
func recruitifyFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/recruitify/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}

// fakeRecruitify answers by URL for one Origin; a request naming another portal is answered
// 404, as the API answers a posting that is not the portal's.
type fakeRecruitify struct {
	origin string
	routes map[string]string
	fail   map[string]error
	calls  int
}

func (f *fakeRecruitify) GetJSONWithHeaders(_ context.Context, url string, headers map[string]string, v any) error {
	f.calls++
	if err, ok := f.fail[url]; ok {
		return err
	}
	body, ok := f.routes[url]
	if !ok || headers["Origin"] != f.origin {
		return &StatusError{Code: http.StatusNotFound, URL: url}
	}
	return json.Unmarshal([]byte(body), v)
}

func recruitifyMotife(t *testing.T) *fakeRecruitify {
	const api = "https://api.recruitify.ai/apply/project"
	routes := map[string]string{
		api + "?page=1": recruitifyFixture(t, "list_motife_p1.json"),
		api + "?page=2": recruitifyFixture(t, "list_motife_p2.json"),
		api + "?page=3": recruitifyFixture(t, "list_motife_p3.json"),
	}
	for _, g := range []string{"1c4f6a44-9d6a-4ad8-9628-939d53e7d728", "16368099-8093-4447-a304-f8f1bd62fa84", "fbf62ae0-35cc-4a35-b122-f414a0e00f73"} {
		routes[api+"/"+g] = recruitifyFixture(t, "detail_"+g+".json")
	}
	return &fakeRecruitify{origin: "https://motife.recruitify.ai", routes: routes, fail: map[string]error{}}
}

var recruitifyMotifeEntry = CompanyEntry{Company: "MOTIFE", Provider: "recruitify", Board: "motife"}

func TestRecruitifyProvider(t *testing.T) {
	s, ok := All(nil)["recruitify"]
	if !ok || s.Provider() != "recruitify" {
		t.Fatal("All() must register recruitify")
	}
	if _, ok := s.(HydratingSource); !ok {
		t.Error("recruitify must be a HydratingSource: the body costs a detail request")
	}
	if !FullBoardListingProviders(All(nil))["recruitify"] {
		t.Error("recruitify walks its listing to the empty page: it must be fullBoardListing")
	}
}

func TestRecruitifyListingRefreshesSeenPostings(t *testing.T) {
	fake := recruitifyMotife(t)
	jobs, err := NewRecruitify(fake).(HydratingSource).FetchNew(context.Background(), recruitifyMotifeEntry,
		func(string) bool { return true })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 30 || fake.calls != 3 {
		t.Fatalf("jobs/requests = %d/%d, want the listing's 30 postings from its three pages", len(jobs), fake.calls)
	}
	for _, j := range jobs {
		if !j.SeenRefresh || j.ExternalID == "" || !strings.HasPrefix(j.URL, "https://motife.recruitify.ai/job/") {
			t.Errorf("job = %+v, want an identity-only SeenRefresh", j)
		}
	}
}

func TestRecruitifyMapsDetails(t *testing.T) {
	hydrate := map[string]bool{
		"1c4f6a44-9d6a-4ad8-9628-939d53e7d728": true,
		"16368099-8093-4447-a304-f8f1bd62fa84": true,
		"fbf62ae0-35cc-4a35-b122-f414a0e00f73": true,
	}
	jobs, err := NewRecruitify(recruitifyMotife(t)).(HydratingSource).FetchNew(context.Background(), recruitifyMotifeEntry,
		func(id string) bool { return !hydrate[id] })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	krakow := byID["1c4f6a44-9d6a-4ad8-9628-939d53e7d728"]
	if krakow.SeenRefresh || krakow.Description == "" || krakow.Title != "Staff Software Engineer (Backend & AI)" {
		t.Fatalf("krakow = %+v, want a hydrated posting", krakow)
	}
	if krakow.URL != "https://motife.recruitify.ai/job/1c4f6a44-9d6a-4ad8-9628-939d53e7d728" || krakow.Company != "MOTIFE" {
		t.Errorf("krakow url/company = %q/%q", krakow.URL, krakow.Company)
	}
	if krakow.Location != "Kraków" || len(krakow.Countries) != 1 || krakow.Countries[0] != "pl" {
		t.Errorf("krakow location/countries = %q/%v, want Kraków in pl", krakow.Location, krakow.Countries)
	}
	if krakow.WorkMode != "" || krakow.Remote {
		t.Errorf("krakow work mode = %q/%v: a remote flag beside a city states no arrangement", krakow.WorkMode, krakow.Remote)
	}
	if krakow.PostedAt == nil || krakow.PostedAt.Format("2006-01-02") != "2026-10-09" {
		t.Errorf("krakow posted = %v, want 2026-10-09", krakow.PostedAt)
	}

	remote := byID["16368099-8093-4447-a304-f8f1bd62fa84"]
	if remote.WorkMode != "remote" || !remote.Remote || remote.Location != "" || len(remote.Countries) != 0 {
		t.Errorf("remote = mode %q remote %v location %q countries %v, want remote with no place", remote.WorkMode, remote.Remote, remote.Location, remote.Countries)
	}

	two := byID["fbf62ae0-35cc-4a35-b122-f414a0e00f73"]
	if two.Location != "Zielona Góra; Wrocław" {
		t.Errorf("two-city location = %q", two.Location)
	}
}

func TestRecruitifyUnknownPortalIsGone(t *testing.T) {
	fake := recruitifyMotife(t)
	_, err := NewRecruitify(fake).Fetch(context.Background(), CompanyEntry{Company: "X", Provider: "recruitify", Board: "nosuchportal"})
	if !errors.Is(err, ErrBoardGone) {
		t.Fatalf("err = %v, want ErrBoardGone for a portal the API does not know", err)
	}
}

func TestRecruitifyLaterPageFailureFailsTheCrawl(t *testing.T) {
	fake := recruitifyMotife(t)
	fake.fail["https://api.recruitify.ai/apply/project?page=2"] = &StatusError{Code: http.StatusBadGateway}
	_, err := NewRecruitify(fake).(HydratingSource).FetchNew(context.Background(), recruitifyMotifeEntry,
		func(string) bool { return true })
	if err == nil || errors.Is(err, ErrBoardGone) {
		t.Fatalf("err = %v, want a crawl failure: a truncated listing would read as closures", err)
	}
}

func TestRecruitifyListingThatIgnoresThePageFails(t *testing.T) {
	fake := recruitifyMotife(t)
	fake.routes["https://api.recruitify.ai/apply/project?page=2"] = fake.routes["https://api.recruitify.ai/apply/project?page=1"]
	_, err := NewRecruitify(fake).(HydratingSource).FetchNew(context.Background(), recruitifyMotifeEntry,
		func(string) bool { return true })
	if err == nil {
		t.Fatal("a page repeating earlier postings must fail the walk, not loop to the page cap")
	}
}

func TestRecruitifyDetailGoneDropsAndFailureMarks(t *testing.T) {
	fake := recruitifyMotife(t)
	const api = "https://api.recruitify.ai/apply/project/"
	delete(fake.routes, api+"16368099-8093-4447-a304-f8f1bd62fa84") // a 404: gone
	fake.fail[api+"fbf62ae0-35cc-4a35-b122-f414a0e00f73"] = &StatusError{Code: http.StatusInternalServerError}
	jobs, err := NewRecruitify(fake).(HydratingSource).FetchNew(context.Background(), recruitifyMotifeEntry,
		func(id string) bool {
			return id != "16368099-8093-4447-a304-f8f1bd62fa84" && id != "fbf62ae0-35cc-4a35-b122-f414a0e00f73"
		})
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	if _, ok := byID["16368099-8093-4447-a304-f8f1bd62fa84"]; ok {
		t.Error("a posting the API answers 404 for is gone and must be dropped")
	}
	if j := byID["fbf62ae0-35cc-4a35-b122-f414a0e00f73"]; !j.Unreadable {
		t.Errorf("a failed detail = %+v, want an unreadable marker", j)
	}
}
