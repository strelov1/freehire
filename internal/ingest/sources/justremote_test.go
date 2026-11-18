package sources

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
)

// justremoteListing mirrors the live /remote-jobs page: the state document is assigned to
// window.__PRELOADED_STATE__ and followed by a semicolon and more markup, and carries unrelated
// state beside jobsState. Postings: an ATS repost restricted to one country, a contract posting
// with 2 countries, one with no restriction, an inactive one and one with no company.
const justremoteListing = `<html><body><script>
      window.__PRELOADED_STATE__ = {"articleState":{"uid":"x"},"jobsState":{"entity":{"all":[
{"id":29247,"title":"Strategic Account Executive 4 ","company_name":"Twilio","job_type":"permanent","date":"17 Nov","category":"manager","href":"remote-manager-exec-jobs/strategic-account-executive-4-twilio","remote_type":"Fully Remote","job_country":null,"is_active":true,"location_restrictions":["United States"]},
{"id":29100,"title":"Backend Engineer","company_name":"Acme","job_type":"contract","date":"10 Nov","category":"developer","href":"remote-developer-jobs/backend-engineer-acme","remote_type":"Fully Remote","is_active":true,"location_restrictions":["Germany","Poland"]},
{"id":29001,"title":"Support Agent","company_name":"Globex","job_type":"permanent","date":"01 Nov","category":"customerservice","href":"remote-customer-service-jobs/support-agent-globex","remote_type":"Fully Remote","is_active":true,"location_restrictions":[]},
{"id":28900,"title":"Closed Role","company_name":"Initech","job_type":"permanent","href":"remote-jobs/closed-initech","is_active":false,"location_restrictions":[]},
{"id":28800,"title":"No Company","company_name":" ","job_type":"permanent","href":"remote-jobs/no-company","is_active":true,"location_restrictions":[]}
]},"meta":{"status":"success"}}};
    </script><script src="/static/x.js"></script></body></html>`

func justremoteDetailPage(body, posted string) string {
	return `<html><head><script type="application/ld+json">{"@context":"http://schema.org","@type":"JobPosting","title":"x","datePosted":"` +
		posted + `","description":"` + body + `"}</script></head></html>`
}

func justremoteFake() *routedHTTP {
	return (&routedHTTP{}).
		route("justremote.co/remote-jobs", justremoteListing).
		route("strategic-account-executive-4-twilio", justremoteDetailPage("\\u003cp\\u003eSell things\\u003c/p\\u003e", "2026-11-17T01:02:03.000Z")).
		route("backend-engineer-acme", justremoteDetailPage("\\u003cp\\u003eBuild Go\\u003c/p\\u003e", "2026-11-10T00:00:00.000Z")).
		route("support-agent-globex", justremoteDetailPage("\\u003cp\\u003eHelp users\\u003c/p\\u003e", "2026-11-01T00:00:00.000Z"))
}

func TestJustRemoteProviderAndMarkers(t *testing.T) {
	s := NewJustRemote(nil)
	if s.Provider() != "justremote" {
		t.Errorf("Provider() = %q", s.Provider())
	}
	if _, ok := s.(boardless); !ok {
		t.Error("justremote should be boardless")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("justremote should be an aggregator")
	}
	if _, ok := s.(CoverageGated); !ok {
		t.Error("justremote should be CoverageGated")
	}
	if _, ok := All(nil)["justremote"]; !ok {
		t.Error("All() should register justremote")
	}
	if !slices.Contains(FilterableProviders(), "justremote") {
		t.Error("FilterableProviders() should include justremote")
	}
}

func TestJustRemoteFetchListOnlyMapsListing(t *testing.T) {
	jobs, err := NewJustRemote(justremoteFake()).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("got %d jobs, want 3 (inactive and company-less dropped)", len(jobs))
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	tw := byID["29247"]
	if tw.Title != "Strategic Account Executive 4" || tw.Company != "Twilio" ||
		tw.URL != "https://justremote.co/remote-manager-exec-jobs/strategic-account-executive-4-twilio" {
		t.Errorf("twilio mapped wrong: %+v", tw)
	}
	if tw.Location != "United States" || !tw.Remote || tw.WorkMode != "remote" || tw.Description != "" {
		t.Errorf("twilio location/remote/body wrong: %+v", tw)
	}
	if got := byID["29100"]; got.Location != "Germany, Poland" || got.EmploymentType != "contract" {
		t.Errorf("acme: location=%q employment=%q", got.Location, got.EmploymentType)
	}
	if got := byID["29001"]; got.Location != "Worldwide" || got.EmploymentType != "" {
		t.Errorf("globex: location=%q employment=%q", got.Location, got.EmploymentType)
	}
}

