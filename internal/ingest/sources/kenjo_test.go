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

// kenjoFixture reads an answer captured verbatim from ict-ds.kenjo.io (2026-10-25): the site's
// listing (four open positions) and two of their details — one with an office address, one
// without.
func kenjoFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/kenjo/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}

const kenjoICTAPI = "ict-ds.kenjo.io/api/controller/career-site/public/ict-ds/positions"

func kenjoICT(t *testing.T) *routedHTTP {
	return (&routedHTTP{}).
		route(kenjoICTAPI+"/devops-engineer-m-w-d-44544", kenjoFixture(t, "detail_devops-engineer-m-w-d-44544.json")).
		route(kenjoICTAPI+"/fullstack-entwickler-m-w-d-361894", kenjoFixture(t, "detail_fullstack-entwickler-m-w-d-361894.json")).
		route(kenjoICTAPI, kenjoFixture(t, "list_ict-ds.json"))
}

func TestKenjoProvider(t *testing.T) {
	s, ok := All(nil)["kenjo"]
	if !ok || s.Provider() != "kenjo" {
		t.Fatal("All() must register kenjo")
	}
	if _, ok := s.(HydratingSource); !ok {
		t.Error("kenjo must be a HydratingSource: the body costs a detail request")
	}
	if !FullBoardListingProviders(All(nil))["kenjo"] {
		t.Error("kenjo's listing is one unpaginated answer: it must be fullBoardListing")
	}
}

func TestKenjoListingRefreshesSeenPositions(t *testing.T) {
	fake := kenjoICT(t)
	jobs, err := NewKenjo(fake).(HydratingSource).FetchNew(context.Background(),
		CompanyEntry{Company: "ICT Digital Solutions", Provider: "kenjo", Board: "ict-ds"},
		func(string) bool { return true })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 4 || fake.calls != 1 {
		t.Fatalf("jobs/requests = %d/%d, want the listing's 4 positions from one request", len(jobs), fake.calls)
	}
	for _, j := range jobs {
		if !j.SeenRefresh || j.ExternalID == "" || !strings.HasPrefix(j.URL, "https://ict-ds.kenjo.io/") {
			t.Errorf("job = %+v, want an identity-only SeenRefresh", j)
		}
	}
}

func TestKenjoMapsADetail(t *testing.T) {
	jobs, err := NewKenjo(kenjoICT(t)).(HydratingSource).FetchNew(context.Background(),
		CompanyEntry{Company: "ICT Digital Solutions", Provider: "kenjo", Board: "ict-ds"},
		func(id string) bool { return id != "657ace136fa6b97681458bea" && id != "67fd15a53f6455d7b72f04b1" })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	d := byID["657ace136fa6b97681458bea"]
	if d.SeenRefresh || d.Description == "" {
		t.Fatalf("devops = %+v, want a hydrated position", d)
	}
	if d.URL != "https://ict-ds.kenjo.io/devops-engineer-m-w-d-44544" || d.Title != "DevOps Engineer (m/w/d)" {
		t.Errorf("URL/Title = %q/%q", d.URL, d.Title)
	}
	if d.Company != "ICT Digital Solutions GmbH" || d.Location != "München" || !slices.Equal(d.Countries, []string{"de"}) {
		t.Errorf("Company/Location/Countries = %q/%q/%v", d.Company, d.Location, d.Countries)
	}
	// Kenjo records no work arrangement; the body's "oder hybrid" is the parser's to read.
	if d.EmploymentType != "full_time" || d.Remote || d.WorkMode != "" {
		t.Errorf("EmploymentType/Remote/WorkMode = %q/%v/%q", d.EmploymentType, d.Remote, d.WorkMode)
	}
	if d.PostedAt == nil || d.PostedAt.Format("2006-01-02") != "2023-12-14" {
		t.Errorf("PostedAt = %v", d.PostedAt)
	}
	// No office at all: no location is invented and no country either.
	f := byID["67fd15a53f6455d7b72f04b1"]
	if f.Location != "" || len(f.Countries) != 0 || f.Description == "" {
		t.Errorf("fullstack Location/Countries = %q/%v", f.Location, f.Countries)
	}
}

