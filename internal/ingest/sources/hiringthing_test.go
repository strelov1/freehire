package sources

import (
	"context"
	"html"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

// htListingHTML builds a HiringThing listing page linking each given posting URL. The platform
// links a posting twice per card (title and "Learn more"), which the enumeration de-duplicates.
func htListingHTML(jobURLs ...string) string {
	var b strings.Builder
	b.WriteString(`<html><body><div class="jobs-list-container">`)
	for _, u := range jobURLs {
		b.WriteString(`<div class="job-container"><a href="` + u + `"><h2>A job</h2></a>` +
			`<a href="` + u + `">Learn more</a></div>`)
	}
	b.WriteString(`<a href="/privacy">Privacy</a></div></body></html>`)
	return b.String()
}

// htDetailHTML builds a posting page mounting the components the platform really mounts, with
// the record-bearing one LAST: a page whose first data-react-props is another component's must
// still yield the record.
func htDetailHTML(record string) string {
	return `<html><body>` +
		`<div data-react-class="HiringThing.Components.JobSalary" ` +
		`data-react-props="` + html.EscapeString(`{"payFrequency":"hourly"}`) + `"></div>` +
		`<div data-react-class="HiringThing.Components.ApplyButtonGroup" ` +
		`data-react-props="` + html.EscapeString(`{"formID":"application-form-container","jobObj":{"table":`+record+`}}`) + `"></div>` +
		`<div data-react-class="HiringThing.Components.CookieBanner" data-react-props="{}"></div>` +
		`</body></html>`
}

// htRecord is one posting record in the shape the platform inlines it.
func htRecord(title, description, postedAt, location, country string, remote bool, salary string) string {
	return `{"id":1052705,"title":"` + title + `","html_description":"` + description + `",` +
		`"posted_at":"` + postedAt + `","location":"` + location + `",` +
		`"location_info":{"country":"` + country + `","city":"Windsor Locks","state":"CT"},` +
		`"remote":` + strconv.FormatBool(remote) + `,` + salary + `}`
}

// htNoSalary is the compensation block of a posting stating none: the bounds are empty objects
// even though a pay frequency is always present.
const htNoSalary = `"min_salary":{},"max_salary":{},"pay_frequency":"hourly"`

func TestHiringThingProvider(t *testing.T) {
	if got := NewHiringThing(nil).Provider(); got != "hiringthing" {
		t.Errorf("Provider() = %q, want %q", got, "hiringthing")
	}
}

func TestHiringThingRegisteredInAll(t *testing.T) {
	s, ok := All(nil)["hiringthing"]
	if !ok {
		t.Fatal("All() missing provider hiringthing")
	}
	if s.Provider() != "hiringthing" {
		t.Errorf("All()[hiringthing].Provider() = %q", s.Provider())
	}
}

func TestHTJobID(t *testing.T) {
	cases := map[string]string{
		"https://skijapan.hiringthing.com/job/1052705/guest-services-admin-hakuba": "1052705",
		"https://crown-shredding-llc.prismhr-hire.com/job/952170/mobile-shred":     "952170",
		"/job/1054668/registered-occupational-therapist":                           "1054668",
		"https://ae.elevate-ats.com/job/1054745":                                   "1054745",
		// A permalink is still one when something follows the id, which is why the pattern
		// anchors nothing after it (and why the harvest prober agrees with this adapter).
		"https://skijapan.hiringthing.com/job/1052705/slug?src=x": "1052705",
		"https://skijapan.hiringthing.com/job/1052705#apply":      "1052705",
		"https://skijapan.hiringthing.com/jobs":                   "",
		"https://skijapan.hiringthing.com/job/apply":              "",
		// An id in another link's query or fragment names no posting: only the path is matched.
		"https://skijapan.hiringthing.com/privacy?next=/job/1052705": "",
		"https://skijapan.hiringthing.com/privacy#/job/1052705":      "",
	}
	for u, want := range cases {
		if got := htJobID(u); got != want {
			t.Errorf("htJobID(%q) = %q, want %q", u, got, want)
		}
	}
}

func TestHTJobRecordFoundByComponentNotByOrder(t *testing.T) {
	// The record-bearing component is neither the first nor the only one mounting props, so a
	// decoder that took the first data-react-props on the page would read a salary widget.
	page := htDetailHTML(htRecord("Mobile Shred Operator", "<p>Drive.</p>",
		"2026-08-17T12:30:54Z", "Orlando, FL", "US", false, htNoSalary))
	rec, ok := htJobRecord(parseHTML(t, page))
	if !ok {
		t.Fatal("htJobRecord() ok = false, want true")
	}
	if rec.Title != "Mobile Shred Operator" {
		t.Errorf("Title = %q", rec.Title)
	}
	if rec.Location != "Orlando, FL" || rec.LocationInfo.Country != "US" {
		t.Errorf("location = %q / %q", rec.Location, rec.LocationInfo.Country)
	}
}

func TestHTJobRecordAbsent(t *testing.T) {
	page := `<html><body><div data-react-class="HiringThing.Components.CookieBanner" ` +
		`data-react-props="{}"></div></body></html>`
	if _, ok := htJobRecord(parseHTML(t, page)); ok {
		t.Error("htJobRecord() ok = true, want false when the page mounts no record")
	}
}

func TestHiringThingFetchListingThenDetailAndMaps(t *testing.T) {
	jobURL := "https://crown-shredding-llc.prismhr-hire.com/job/952170/mobile-shred-operator"
	detail := htDetailHTML(htRecord("Mobile Shred Operator",
		"<p>Drive the <b>truck</b>.</p><script>alert(1)</script>",
		"2026-08-17T12:30:54Z", "Orlando, FL", "US", false,
		`"min_salary":{"amount":"18.50","currency":"USD"},`+
			`"max_salary":{"amount":"21.00","currency":"USD"},"pay_frequency":"hourly"`))
	// Detail routes come first: the listing's URL is a prefix of every posting URL.
	fake := (&routedHTTP{}).route("/job/952170", detail).route("prismhr-hire.com", htListingHTML(jobURL))

	jobs, err := NewHiringThing(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Crown Shredding", Provider: "hiringthing",
		Board: "crown-shredding-llc.prismhr-hire.com",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (the card's two links are one posting)", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "952170" {
		t.Errorf("ExternalID = %q, want 952170", j.ExternalID)
	}
	if j.URL != jobURL {
		t.Errorf("URL = %q, want %q", j.URL, jobURL)
	}
	if j.Title != "Mobile Shred Operator" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "Crown Shredding" {
		t.Errorf("Company = %q", j.Company)
	}
	if j.Location != "Orlando, FL" {
		t.Errorf("Location = %q", j.Location)
	}
	if strings.Contains(j.Description, "<script>") || !strings.Contains(j.Description, "<b>truck</b>") {
		t.Errorf("Description not sanitized: %q", j.Description)
	}
	if !reflect.DeepEqual(j.Countries, []string{"us"}) {
		t.Errorf("Countries = %v, want [us]", j.Countries)
	}
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2026, 8, 17, 12, 30, 54, 0, time.UTC)) {
		t.Errorf("PostedAt = %v", j.PostedAt)
	}
	if j.SalaryMin == nil || *j.SalaryMin != 19 || j.SalaryMax == nil || *j.SalaryMax != 21 {
		t.Errorf("salary bounds = %v/%v, want 19/21 (the decimal string is rounded)", j.SalaryMin, j.SalaryMax)
	}
	if j.SalaryCurrency != "USD" || j.SalaryPeriod != "hour" {
		t.Errorf("salary = %q %q, want USD hour", j.SalaryCurrency, j.SalaryPeriod)
	}
	if j.Remote || j.WorkMode != "" {
		t.Errorf("Remote/WorkMode = %v/%q, want false and unset", j.Remote, j.WorkMode)
	}
}

