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

// covetoFixture reads a page captured verbatim from k60360.coveto.de (2026-10-25): the RSS feed
// (32 open postings), two posting pages, and the notices a switched-off tenant (k29972) and a
// tenant without the feed (k10519) serve at the feed's address.
func covetoFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/coveto/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}

func covetoBoard60360(t *testing.T) *routedHTTP {
	return (&routedHTTP{}).route("k60360.coveto.de/public/rss/", covetoFixture(t, "rss_k60360.xml"))
}

func TestCovetoProvider(t *testing.T) {
	s, ok := All(nil)["coveto"]
	if !ok || s.Provider() != "coveto" {
		t.Fatal("All() must register coveto")
	}
	if _, ok := s.(HydratingSource); !ok {
		t.Error("coveto must be a HydratingSource: the body costs a page request")
	}
	if !FullBoardListingProviders(All(nil))["coveto"] {
		t.Error("coveto's feed is one unpaginated document: it must be fullBoardListing")
	}
}

func TestCovetoFeedListsEveryPosting(t *testing.T) {
	fake := covetoBoard60360(t)
	jobs, err := NewCoveto(fake).(HydratingSource).FetchNew(context.Background(),
		CompanyEntry{Company: "Stadtmission Nürnberg", Provider: "coveto", Board: "k60360"},
		func(string) bool { return true })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 32 {
		t.Fatalf("len(jobs) = %d, want the feed's 32 items", len(jobs))
	}
	if fake.calls != 1 {
		t.Errorf("requests = %d, want the feed alone", fake.calls)
	}
	for _, j := range jobs {
		if !j.SeenRefresh || j.ExternalID == "" || j.Title == "" || !strings.HasPrefix(j.URL, "https://k60360.coveto.de/job-") {
			t.Errorf("job = %+v, want an identity-only SeenRefresh", j)
		}
	}
}

func TestCovetoMapsPostingPages(t *testing.T) {
	fake := covetoBoard60360(t).
		route("-1934.html", covetoFixture(t, "detail_k60360_1934.html")).
		route("-1937.html", covetoFixture(t, "detail_k60360_1937.html"))
	jobs, err := NewCoveto(fake).(HydratingSource).FetchNew(context.Background(),
		CompanyEntry{Company: "Stadtmission Nürnberg", Provider: "coveto", Board: "k60360"},
		func(id string) bool { return id != "1934" && id != "1937" })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	// A field-sales post the employer filed as TELECOMMUTE, for a subsidiary of the tenant.
	r := byID["1934"]
	if r.SeenRefresh || r.Description == "" {
		t.Fatalf("1934 = %+v, want a hydrated posting", r)
	}
	if r.Company != "PROSUM GmbH" {
		t.Errorf("Company = %q, want the posting's own employer", r.Company)
	}
	if !r.Remote || r.WorkMode != "remote" {
		t.Errorf("Remote/WorkMode = %v/%q, want TELECOMMUTE read as remote", r.Remote, r.WorkMode)
	}
	if r.Location != "Region Ostdeutschland (Berlin, Brandenburg, Sachsen), Berlin" || !slices.Equal(r.Countries, []string{"de"}) {
		t.Errorf("Location/Countries = %q/%v", r.Location, r.Countries)
	}
	if r.EmploymentType != "full_time" || r.SalaryCurrency != "EUR" || r.SalaryPeriod != "year" ||
		r.SalaryMin == nil || *r.SalaryMin != 48000 || r.SalaryMax == nil || *r.SalaryMax != 60000 {
		t.Errorf("employment/salary = %q %v-%v %s/%s", r.EmploymentType, r.SalaryMin, r.SalaryMax, r.SalaryCurrency, r.SalaryPeriod)
	}
	if r.PostedAt == nil || r.PostedAt.Format("2006-01-02") != "2026-10-14" {
		t.Errorf("PostedAt = %v, want the feed's pubDate", r.PostedAt)
	}

	// An office post states no work arrangement, which must not become onsite.
	o := byID["1937"]
	if o.Remote || o.WorkMode != "" {
		t.Errorf("1937 Remote/WorkMode = %v/%q, want unknown", o.Remote, o.WorkMode)
	}
	if o.Location != "Röthenbach an der Pegnitz, Bayern" || o.Company != "DNE Catering gGmbH - Karl-Heller-Stift" {
		t.Errorf("1937 Location/Company = %q/%q", o.Location, o.Company)
	}
}