// A position published without a body is stored body-less: the detail answered, and a marker
// would recur every run. An answer that is not the position at all is still a marker.
func TestKenjoBodylessPositionIsStoredAndReshapedDetailIsMarked(t *testing.T) {
	fake := (&routedHTTP{}).
		route(kenjoICTAPI+"/devops", `{"_id":"657ace136fa6b97681458bea","jobTitle":"DevOps Engineer (m/w/d)","positionType":"Full-time"}`).
		route(kenjoICTAPI+"/fullstack", `{"position":{}}`).
		route(kenjoICTAPI, kenjoFixture(t, "list_ict-ds.json"))
	jobs, err := NewKenjo(fake).(HydratingSource).FetchNew(context.Background(),
		CompanyEntry{Provider: "kenjo", Board: "ict-ds"},
		func(id string) bool { return id != "657ace136fa6b97681458bea" && id != "67fd15a53f6455d7b72f04b1" })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	if b := byID["657ace136fa6b97681458bea"]; b.Unreadable || b.Title != "DevOps Engineer (m/w/d)" || b.Description != "" {
		t.Errorf("body-less position = %+v, want it stored without a body", b)
	}
	if !byID["67fd15a53f6455d7b72f04b1"].Unreadable {
		t.Error("a detail that is not the position must be an unreadable marker")
	}
}

func TestKenjoUnknownSiteIsGone(t *testing.T) {
	fake := (&routedHTTP{}).routeErr("ampolon.kenjo.io", &StatusError{Method: http.MethodGet, Code: http.StatusNotFound})
	_, err := NewKenjo(fake).Fetch(context.Background(), CompanyEntry{Provider: "kenjo", Board: "ampolon"})
	if !errors.Is(err, ErrBoardGone) {
		t.Fatalf("err = %v, want ErrBoardGone", err)
	}
}

func TestKenjoReshapedListingFails(t *testing.T) {
	fake := (&routedHTTP{}).route("x.kenjo.io", `{"active":true,"companyName":"X"}`)
	if _, err := NewKenjo(fake).Fetch(context.Background(), CompanyEntry{Provider: "kenjo", Board: "x"}); err == nil {
		t.Fatal("a listing without activePositions must fail, not read as an empty site")
	}
}

func TestKenjoUnreadableDetailIsAMarker(t *testing.T) {
	fake := (&routedHTTP{}).
		routeErr(kenjoICTAPI+"/devops", &StatusError{Method: http.MethodGet, Code: http.StatusBadGateway}).
		routeErr(kenjoICTAPI+"/fullstack", &StatusError{Method: http.MethodGet, Code: http.StatusNotFound}).
		route(kenjoICTAPI, kenjoFixture(t, "list_ict-ds.json"))
	jobs, err := NewKenjo(fake).(HydratingSource).FetchNew(context.Background(),
		CompanyEntry{Provider: "kenjo", Board: "ict-ds"},
		func(id string) bool { return id != "657ace136fa6b97681458bea" && id != "67fd15a53f6455d7b72f04b1" })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	var marker, dropped = false, true
	for _, j := range jobs {
		if j.ExternalID == "657ace136fa6b97681458bea" {
			marker = j.Unreadable
		}
		if j.ExternalID == "67fd15a53f6455d7b72f04b1" {
			dropped = false
		}
	}
	if !marker || !dropped {
		t.Errorf("marker/dropped = %v/%v, want a 502 marked unreadable and a 404 dropped", marker, dropped)
	}
}

func TestKenjoEmploymentType(t *testing.T) {
	for in, want := range map[string]string{
		"Full-time": "full_time", "Part-time": "part_time", "Freelance": "contract", "Intern": "internship",
		"Flexible": "", "Apprentice": "", "Student": "", "": "",
	} {
		if got := kenjoEmploymentType(in); got != want {
			t.Errorf("kenjoEmploymentType(%q) = %q, want %q", in, got, want)
		}
	}
}
