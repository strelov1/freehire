package sources

import (
	"context"
	"strings"
	"testing"
)

// hrmdirectList is the "all openings" listing: one row per (req, req_loc), so requisition 100
// appears twice for its two locations. Each row links its posting from the title anchor with
// the site's own trailing empty parameter and "#job" fragment, and the department filter links
// back to the listing itself.
const hrmdirectList = `<html><body><div class="reqResult">
<table class="reqResultTable">
<tr class="reqitem"><td class="posTitle"><a href="job-opening.php?req=100&amp;req_loc=11&amp;&amp;#job">Senior Platform Engineer</a></td><td>Austin</td></tr>
<tr class="reqitem1"><td class="posTitle"><a href="job-opening.php?req=100&amp;req_loc=12&amp;&amp;#job">Senior Platform Engineer</a></td><td>Remote</td></tr>
<tr class="reqitem"><td class="posTitle"><a href="job-opening.php?req=200&amp;req_loc=21&amp;&amp;#job">Support Lead</a></td><td></td></tr>
<tr class="reqitem1"><td class="posTitle"><a href="job-opening.php?req=200&amp;req_loc=21&amp;&amp;#job">Support Lead</a></td><td></td></tr>
</table>
<a href="job-openings.php?search=true&amp;dept=7">Engineering</a>
</div></body></html>`

// hrmdirectDetailA is a job page. The tenant-authored welcome blurb above the posting carries a
// heading of its own, which is why the adapter reads inside div.reqResult rather than taking the
// document's first h2. The \x92 byte is windows-1252's right single quote: HRM Direct declares
// no charset and serves these raw.
const hrmdirectDetailA = "<html><body>" +
	`<div class="hrmWelcomeContent"><h2>Welcome to Acme</h2></div>` +
	`<div class="reqResult"><h2>Senior Platform Engineer</h2>` +
	`<table class="viewFields">` +
	`<tr><td class="viewFieldName"><b>Department:</b></td><td class="viewFieldValue">Engineering</td></tr>` +
	`<tr><td class="viewFieldName"><b>Location:</b></td><td class="viewFieldValue">Austin, TX<br></td></tr>` +
	`</table>` +
	"<div class=\"jobDesc\"><p>Own the platform\x92s roadmap.</p><script>track()</script></div>" +
	`</div></body></html>`

// hrmdirectDetailB has no city, which the platform renders as a bare ", <state>", and an extra
// tenant-configured field between the two the adapter reads.
const hrmdirectDetailB = `<html><body><div class="reqResult"><h2>Senior Platform Engineer</h2>
<table class="viewFields">
<tr><td class="viewFieldName"><b>Department:</b></td><td class="viewFieldValue">Engineering</td></tr>
<tr><td class="viewFieldName"><b>Office:</b></td><td class="viewFieldValue">HQ</td></tr>
<tr><td class="viewFieldName"><b>Location:</b></td><td class="viewFieldValue">, Remote<br></td></tr>
</table>
<div class="jobDesc"><p>Same role, second location.</p></div>
</div></body></html>`

// hrmdirectDetailC states no location at all — a posting whose city and state columns are both
// empty renders as "".
const hrmdirectDetailC = `<html><body><div class="reqResult"><h2>Support Lead</h2>
<table class="viewFields">
<tr><td class="viewFieldName"><b>Department:</b></td><td class="viewFieldValue">Support</td></tr>
<tr><td class="viewFieldName"><b>Location:</b></td><td class="viewFieldValue"><br></td></tr>
</table>
<div class="jobDesc"><p>Lead the desk.</p></div>
</div></body></html>`

func hrmdirectFake() *routedHTTP {
	// Contains-matching, so the detail routes (whose URLs carry "job-opening.php") precede the
	// listing route; the two never collide, since "job-openings.php" does not contain
	// "job-opening.php".
	return (&routedHTTP{}).
		route("req=100&req_loc=11", hrmdirectDetailA).
		route("req=100&req_loc=12", hrmdirectDetailB).
		route("req=200&req_loc=21", hrmdirectDetailC).
		route("job-openings.php", hrmdirectList)
}

func TestHRMDirectProvider(t *testing.T) {
	if got := NewHRMDirect(nil).Provider(); got != "hrmdirect" {
		t.Errorf("Provider() = %q, want %q", got, "hrmdirect")
	}
}

func TestHRMDirectRef(t *testing.T) {
	cases := []struct {
		url      string
		req, loc string
		ok       bool
	}{
		{"job-opening.php?req=100&req_loc=11&&#job", "100", "11", true},
		{"https://acme.hrmdirect.com/employment/job-opening.php?req=3798827&req_loc=1447910",
			"3798827", "1447910", true},
		// The query is decoded, not pattern-matched, so parameter order and anything sitting
		// between the two ids are irrelevant.
		{"job-opening.php?req_loc=11&src=email&req=100", "100", "11", true},
		// req without req_loc is not a permalink: one requisition spans several locations,
		// and keying on req alone would collapse them onto one dedup key.
		{"job-opening.php?req=100", "", "", false},
		{"job-opening.php?req=100&req_loc=", "", "", false},
		{"job-opening.php?req=abc&req_loc=11", "", "", false},
		{"job-openings.php?search=true&dept=7", "", "", false},
		{"/employment/index.php", "", "", false},
	}
	for _, c := range cases {
		req, loc, ok := hrmdirectRef(c.url)
		if req != c.req || loc != c.loc || ok != c.ok {
			t.Errorf("hrmdirectRef(%q) = (%q, %q, %v), want (%q, %q, %v)",
				c.url, req, loc, ok, c.req, c.loc, c.ok)
		}
	}
}