func TestCovetoUnreadablePostingIsAMarker(t *testing.T) {
	fake := covetoBoard60360(t).
		routeErr("-1934.html", &StatusError{Method: http.MethodGet, Code: http.StatusServiceUnavailable}).
		routeErr("-1937.html", &StatusError{Method: http.MethodGet, Code: http.StatusNotFound})
	jobs, err := NewCoveto(fake).(HydratingSource).FetchNew(context.Background(),
		CompanyEntry{Company: "Stadtmission Nürnberg", Provider: "coveto", Board: "k60360"},
		func(id string) bool { return id != "1934" && id != "1937" })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	var marker, gone bool
	for _, j := range jobs {
		switch j.ExternalID {
		case "1934":
			marker = j.Unreadable
		case "1937":
			gone = true
		}
	}
	if !marker {
		t.Error("a 503 posting page must yield an unreadable marker, not a drop")
	}
	if gone {
		t.Error("a 404 posting page is the platform's own answer and must be dropped")
	}
}

// An open application's page carries no JobPosting by the platform's design; it is not a
// posting, and must not become a marker that recurs every run — while a posting page missing
// its JobPosting still must.
func TestCovetoOpenApplicationIsDroppedNotMarked(t *testing.T) {
	page := covetoFixture(t, "detail_k13466_4736_open_application.html")
	fake := (&routedHTTP{}).
		route("k13466.coveto.de/public/rss/", covetoFixture(t, "rss_k13466.xml")).
		route("-4736.html", page).
		route("-4735.html", page) // a real posting's address answering without a JobPosting
	jobs, err := NewCoveto(fake).(HydratingSource).FetchNew(context.Background(),
		CompanyEntry{Company: "Albert-Schweitzer-Kinderdörfer", Provider: "coveto", Board: "k13466"},
		func(id string) bool { return id != "4736" && id != "4735" })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	var open, posting *Job
	for i := range jobs {
		switch jobs[i].ExternalID {
		case "4736":
			open = &jobs[i]
		case "4735":
			posting = &jobs[i]
		}
	}
	if open != nil {
		t.Errorf("open application = %+v, want it dropped", *open)
	}
	if posting == nil || !posting.Unreadable {
		t.Errorf("posting without a JobPosting = %+v, want an unreadable marker", posting)
	}
}

func TestCovetoSwitchedOffTenantIsGone(t *testing.T) {
	fake := (&routedHTTP{}).route("k29972.coveto.de/public/rss/", covetoFixture(t, "rss_inactive.html"))
	_, err := NewCoveto(fake).Fetch(context.Background(), CompanyEntry{Provider: "coveto", Board: "k29972"})
	if !errors.Is(err, ErrBoardGone) {
		t.Fatalf("err = %v, want ErrBoardGone", err)
	}
}

// A tenant that never enabled the feed still has live postings; this adapter cannot list them,
// which is a failure to report, not a board to retire.
func TestCovetoFeedNotEnabledIsNotGone(t *testing.T) {
	fake := (&routedHTTP{}).route("k10519.coveto.de/public/rss/", covetoFixture(t, "rss_disabled.html"))
	_, err := NewCoveto(fake).Fetch(context.Background(), CompanyEntry{Provider: "coveto", Board: "k10519"})
	if err == nil || errors.Is(err, ErrBoardGone) {
		t.Fatalf("err = %v, want an ordinary failure", err)
	}
}

func TestCovetoRejectsANonCustomerBoard(t *testing.T) {
	for _, b := range []string{"", "www", "stadtmission", "k60360.coveto.de"} {
		if _, err := NewCoveto(&routedHTTP{}).Fetch(context.Background(), CompanyEntry{Provider: "coveto", Board: b}); err == nil {
			t.Errorf("board %q: want an error", b)
		}
	}
}

func TestCovetoEmploymentTypeMapsOnlyASingleValue(t *testing.T) {
	for in, want := range map[string]string{
		"FULL_TIME": "full_time", "PART_TIME": "part_time", "FULL_TIME,PART_TIME": "", "FULL_TIME,INTERN": "", "": "",
	} {
		if got := covetoEmploymentType(in); got != want {
			t.Errorf("covetoEmploymentType(%q) = %q, want %q", in, got, want)
		}
	}
}
