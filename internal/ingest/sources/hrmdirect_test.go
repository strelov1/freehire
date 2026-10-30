package sources

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// hrmdirectCareerPage is the hosted career page an HRM Direct tenant now redirects to
// (apply.clearcompany.com/<board>), trimmed from the live "sms" tenant on 2026-10-30. It renders
// no posting at all: the list is drawn by ClearCompany's widget script, which carries the
// tenant's career-site id the posting API is keyed by.
const hrmdirectCareerPage = `<!DOCTYPE html>
<html lang="en"><head><meta charset="utf-8">
<title>SMS DATA PRODUCTS GROUP, INC &#x2013; Careers</title>
</head><body>
<a class="cc-skip" href="#cc-widget">Skip to jobs</a>
<main class="cc-main">
    <div class="cc-wrap cc-widget" id="cc-widget">
            <script src="https://careers-content.clearcompany.com/js/v1/career-site.js?hosted=true" data-site-id="8a53c87d-2c7f-85ba-20f0-66842148ace3" data-default-source="CJB-0"></script>
    </div>
</main>
</body></html>`

// hrmdirectAPIPage is one page of the career-site posting API, trimmed from the live "sms" tenant on
// 2026-10-30 (two of its postings, descriptions cut, address fields dropped; totalCount set to
// what the trimmed page holds). The first is a remote posting stated in the structured
// locations; the second lists one requisition at five locations, four of them the same town.
const hrmdirectAPIPage = `{
 "results": [
  {
   "id": "2F9F6833-113B-0D9B-5817-09E9BEDC812D",
   "userDefinedId": "2026-5615",
   "positionTitle": "Systems Engineer",
   "description": "<p>SMS is seeking a Mid-Senior level System Engineer to work as part of the BIM Design team.</p><script>track()</script>",
   "postedDate": "2026-10-23T00:00:00",
   "departmentName": "BIM TO 05",
   "location": "Remote (US)",
   "locations": [
    {"city": null, "subdivision": "", "subdivisionFullName": "", "country": "US", "isRemote": true, "isNationwide": true, "isStatewide": false}
   ]
  },
  {
   "id": "86e8505e-7101-13ec-1693-7018e401f109",
   "userDefinedId": "2026-5502",
   "positionTitle": "Information System Security Officer",
   "description": "<p>The Senior Information System Security Officer (ISSO) supports the Information System Security Manager (ISSM).</p>",
   "postedDate": "2026-09-15T00:00:00",
   "departmentName": "AFCENT NOSC",
   "location": "Shaw Afb SC, Sumter SC, Sumter SC, Sumter SC, Sumter SC",
   "locations": [
    {"city": "Shaw Afb", "subdivision": "SC", "subdivisionFullName": "South Carolina", "country": "US", "isRemote": false, "isNationwide": false, "isStatewide": false},
    {"city": "Sumter", "subdivision": "SC", "subdivisionFullName": "South Carolina", "country": "US", "isRemote": false, "isNationwide": false, "isStatewide": false},
    {"city": "Sumter", "subdivision": "SC", "subdivisionFullName": "South Carolina", "country": "US", "isRemote": false, "isNationwide": false, "isStatewide": false},
    {"city": "Sumter", "subdivision": "SC", "subdivisionFullName": "South Carolina", "country": "US", "isRemote": false, "isNationwide": false, "isStatewide": false},
    {"city": "Sumter", "subdivision": "SC", "subdivisionFullName": "South Carolina", "country": "US", "isRemote": false, "isNationwide": false, "isStatewide": false}
   ]
  }
 ],
 "currentPageIndex": 0,
 "totalCount": 2,
 "currentPageCount": 2
}`

const hrmdirectSiteID = "8a53c87d-2c7f-85ba-20f0-66842148ace3"

// hrmdirectFake answers each exact URL with its body, and anything else with an error: the
// career page through GetHTML, the posting API through GetStream.
type hrmdirectFake map[string]string

