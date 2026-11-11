package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// tbListing builds a results-endpoint response: the JSON envelope around the HTML fragment, with
// one anchor per (href, id) pair. A pair with an empty id models a non-posting link.
func tbListing(totalPages int, jobs ...[2]string) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<section id="search-results" data-total-pages="%d"><ul>`, totalPages)
	for _, j := range jobs {
		id := ""
		if j[1] != "" {
			id = ` data-job-id="` + j[1] + `"`
		}
		// The live markup repeats the id on a save button; only the anchor names a posting.
		fmt.Fprintf(&b, `<li><a href="%s"%s><h2>A job</h2></a><button data-job-id="%s">Save</button></li>`, j[0], id, j[1])
	}
	b.WriteString(`</ul></section>`)
	out, _ := json.Marshal(map[string]any{"results": b.String(), "hasJobs": true})
	return string(out)
}

func tbDetail(title, description, datePosted string) string {
	return `<html><head><script type="application/ld+json">` +
		`{"@context":"http://schema.org","@type":"JobPosting","title":"` + title + `",` +
		`"description":"` + description + `","datePosted":"` + datePosted + `",` +
		`"jobLocation":[{"@type":"Place","address":{"addressLocality":"Hamburg",` +
		`"addressRegion":"New York","addressCountry":"United States","streetAddress":"5999 S Park Ave"}}]}` +
		`</script></head><body></body></html>`
}

func TestTalentbrewProvider(t *testing.T) {
	if got := NewTalentbrew(nil).Provider(); got != "talentbrew" {
		t.Errorf("Provider() = %q", got)
	}
	if _, ok := All(nil)["talentbrew"]; !ok {
		t.Error("All() missing provider talentbrew")
	}
}

func TestTalentbrewFetchListingThenDetail(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/job/hamburg/cashier/61/100438924608", tbDetail("Cashier", "<p>Ring &rsquo;em up</p><iframe src=x></iframe>", "2026-11-03")).
		route("careers.aldi.us/search-jobs/results", tbListing(1,
			[2]string{"/job/hamburg/cashier/61/100438924608", "100438924608"},
			[2]string{"/job/hamburg/cashier/61/100438924608", "100438924608"}, // repeated link, one posting
			[2]string{"/search-jobs?page=2", ""},                              // not a posting
			[2]string{"https://other.example.com/job/x/1", "999"}))            // another host's posting

	jobs, err := NewTalentbrew(fake).Fetch(context.Background(), CompanyEntry{Company: "ALDI", Provider: "talentbrew", Board: "careers.aldi.us"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1: %+v", len(jobs), jobs)
	}
	j := jobs[0]
	if j.ExternalID != "100438924608" || j.URL != "https://careers.aldi.us/job/hamburg/cashier/61/100438924608" {
		t.Errorf("identity = %q %q", j.ExternalID, j.URL)
	}
	if j.Title != "Cashier" || j.Company != "ALDI" || j.Location != "Hamburg, New York" {
		t.Errorf("fields = %q %q %q", j.Title, j.Company, j.Location)
	}
	if strings.Contains(j.Description, "<iframe") || !strings.Contains(j.Description, "<p>") {
		t.Errorf("Description not sanitized: %q", j.Description)
	}
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("PostedAt = %v", j.PostedAt)
	}
}

func TestTalentbrewPaginatesAndKeepsLocalePrefix(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/job/a/1", tbDetail("A", "x", "2026-11-03")).
		route("/job/b/2", tbDetail("B", "x", "2026-11-03")).
		route("CurrentPage=2", tbListing(2, [2]string{"/en/job/b/2", "2"})).
		route("CurrentPage=1", tbListing(2, [2]string{"/en/job/a/1", "1"}))

	jobs, err := NewTalentbrew(fake).Fetch(context.Background(), CompanyEntry{Company: "BAT", Board: "careers.bat.com/en"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 || jobs[0].ExternalID != "1" || jobs[1].ExternalID != "2" {
		t.Fatalf("jobs = %+v", jobs)
	}
	if jobs[1].URL != "https://careers.bat.com/en/job/b/2" {
		t.Errorf("URL = %q", jobs[1].URL)
	}
}

func TestTalentbrewSeenPostingIsLivenessRefreshOnly(t *testing.T) {
	fake := (&routedHTTP{}).route("search-jobs/results", tbListing(1, [2]string{"/job/a/1", "1"}))
	jobs, err := NewTalentbrew(fake).(HydratingSource).FetchNew(context.Background(), CompanyEntry{Company: "ALDI", Board: "careers.aldi.us"},
		func(id string) bool { return id == "1" })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 1 || !jobs[0].SeenRefresh || jobs[0].ExternalID != "1" {
		t.Fatalf("jobs = %+v", jobs)
	}
}

func TestTalentbrewFailsWhenNoDetailIsReadable(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/job/a/1", `<html><body>nothing</body></html>`).
		route("search-jobs/results", tbListing(1, [2]string{"/job/a/1", "1"}))
	jobs, err := NewTalentbrew(fake).Fetch(context.Background(), CompanyEntry{Company: "ALDI", Board: "careers.aldi.us"})
	if err == nil {
		t.Fatalf("a crawl that read none of its new postings must fail, got %+v", jobs)
	}
}

func TestTalentbrewBoardIdentityErrors(t *testing.T) {
	for _, board := range []string{"", "aldi", "https://careers.aldi.us", "careers.aldi.us?x=1", "/careers.aldi.us", "careers aldi.us"} {
		_, err := NewTalentbrew(&routedHTTP{}).Fetch(context.Background(), CompanyEntry{Board: board})
		if err == nil {
			t.Errorf("board %q: want error", board)
		}
	}
}

func TestTalentbrewListingErrorFailsTheBoard(t *testing.T) {
	fake := (&routedHTTP{}).routeErr("search-jobs/results", fmt.Errorf("boom"))
	if _, err := NewTalentbrew(fake).Fetch(context.Background(), CompanyEntry{Board: "careers.aldi.us"}); err == nil {
		t.Fatal("want listing error")
	}
}

func TestTalentbrewDeclaredSingleEmptyPage(t *testing.T) {
	fake := (&routedHTTP{}).route("search-jobs/results", tbListing(1))
	jobs, err := NewTalentbrew(fake).Fetch(context.Background(), CompanyEntry{Board: "careers.aldi.us"})
	if err != nil || len(jobs) != 0 {
		t.Fatalf("declared single empty page: jobs=%v err=%v", jobs, err)
	}
}

func TestTalentbrewPaginationBoundRequiresCompletion(t *testing.T) {
	for _, pages := range []int{100, 101} {
		t.Run(fmt.Sprintf("declared_%d_pages", pages), func(t *testing.T) {
			fake := &routedHTTP{}
			for page := 1; page <= 100; page++ {
				fake.route(fmt.Sprintf("CurrentPage=%d&", page), tbListing(pages,
					[2]string{fmt.Sprintf("/job/a/%d", page), fmt.Sprint(page)}))
			}
			jobs, err := NewTalentbrew(fake).(HydratingSource).FetchNew(context.Background(),
				CompanyEntry{Board: "careers.aldi.us"}, func(string) bool { return true })
			if pages == 100 && (err != nil || len(jobs) != 100) {
				t.Fatalf("complete 100-page listing: jobs=%d err=%v", len(jobs), err)
			}
			if pages == 101 && (err == nil || len(jobs) != 0) {
				t.Fatalf("100 of 101 pages cannot authorize a sweep: jobs=%d err=%v", len(jobs), err)
			}
		})
	}
}

func TestTalentbrewIncompleteListingFailsBeforeHydration(t *testing.T) {
	first := tbListing(2, [2]string{"/job/a/1", "1"})
	for _, tc := range []struct {
		name, first, second string
		secondErr           error
	}{
		{name: "later fetch failure", first: first, secondErr: fmt.Errorf("temporary failure")},
		{name: "later malformed JSON", first: first, second: `{`},
		{name: "missing results", first: `{}`},
		{name: "missing section", first: `{"results":"<a data-job-id=1 href=/job/a/1>A</a>"}`},
		{name: "nested duplicate section", first: `{"results":"<section id=search-results data-total-pages=1><section id=search-results data-total-pages=1><a data-job-id=1 href=/job/a/1>A</a></section></section>"}`},
		{name: "sibling duplicate sections", first: `{"results":"<section id=search-results data-total-pages=1><a data-job-id=1 href=/job/a/1>A</a></section><section id=search-results data-total-pages=1></section>"}`},
		{name: "missing page declaration", first: strings.Replace(first, ` data-total-pages=\"2\"`, "", 1)},
		{name: "invalid page declaration", first: strings.Replace(first, `data-total-pages=\"2\"`, `data-total-pages=\"unknown\"`, 1)},
		{name: "later missing metadata", first: first, second: `{"results":"<a data-job-id=2 href=/job/b/2>B</a>"}`},
		{name: "changed declaration", first: first, second: tbListing(1, [2]string{"/job/b/2", "2"})},
		{name: "early empty page", first: tbListing(3, [2]string{"/job/a/1", "1"}), second: tbListing(3)},
		{name: "empty first page before end", first: tbListing(2)},
		{name: "repeated last page", first: first, second: first},
		{name: "page cap", first: tbListing(tbMaxPages+1, [2]string{"/job/a/1", "1"})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := (&routedHTTP{}).route("CurrentPage=1&", tc.first).route("CurrentPage=2&", tc.second)
			if tc.secondErr != nil {
				fake.routeErr("CurrentPage=2&", tc.secondErr)
			}
			seenCalls := 0
			jobs, err := NewTalentbrew(fake).(HydratingSource).FetchNew(context.Background(),
				CompanyEntry{Company: "ALDI", Board: "careers.aldi.us"}, func(string) bool {
					seenCalls++
					return true
				})
			if err == nil || len(jobs) != 0 || seenCalls != 0 {
				t.Fatalf("incomplete listing must fail before refresh/detail processing: jobs=%v err=%v seen calls=%d", jobs, err, seenCalls)
			}
		})
	}
}
