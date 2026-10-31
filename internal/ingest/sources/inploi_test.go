package sources

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"golang.org/x/net/html"
)

func TestInploiProvider(t *testing.T) {
	if got := NewInploi(nil).Provider(); got != "inploi" {
		t.Errorf("Provider() = %q, want inploi", got)
	}
}

func TestInploiRegisteredAsFullBoardListing(t *testing.T) {
	if !FullBoardListingProviders(All(nil))["inploi"] {
		t.Error("FullBoardListingProviders(All(nil)) should include inploi")
	}
	if EmptyBoardListingProviders(All(nil))["inploi"] {
		t.Error("an unknown segment answers an empty list, so inploi must not claim emptyBoardListing")
	}
}

func TestInploiFetchListsPostings(t *testing.T) {
	fake := &inploiFake{
		pages: map[string]string{
			"https://www.inploi.com/company/compass/search": `<html><body><script>pk_live_example_key_1 "segment_ids","Segment","248"</script></body></html>`,
		},
		final: map[string]string{
			"https://www.inploi.com/company/compass/search": "https://jobs.compass-group.co.uk/search",
		},
		json: map[string]string{
			"1": `{
				"data": [
					{
						"id": 75853365,
						"title": "Guest Service Ambassador",
						"company_name": "Rapport",
						"employment_type": "FULL_TIME",
						"location_type": "LOCATION",
						"city": "London",
						"country": "United Kingdom",
						"created_at": "2026-10-30T18:09:42.000000Z",
						"pay": "34000.00",
						"pay_type": "ANNUALLY",
						"pay_currency": "GBP",
						"pay_display": true,
						"pay_mask": false
					},
					{
						"id": 75852084,
						"title": "Night Support",
						"employment_type": "PART_TIME",
						"location_type": "REMOTE",
						"country": "GB",
						"pay": "0.00",
						"pay_type": "HOURLY",
						"pay_currency": "GBP",
						"pay_mask": true,
						"pay_display": true
					}
				],
				"pagination": {"total": 2, "current_page": 1, "last_page": 1}
			}`,
		},
		html: map[string]string{
			"https://jobs.compass-group.co.uk/job/75853365": `<html><head><script type="application/ld+json">{"@type":"JobPosting","description":"<p>Pour the coffee.</p>"}</script></head></html>`,
			"https://jobs.compass-group.co.uk/job/75852084": `<html><head><script type="application/ld+json">{"@type":"JobPosting","description":"Cover the ward.\n\nCall the nurse."}</script></head></html>`,
		},
	}
	jobs, err := NewInploi(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Compass Group", Provider: "inploi", Board: "Compass",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("len = %d, want 2", len(jobs))
	}
	guest := jobs[0]
	if guest.ExternalID != "75853365" || guest.Title != "Guest Service Ambassador" || guest.Company != "Rapport" {
		t.Errorf("guest = %+v", guest)
	}
	if guest.URL != "https://jobs.compass-group.co.uk/job/75853365" {
		t.Errorf("URL = %q", guest.URL)
	}
	if guest.EmploymentType != "full_time" || guest.WorkMode != "" || len(guest.Countries) != 1 || guest.Countries[0] != "gb" {
		t.Errorf("facets = %s %q %v", guest.EmploymentType, guest.WorkMode, guest.Countries)
	}
	if guest.SalaryMin == nil || *guest.SalaryMin != 34000 || guest.SalaryMax == nil || *guest.SalaryMax != 34000 || guest.SalaryCurrency != "GBP" || guest.SalaryPeriod != "year" {
		t.Errorf("salary = %v %v %s %s", guest.SalaryMin, guest.SalaryMax, guest.SalaryCurrency, guest.SalaryPeriod)
	}
	if guest.PostedAt == nil || !strings.Contains(guest.Description, "<p>Pour the coffee.</p>") {
		t.Errorf("posted=%v description=%q", guest.PostedAt, guest.Description)
	}
	night := jobs[1]
	if night.Company != "Compass Group" || night.EmploymentType != "part_time" || night.WorkMode != "remote" || night.Countries[0] != "gb" {
		t.Errorf("night = %+v", night)
	}
	if night.SalaryMin != nil || night.SalaryPeriod != "" {
		t.Errorf("masked pay was stored: %v %s", night.SalaryMin, night.SalaryPeriod)
	}
	if !strings.Contains(night.Description, "<p>Cover the ward.</p>") || !strings.Contains(night.Description, "<p>Call the nurse.</p>") {
		t.Errorf("Description = %q, want the plain text rebuilt as paragraphs", night.Description)
	}
	if fake.header != "pk_live_example_key_1" {
		t.Errorf("publishable key header = %q", fake.header)
	}
}