func TestHiringThingEnumeratesOnePostingPerID(t *testing.T) {
	// The slug is not part of a posting's identity, so the same id linked under two slugs is one
	// posting: enumerating by URL would buy its page twice and emit two jobs sharing one dedup key.
	detail := htDetailHTML(htRecord("Kept", "<p>x</p>",
		"2026-08-17T12:30:54Z", "Orlando, FL", "US", false, htNoSalary))
	fake := (&routedHTTP{}).
		route("/job/111", detail).
		route("https://b/", htListingHTML("https://b/job/111/kept", "https://b/job/111/kept-renamed"))

	jobs, err := NewHiringThing(fake).Fetch(context.Background(), CompanyEntry{Board: "b"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "111" {
		t.Fatalf("got %v, want the one posting", jobs)
	}
}

func TestHiringThingSkipsPostingsLinkedOffTheBoardsHost(t *testing.T) {
	// A board IS a host here, so a posting linked on a sibling tenant's host is another board's:
	// crawling it would file a second employer's posting under this board's company.
	detail := htDetailHTML(htRecord("Ours", "<p>x</p>",
		"2026-08-17T12:30:54Z", "Orlando, FL", "US", false, htNoSalary))
	fake := (&routedHTTP{}).
		route("/job/111", detail).
		route("https://b/", htListingHTML(
			"https://b/job/111/ours", "https://elsewhere.example/job/222/theirs"))

	jobs, err := NewHiringThing(fake).Fetch(context.Background(), CompanyEntry{Board: "b"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "111" {
		t.Fatalf("got %v, want only this board's posting", jobs)
	}
}

func TestHiringThingRemoteComesFromTheRecordFlag(t *testing.T) {
	jobURL := "https://b/job/9/revit-field-technician"
	detail := htDetailHTML(htRecord("Revit Field Technician", "<p>x</p>",
		"2026-07-06T11:21:20-04:00", "Remote - Windsor Locks, CT", "US", true, htNoSalary))
	fake := (&routedHTTP{}).route("/job/9", detail).route("https://b/", htListingHTML(jobURL))

	jobs, err := NewHiringThing(fake).Fetch(context.Background(), CompanyEntry{Board: "b"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || !jobs[0].Remote || jobs[0].WorkMode != "remote" {
		t.Fatalf("want one remote job with WorkMode remote, got %+v", jobs)
	}
}

func TestHiringThingRemoteIgnoresLocationText(t *testing.T) {
	// The platform writes the "Remote - " prefix FROM the flag, so a record that leaves the flag
	// false is not remote whatever its location reads — and a place name is never the signal.
	jobURL := "https://b/job/9/site-manager"
	detail := htDetailHTML(htRecord("Site Manager", "<p>x</p>",
		"2026-07-06T11:21:20-04:00", "Remote, OR", "US", false, htNoSalary))
	fake := (&routedHTTP{}).route("/job/9", detail).route("https://b/", htListingHTML(jobURL))

	jobs, err := NewHiringThing(fake).Fetch(context.Background(), CompanyEntry{Board: "b"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].Remote {
		t.Fatalf("a place named Remote must not flag remote, got %+v", jobs)
	}
}

func TestHTPostingSalary(t *testing.T) {
	cases := []struct {
		name                     string
		record                   htPosting
		wantMin, wantMax         int // 0 = expected absent
		wantCurrency, wantPeriod string
	}{
		{
			name: "annual range",
			record: htPosting{
				MinSalary:    htMoney{Amount: "90000.00", Currency: "USD"},
				MaxSalary:    htMoney{Amount: "110000.00", Currency: "USD"},
				PayFrequency: "annually",
			},
			wantMin: 90000, wantMax: 110000, wantCurrency: "USD", wantPeriod: "year",
		},
		{
			// "up to"/"from" ranges are common; one bound is still worth stating.
			name: "one-sided range",
			record: htPosting{
				MinSalary:    htMoney{Amount: "30.00", Currency: "USD"},
				PayFrequency: "hourly",
			},
			wantMin: 30, wantCurrency: "USD", wantPeriod: "hour",
		},
		{
			// pay_frequency is set on every posting, so it alone states nothing.
			name:   "no bounds",
			record: htPosting{PayFrequency: "hourly"},
		},
		{
			// freehire has no weekly period; restating a weekly rate as any other would be wrong.
			name: "period freehire has no value for",
			record: htPosting{
				MinSalary:    htMoney{Amount: "800.00", Currency: "USD"},
				PayFrequency: "weekly",
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotMin, gotMax, gotCurrency, gotPeriod := c.record.salary()
			if want := c.wantMin; (gotMin == nil) != (want == 0) || (gotMin != nil && *gotMin != want) {
				t.Errorf("min = %v, want %d (0 = absent)", gotMin, want)
			}
			if want := c.wantMax; (gotMax == nil) != (want == 0) || (gotMax != nil && *gotMax != want) {
				t.Errorf("max = %v, want %d (0 = absent)", gotMax, want)
			}
			if gotCurrency != c.wantCurrency || gotPeriod != c.wantPeriod {
				t.Errorf("currency/period = %q/%q, want %q/%q",
					gotCurrency, gotPeriod, c.wantCurrency, c.wantPeriod)
			}
		})
	}
}

func TestHiringThingFetchNewSkipsDetailForSeenPostings(t *testing.T) {
	detail := htDetailHTML(htRecord("New Role", "<p>x</p>",
		"2026-08-17T12:30:54Z", "Orlando, FL", "US", false, htNoSalary))
	fake := (&routedHTTP{}).
		route("/job/222", detail).
		route("https://b/", htListingHTML("https://b/job/111/seen", "https://b/job/222/new"))

	jobs, err := NewHiringThing(fake).(HydratingSource).FetchNew(context.Background(),
		CompanyEntry{Company: "Acme", Board: "b"},
		func(id string) bool { return id == "111" })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2", len(jobs))
	}
	byID := map[string]Job{jobs[0].ExternalID: jobs[0], jobs[1].ExternalID: jobs[1]}
	seen, ok := byID["111"]
	if !ok {
		t.Fatal("the seen posting was not re-listed")
	}
	// No route for /job/111: a detail fetch for it would have errored the posting away.
	if !seen.SeenRefresh || seen.Description != "" || seen.Company != "Acme" {
		t.Errorf("seen posting = %+v, want an identity-only refresh", seen)
	}
	fresh := byID["222"]
	if fresh.SeenRefresh || fresh.Title != "New Role" {
		t.Errorf("unseen posting = %+v, want it hydrated", fresh)
	}
}

func TestHiringThingFailedDetailDropsOnlyThatPosting(t *testing.T) {
	detail := htDetailHTML(htRecord("Kept", "<p>x</p>",
		"2026-08-17T12:30:54Z", "Orlando, FL", "US", false, htNoSalary))
	// No route for /job/222 → GetHTML errors → that posting drops, the board survives.
	fake := (&routedHTTP{}).
		route("/job/111", detail).
		route("https://b/", htListingHTML("https://b/job/111/kept", "https://b/job/222/dropped"))

	jobs, err := NewHiringThing(fake).Fetch(context.Background(), CompanyEntry{Board: "b"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "111" {
		t.Fatalf("got %v, want only the kept posting", jobs)
	}
}

func TestHiringThingListingFailureFailsTheBoard(t *testing.T) {
	// An unreachable listing is a board-level error: reporting zero postings instead would
	// look like an emptied board and hand the sweep every posting to close.
	if _, err := NewHiringThing(&routedHTTP{}).Fetch(context.Background(),
		CompanyEntry{Board: "b"}); err == nil {
		t.Fatal("Fetch() err = nil, want the listing failure surfaced")
	}
}

func TestHiringThingEmptyListingYieldsNoJobsNoError(t *testing.T) {
	fake := (&routedHTTP{}).route("https://b/", htListingHTML())
	jobs, err := NewHiringThing(fake).Fetch(context.Background(), CompanyEntry{Board: "b"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs, want 0", len(jobs))
	}
}