func TestJustRemoteFetchNewHydratesOnlyUnseen(t *testing.T) {
	fake := justremoteFake()
	seen := func(id string) bool { return id == "29100" }
	jobs, err := NewJustRemote(fake).(HydratingSource).FetchNew(context.Background(), CompanyEntry{}, seen)
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	if !byID["29100"].SeenRefresh || byID["29100"].Description != "" {
		t.Errorf("seen posting should be a bare refresh: %+v", byID["29100"])
	}
	tw := byID["29247"]
	if !strings.Contains(tw.Description, "Sell things") || strings.Contains(tw.Description, "\\u003c") {
		t.Errorf("twilio description = %q", tw.Description)
	}
	if tw.PostedAt == nil || tw.PostedAt.Format("2006-01-02") != "2026-11-17" {
		t.Errorf("twilio PostedAt = %v", tw.PostedAt)
	}
	// Listing + two unseen pages; the seen posting costs no request.
	if fake.calls != 3 {
		t.Errorf("requests = %d, want 3", fake.calls)
	}
}

func TestJustRemoteFetchNewGatedSkipsCoveredEmployers(t *testing.T) {
	fake := justremoteFake()
	var asked []string
	gate := DetailGate{Covered: func(c []string) map[string]bool {
		asked = c
		return map[string]bool{"Twilio": true}
	}}
	jobs, err := NewJustRemote(fake).(CoverageGated).FetchNewGated(
		context.Background(), CompanyEntry{}, func(string) bool { return false }, gate)
	if err != nil {
		t.Fatalf("FetchNewGated: %v", err)
	}
	if len(asked) != 3 {
		t.Errorf("gate asked about %v, want one name per open posting", asked)
	}
	if len(jobs) != 3 {
		t.Fatalf("got %d jobs, want 3 (a covered posting is still yielded)", len(jobs))
	}
	for _, j := range jobs {
		if j.Company == "Twilio" && (j.Description != "" || j.SeenRefresh) {
			t.Errorf("covered posting should be body-less and not a refresh: %+v", j)
		}
	}
	if fake.calls != 3 { // listing + acme + globex
		t.Errorf("requests = %d, want 3", fake.calls)
	}
}

func TestJustRemoteOneFailedBodyKeepsPostingListOnly(t *testing.T) {
	fake := justremoteFake().routeErr("backend-engineer-acme", errors.New("boom"))
	jobs, err := NewJustRemote(fake).(HydratingSource).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("got %d jobs, want 3", len(jobs))
	}
	for _, j := range jobs {
		if j.ExternalID == "29100" && j.Description != "" {
			t.Errorf("failed body should leave the posting list-only: %+v", j)
		}
	}
}

// The echojobs#2588 guard: postings due a body, none readable, is a failed crawl.
func TestJustRemoteFailsWhenNoBodyIsReadable(t *testing.T) {
	fake := (&routedHTTP{}).route("justremote.co/remote-jobs", justremoteListing)
	_, err := NewJustRemote(fake).(HydratingSource).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err == nil || !strings.Contains(err.Error(), "read none") {
		t.Fatalf("err = %v, want a read-none failure", err)
	}
}

func TestJustRemoteListingWithoutStateFails(t *testing.T) {
	fake := (&routedHTTP{}).route("justremote.co/remote-jobs", "<html>Just a moment...</html>")
	if _, err := NewJustRemote(fake).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("a page without the state document must fail the crawl, not read as an empty inventory")
	}
}

func TestJustRemoteListingFetchErrorPropagates(t *testing.T) {
	fake := (&routedHTTP{}).routeErr("justremote.co/remote-jobs", errors.New("503"))
	if _, err := NewJustRemote(fake).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("expected the listing error")
	}
}