func TestInploiFetchNewSkipsASeenBody(t *testing.T) {
	fake := inploiOne(`{"data":[{"id":7,"title":"Chef","company_name":"Haven"}],"pagination":{"total":1,"current_page":1,"last_page":1}}`)
	fake.html["https://jobs.example.test/job/7"] = `<html><head><script type="application/ld+json">{"@type":"JobPosting","description":"<p>Cook.</p>"}</script></head></html>`
	src := NewInploi(fake).(inploi)
	jobs, err := src.FetchNew(context.Background(), CompanyEntry{Board: "haven"}, func(id string) bool { return id == "7" })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 1 || !jobs[0].SeenRefresh || fake.htmlN.Load() != 0 {
		t.Fatalf("jobs=%+v htmlGets=%d, want a refresh and no detail request", jobs, fake.htmlN.Load())
	}
}

func TestInploiFetchNewCapsUnseenDetail(t *testing.T) {
	fake := &inploiFake{
		pages: map[string]string{
			"https://www.inploi.com/company/haven/search": `<html><script>pk_live_example_key_1 "segment_ids","segment","497"</script></html>`,
		},
		final: map[string]string{
			"https://www.inploi.com/company/haven/search": "https://jobs.haven.com/search",
		},
		json: map[string]string{
			"1": `{"data":[{"id":1,"title":"One"}],"pagination":{"total":2,"current_page":1,"last_page":2}}`,
			"2": `{"data":[{"id":2,"title":"Two"}],"pagination":{"total":2,"current_page":2,"last_page":2}}`,
		},
		html: map[string]string{
			"https://jobs.haven.com/job/1": `<html><script type="application/ld+json">{"@type":"JobPosting","description":"<p>One.</p>"}</script></html>`,
			"https://jobs.haven.com/job/2": `<html><script type="application/ld+json">{"@type":"JobPosting","description":"<p>Two.</p>"}</script></html>`,
		},
	}
	src := NewInploi(fake).(inploi)
	src.pageSize = 1
	src.maxNew = 1
	jobs, err := src.FetchNew(context.Background(), CompanyEntry{Board: "haven"}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 1 || fake.htmlN.Load() != 1 {
		t.Fatalf("jobs=%+v htmlGets=%d, want only one new posting hydrated", jobs, fake.htmlN.Load())
	}
}

func TestInploiFetchNewReachesPastUnreadablePostings(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fake := inploiOne(`{"data":[{"id":1,"title":"One"},{"id":2,"title":"Two"},{"id":3,"title":"Engineer"},{"id":4,"title":"Seen"}],"pagination":{"total":4,"current_page":1,"last_page":1}}`)
		fake.html["https://jobs.example.test/job/3"] = `<html><script type="application/ld+json">{"@type":"JobPosting","description":"<p>Build software.</p>"}</script></html>`
		src := NewInploi(fake).(inploi)
		src.maxNew = 2
		found := false
		for range 2 {
			asked := map[string]bool{}
			jobs, err := src.FetchNew(context.Background(), CompanyEntry{Board: "haven"}, func(id string) bool {
				asked[id] = true
				return id == "4"
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(asked) != 4 {
				t.Fatalf("consulted %d listed IDs, want all 4 including deferred postings", len(asked))
			}
			refreshed := false
			for _, job := range jobs {
				if job.ExternalID == "3" && strings.Contains(job.Description, "Build software.") {
					found = true
				}
				if job.ExternalID == "4" && job.SeenRefresh {
					refreshed = true
				}
			}
			if !refreshed {
				t.Fatal("seen posting lost its liveness refresh")
			}
			time.Sleep(time.Hour)
		}
		if !found {
			t.Fatal("readable posting behind the unreadable prefix was never hydrated")
		}
		if got := fake.htmlN.Load(); got != 4 {
			t.Fatalf("detail requests = %d, want two per crawl", got)
		}
	})
}

func TestInploiFetchReadsTheHostSearchWhenTheRedirectDropsIt(t *testing.T) {
	fake := &inploiFake{
		pages: map[string]string{
			"https://www.inploi.com/company/puregym/search": `<html>home</html>`,
		},
		final: map[string]string{
			"https://www.inploi.com/company/puregym/search": "https://careers.puregym.com/",
		},
		html: map[string]string{
			"https://careers.puregym.com/search": `<html><script>pk_live_example_key_1 "segment_ids","segment","9"</script></html>`,
			"https://careers.puregym.com/job/3":  `<html><script type="application/ld+json">{"@type":"JobPosting","description":"<p>Lift.</p>"}</script></html>`,
		},
		json: map[string]string{
			"1": `{"data":[{"id":3,"title":"Coach","company_name":"PureGym"}],"pagination":{"total":1,"current_page":1,"last_page":1}}`,
		},
	}
	jobs, err := NewInploi(fake).Fetch(context.Background(), CompanyEntry{Board: "puregym"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].URL != "https://careers.puregym.com/job/3" || !strings.Contains(jobs[0].Description, "<p>Lift.</p>") {
		t.Fatalf("jobs = %+v", jobs)
	}
}

func TestInploiFetchEmptySegment(t *testing.T) {
	fake := inploiOne(`{"data":[],"pagination":{"total":0,"current_page":1,"last_page":1}}`)
	jobs, err := NewInploi(fake).Fetch(context.Background(), CompanyEntry{Board: "haven"})
	if err != nil || len(jobs) != 0 {
		t.Fatalf("jobs=%v err=%v, want an empty read", jobs, err)
	}
}

func TestInploiFetchRejectsAMissingCollection(t *testing.T) {
	fake := inploiOne(`{"pagination":{"total":0,"current_page":1,"last_page":1}}`)
	if _, err := NewInploi(fake).Fetch(context.Background(), CompanyEntry{Board: "compass"}); err == nil {
		t.Fatal("Fetch succeeded without a data collection")
	}
}

func TestInploiFetchRejectsAShortPage(t *testing.T) {
	fake := inploiOne(`{"data":[{"id":1,"title":"One"}],"pagination":{"total":2,"current_page":1,"last_page":1}}`)
	if _, err := NewInploi(fake).Fetch(context.Background(), CompanyEntry{Board: "compass"}); err == nil {
		t.Fatal("Fetch succeeded with fewer rows than the source listed")
	}
}

func TestInploiFetchRejectsADuplicateID(t *testing.T) {
	fake := inploiOne(`{"data":[{"id":1,"title":"One"},{"id":1,"title":"One"}],"pagination":{"total":2,"current_page":1,"last_page":1}}`)
	if _, err := NewInploi(fake).Fetch(context.Background(), CompanyEntry{Board: "compass"}); err == nil {
		t.Fatal("Fetch succeeded with fewer distinct postings than the source listed")
	}
}

func TestInploiFetchGoneBoard(t *testing.T) {
	fake := &inploiFake{status: map[string]int{
		"https://www.inploi.com/company/missing/search": http.StatusNotFound,
	}}
	_, err := NewInploi(fake).Fetch(context.Background(), CompanyEntry{Board: "missing"})
	if !errors.Is(err, ErrBoardGone) {
		t.Fatalf("err = %v, want ErrBoardGone", err)
	}
}

func TestInploiFetchRejectsABadLabel(t *testing.T) {
	if _, err := NewInploi(&inploiFake{}).Fetch(context.Background(), CompanyEntry{Board: "../x"}); err == nil {
		t.Fatal("Fetch accepted a board label that is not a slug")
	}
}

func TestInploiFetchRejectsAPageWithoutASegment(t *testing.T) {
	fake := &inploiFake{
		pages: map[string]string{
			"https://www.inploi.com/company/puregym/search": `<html><script>pk_live_example_key_1</script></html>`,
		},
		final: map[string]string{
			"https://www.inploi.com/company/puregym/search": "https://careers.puregym.com/search",
		},
	}
	if _, err := NewInploi(fake).Fetch(context.Background(), CompanyEntry{Board: "puregym"}); err == nil {
		t.Fatal("Fetch succeeded without a segment id")
	}
}

func inploiOne(body string) *inploiFake {
	return &inploiFake{
		pages: map[string]string{
			"https://www.inploi.com/company/compass/search": `<html><script>pk_live_example_key_1 "segment_ids","segment","248"</script></html>`,
			"https://www.inploi.com/company/haven/search":   `<html><script>pk_live_example_key_1 "segment_ids","segment","497"</script></html>`,
		},
		final: map[string]string{
			"https://www.inploi.com/company/compass/search": "https://jobs.example.test/search",
			"https://www.inploi.com/company/haven/search":   "https://jobs.example.test/search",
		},
		json: map[string]string{"1": body},
		html: map[string]string{},
	}
}

type inploiFake struct {
	pages  map[string]string
	final  map[string]string
	json   map[string]string
	html   map[string]string
	status map[string]int
	header string
	htmlN  atomic.Int64
}

func (f *inploiFake) GetHTMLResolved(_ context.Context, raw string) (*html.Node, string, error) {
	if code, ok := f.status[raw]; ok {
		return nil, "", &StatusError{Code: code, URL: raw}
	}
	body, ok := f.pages[raw]
	if !ok {
		return nil, "", errors.New("missing " + raw)
	}
	n, err := html.Parse(strings.NewReader(body))
	final := f.final[raw]
	if final == "" {
		final = raw
	}
	return n, final, err
}

func (f *inploiFake) GetHTML(_ context.Context, raw string) (*html.Node, error) {
	f.htmlN.Add(1)
	body, ok := f.html[raw]
	if !ok {
		return nil, errors.New("missing " + raw)
	}
	return html.Parse(strings.NewReader(body))
}

func (f *inploiFake) GetJSONWithHeaders(_ context.Context, raw string, headers map[string]string, v any) error {
	f.header = headers["x-publishable-key"]
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	body, ok := f.json[u.Query().Get("page")]
	if !ok {
		return errors.New("missing page " + u.Query().Get("page"))
	}
	return json.Unmarshal([]byte(body), v)
}