func (f hrmdirectFake) body(url string) (string, error) {
	body, ok := f[url]
	if !ok {
		return "", fmt.Errorf("unexpected request %s", url)
	}
	return body, nil
}

func (f hrmdirectFake) GetHTML(_ context.Context, url string) (*html.Node, error) {
	body, err := f.body(url)
	if err != nil {
		return nil, err
	}
	return html.Parse(strings.NewReader(body))
}

func (f hrmdirectFake) GetStream(_ context.Context, url, _ string, fn func(io.Reader) error) error {
	body, err := f.body(url)
	if err != nil {
		return err
	}
	return fn(strings.NewReader(body))
}

// hrmdirectListingURL is the one listing request a board costs: the whole board in one page.
var hrmdirectListingURL = fmt.Sprintf("https://careers-api.clearcompany.com/v1/%s?pageIndex=0&pageSize=%d",
	hrmdirectSiteID, hrmdirectMaxPostings)

// hrmdirectBoard is a fake tenant whose career page names the fixture's site and whose listing
// answers listing.
func hrmdirectBoard(listing string) hrmdirectFake {
	return hrmdirectFake{
		"https://apply.clearcompany.com/sms": hrmdirectCareerPage,
		hrmdirectListingURL:                  listing,
	}
}

// hrmdirectEnvelope renders a listing of minimal postings with the given stated total.
func hrmdirectEnvelope(total int, ids ...string) string {
	rows := make([]string, len(ids))
	for i, id := range ids {
		rows[i] = fmt.Sprintf(`{"id":%q,"positionTitle":"Role %s","description":"<p>body</p>","location":"Austin TX",`+
			`"locations":[{"city":"Austin","subdivision":"TX","country":"US","isRemote":false}]}`, id, id)
	}
	return fmt.Sprintf(`{"results":[%s],"totalCount":%d}`, strings.Join(rows, ","), total)
}

var hrmdirectEntry = CompanyEntry{Company: "SMS Data Products", Provider: "hrmdirect", Board: "sms"}

func TestHRMDirectProvider(t *testing.T) {
	if got := NewHRMDirect(nil).Provider(); got != "hrmdirect" {
		t.Errorf("Provider() = %q, want %q", got, "hrmdirect")
	}
}

func TestHRMDirectFetchMapsPostings(t *testing.T) {
	jobs, err := NewHRMDirect(hrmdirectBoard(hrmdirectAPIPage)).Fetch(context.Background(), hrmdirectEntry)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2", len(jobs))
	}

	remote := jobs[0]
	// The id is the posting's own GUID, lower-cased: the widget itself links a posting by the
	// lower-case spelling, and the API answers either.
	if remote.ExternalID != "2f9f6833-113b-0d9b-5817-09e9bedc812d" {
		t.Errorf("ExternalID = %q, want the lower-cased posting GUID", remote.ExternalID)
	}
	if want := "https://apply.clearcompany.com/sms?jobId=2f9f6833-113b-0d9b-5817-09e9bedc812d"; remote.URL != want {
		t.Errorf("URL = %q, want %q", remote.URL, want)
	}
	if remote.Title != "Systems Engineer" || remote.Company != "SMS Data Products" {
		t.Errorf("got title=%q company=%q", remote.Title, remote.Company)
	}
	if remote.Location != "Remote (US)" {
		t.Errorf("Location = %q, want the platform's own location line", remote.Location)
	}
	if !remote.Remote || remote.WorkMode != "remote" {
		t.Errorf("Remote=%v WorkMode=%q, want the structured isRemote flag to mark it remote",
			remote.Remote, remote.WorkMode)
	}
	if len(remote.Countries) != 1 || remote.Countries[0] != "us" {
		t.Errorf("Countries = %v, want [us]", remote.Countries)
	}
	if !strings.Contains(remote.Description, "Mid-Senior level System Engineer") ||
		strings.Contains(remote.Description, "<script>") {
		t.Errorf("Description = %q, want the sanitized listing body", remote.Description)
	}
	if remote.PostedAt == nil || remote.PostedAt.Format("2006-01-02") != "2026-10-23" {
		t.Errorf("PostedAt = %v, want 2026-10-23", remote.PostedAt)
	}

	office := jobs[1]
	if office.Remote || office.WorkMode != "" {
		t.Errorf("office posting: Remote=%v WorkMode=%q, want neither", office.Remote, office.WorkMode)
	}
	if len(office.Countries) != 1 || office.Countries[0] != "us" {
		t.Errorf("office posting Countries = %v, want one us however many locations state it", office.Countries)
	}
}