func TestHRMDirectLocation(t *testing.T) {
	cases := map[string]string{
		"Austin, TX":   "Austin, TX",
		", Remote":     "Remote",
		"Brooks, AB\n": "Brooks, AB",
		"":             "",
		",":            "",
	}
	for raw, want := range cases {
		if got := hrmdirectLocation(raw); got != want {
			t.Errorf("hrmdirectLocation(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestHRMDirectFetchMapsPostings(t *testing.T) {
	jobs, err := NewHRMDirect(hrmdirectFake()).Fetch(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "hrmdirect", Board: "acme",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("got %d jobs, want 3 (two locations of one requisition, plus a de-duped row)", len(jobs))
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	a, ok := byID["100-11"]
	if !ok {
		t.Fatalf("missing job 100-11; got %v", byID)
	}
	if a.Title != "Senior Platform Engineer" || a.Company != "Acme" {
		t.Errorf("job 100-11: got title=%q company=%q (title must come from the posting block, "+
			"not the welcome blurb's heading)", a.Title, a.Company)
	}
	if a.Location != "Austin, TX" {
		t.Errorf("job 100-11 location = %q, want %q", a.Location, "Austin, TX")
	}
	want := "https://acme.hrmdirect.com/employment/job-opening.php?req=100&req_loc=11"
	if a.URL != want {
		t.Errorf("job 100-11 URL = %q, want %q (rebuilt from the ids, not the listing href)", a.URL, want)
	}
	if !strings.Contains(a.Description, "Own the platform’s roadmap") {
		t.Errorf("job 100-11 description = %q, want the windows-1252 apostrophe decoded", a.Description)
	}
	if strings.Contains(a.Description, "<script>") {
		t.Errorf("job 100-11 description not sanitized: %q", a.Description)
	}
	if a.Remote || a.WorkMode != "" {
		t.Errorf("job 100-11: got Remote=%v WorkMode=%q, want an office posting", a.Remote, a.WorkMode)
	}

	b, ok := byID["100-12"]
	if !ok {
		t.Fatalf("missing job 100-12 (the second location of requisition 100)")
	}
	if b.Location != "Remote" {
		t.Errorf("job 100-12 location = %q, want %q (the empty city must not leave a comma)",
			b.Location, "Remote")
	}
	if !b.Remote || b.WorkMode != "remote" {
		t.Errorf("job 100-12: got Remote=%v WorkMode=%q, want the location text to flag it",
			b.Remote, b.WorkMode)
	}

	c, ok := byID["200-21"]
	if !ok {
		t.Fatalf("missing job 200-21")
	}
	if c.Location != "" {
		t.Errorf("job 200-21 location = %q, want empty", c.Location)
	}
	if c.PostedAt != nil {
		t.Errorf("job 200-21 PostedAt = %v, want nil (the platform publishes no date)", c.PostedAt)
	}
}

func TestHRMDirectFetchNewSkipsSeenDetail(t *testing.T) {
	fake := hrmdirectFake()
	jobs, err := NewHRMDirect(fake).(HydratingSource).FetchNew(context.Background(),
		CompanyEntry{Company: "Acme", Provider: "hrmdirect", Board: "acme"},
		func(id string) bool { return id != "200-21" })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("got %d jobs, want 3 (every listed posting is reported, seen or not)", len(jobs))
	}
	// One listing fetch plus the single unseen posting's page.
	if fake.calls != 2 {
		t.Errorf("made %d requests, want 2 (the two seen postings must cost no detail fetch)", fake.calls)
	}
	for _, j := range jobs {
		seen := j.ExternalID != "200-21"
		if j.SeenRefresh != seen {
			t.Errorf("job %s: SeenRefresh = %v, want %v", j.ExternalID, j.SeenRefresh, seen)
		}
		if !seen {
			continue
		}
		// The listing's own title travels, so the catalogue filter can still age the row out;
		// nothing that would rewrite the stored content does.
		if j.Title != "Senior Platform Engineer" {
			t.Errorf("job %s: refresh title = %q, want the listing's row title", j.ExternalID, j.Title)
		}
		if j.Description != "" || j.Location != "" {
			t.Errorf("job %s: a liveness refresh must not carry content, got description=%q location=%q",
				j.ExternalID, j.Description, j.Location)
		}
	}
}

func TestHRMDirectListingFailureIsBoardError(t *testing.T) {
	_, err := NewHRMDirect(&routedHTTP{}).Fetch(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "hrmdirect", Board: "acme",
	})
	if err == nil {
		t.Fatal("Fetch: want an error when the listing cannot be read")
	}
}