// A posting the answer repeats is one posting, and the distinct postings must still reach the
// stated total.
func TestHRMDirectFetchDropsARepeatedPosting(t *testing.T) {
	jobs, err := NewHRMDirect(hrmdirectBoard(hrmdirectEnvelope(2, "a", "b", "A"))).Fetch(context.Background(), hrmdirectEntry)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 || jobs[0].ExternalID != "a" || jobs[1].ExternalID != "b" {
		t.Errorf("got %v, want [a b]", jobs)
	}
}

// A listing short of the total it states is a failed crawl, never a partial success: the board
// scope would close every posting it left out. A board past hrmdirectMaxPostings answers
// exactly like this.
func TestHRMDirectFetchFailsAnIncompleteListing(t *testing.T) {
	cases := map[string]string{
		"short":           hrmdirectEnvelope(3, "a", "b"),
		"short by repeat": hrmdirectEnvelope(3, "a", "b", "b"),
	}
	for name, listing := range cases {
		t.Run(name, func(t *testing.T) {
			if jobs, err := NewHRMDirect(hrmdirectBoard(listing)).Fetch(context.Background(), hrmdirectEntry); err == nil {
				t.Errorf("Fetch = %d jobs and no error, want the short listing to fail", len(jobs))
			}
		})
	}
}

func TestHRMDirectFetchEmptyBoard(t *testing.T) {
	jobs, err := NewHRMDirect(hrmdirectBoard(`{"results":[],"currentPageIndex":0,"totalCount":0,"currentPageCount":0}`)).
		Fetch(context.Background(), hrmdirectEntry)
	if err != nil || len(jobs) != 0 {
		t.Errorf("Fetch = (%d jobs, %v), want an empty listing", len(jobs), err)
	}
}

func TestHRMDirectFetchFailures(t *testing.T) {
	cases := map[string]hrmdirectFake{
		// The career page moved and no longer names a site: nothing can be listed, and an
		// empty answer here would read as an emptied board.
		"no site id": {"https://apply.clearcompany.com/sms": `<html><body><main></main></body></html>`},
		// The career page itself is unreachable (an offline tenant answers 404).
		"career page": {},
		// The listing request fails (the API answers an unknown site, and a passing outage
		// measured 2026-10-30, with 404).
		"listing": {"https://apply.clearcompany.com/sms": hrmdirectCareerPage},
		// An answer without its posting collection is not an empty board.
		"no results": hrmdirectBoard(`{"totalCount":0}`),
		"not json":   hrmdirectBoard(`<html>Not Found</html>`),
	}
	for name, fake := range cases {
		t.Run(name, func(t *testing.T) {
			if jobs, err := NewHRMDirect(fake).Fetch(context.Background(), hrmdirectEntry); err == nil {
				t.Errorf("Fetch = %d jobs and no error, want a failed fetch", len(jobs))
			}
		})
	}
}

func TestHRMDirectIsAFullBoardListing(t *testing.T) {
	if !FullBoardListingProviders(All(nil))["hrmdirect"] {
		t.Error("FullBoardListingProviders(All(nil)) should include hrmdirect")
	}
}
