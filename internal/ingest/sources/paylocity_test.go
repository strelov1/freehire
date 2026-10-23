package sources

import (
	"context"
	"strings"
	"testing"
)

// paylocityListingHTML is a recruiting.paylocity.com /Recruiting/Jobs/All/<guid> page: the
// job list rides in the window.pageData blob's Jobs[] array (id/title/location/date/remote).
const paylocityListingHTML = `<html><body>
<script>window.pageData = {"CookieBannerScriptSource":"x","Departments":["All Departments"],"IsLeadJoinEnabled":true,"Jobs":[
{"JobId":4026535,"JobTitle":"Technician Aide","LocationName":"CBAH - Airport Rd.","PublishedDate":"2026-06-29T11:55:56-05:00","IsRemote":false},
{"JobId":3836522,"JobTitle":"Associate Veterinarian","LocationName":"Remote","PublishedDate":"2026-03-19T16:53:34-05:00","IsRemote":true}
]};</script>
</body></html>`

// paylocityDetailHTML is a /Recruiting/Jobs/Details/<id> page. Paylocity dropped the
// schema.org ld+json (the page is now a client-rendered shell), but the description still
// renders server-side as the <div> following the "Description" section header. A trailing
// <script> in that block must be stripped by sanitizeHTML; the sibling "Job Type" section
// proves the selector picks the Description block, not the first header.
const paylocityDetailHTML = `<html><body>
<div class="job-preview-details">
  <div class="vertical-padding">
    <div class="job-listing-header">Job Type</div>
    <div>Full-time</div>
  </div>
  <div class="job-listing-header">Description</div>
  <div><h2>About</h2><p>Care for animals.</p><script>alert(1)</script></div>
</div>
</body></html>`

func TestPaylocityProvider(t *testing.T) {
	if got := NewPaylocity(nil).Provider(); got != "paylocity" {
		t.Errorf("Provider() = %q, want %q", got, "paylocity")
	}
}

// Paylocity earns fullBoardListing because Fetch's listing GET is a single request whose
// window.pageData embeds the company's whole Jobs array (no pagination), so a listing
// failure aborts the whole Fetch. Per-job detail fetches are best-effort (only downgrade the
// description, never drop the posting).
func TestPaylocityMarkers(t *testing.T) {
	s := NewPaylocity(nil)
	if _, ok := s.(fullBoardListing); !ok {
		t.Error("paylocity should implement the fullBoardListing marker")
	}
}

func TestPaylocityRegisteredAsFullBoardListing(t *testing.T) {
	if !FullBoardListingProviders(All(nil))["paylocity"] {
		t.Error("FullBoardListingProviders(All(nil)) should include paylocity")
	}
}

// A listing fetch failure must abort the whole Fetch, never return a partial result as
// success — the property TestPaylocityMarkers' fullBoardListing claim rests on. An unrouted
// fake (no /Recruiting/Jobs/All/ route) fails exactly as a real listing error would.
func TestPaylocityFetchPropagatesAListingError(t *testing.T) {
	fake := &routedHTTP{}
	if _, err := NewPaylocity(fake).Fetch(context.Background(), CompanyEntry{Board: "acme-guid"}); err == nil {
		t.Fatal("Fetch succeeded despite a listing error")
	}
}

func TestPaylocityFetch(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/Recruiting/Jobs/All/", paylocityListingHTML).
		route("/Recruiting/Jobs/Details/", paylocityDetailHTML)

	jobs, err := NewPaylocity(fake).Fetch(context.Background(),
		CompanyEntry{Company: "Clover Basin Animal Hospital", Board: "1a06dc72-45ee-4c90-a268-fe881bbeb577"})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("jobs = %d, want 2", len(jobs))
	}

	// fetchDetails fans out concurrently, so index is not stable — key by ExternalID.
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	tech, ok := byID["4026535"]
	if !ok {
		t.Fatalf("missing job 4026535; got %v", byID)
	}
	if tech.Title != "Technician Aide" {
		t.Errorf("title = %q", tech.Title)
	}
	if tech.Location != "CBAH - Airport Rd." {
		t.Errorf("location = %q", tech.Location)
	}
	if tech.Company != "Clover Basin Animal Hospital" {
		t.Errorf("company = %q", tech.Company)
	}
	if tech.PostedAt == nil {
		t.Error("posted_at not parsed")
	}
	if tech.Remote {
		t.Error("job wrongly flagged remote")
	}
	if !strings.Contains(tech.Description, "Care for animals") || strings.Contains(tech.Description, "alert(1)") {
		t.Errorf("description not sanitized ld+json body: %q", tech.Description)
	}
	if tech.URL != "https://recruiting.paylocity.com/Recruiting/Jobs/Details/4026535" {
		t.Errorf("url = %q", tech.URL)
	}

	vet := byID["3836522"]
	if !vet.Remote {
		t.Error("IsRemote job not flagged remote")
	}
}

// A board with no openings renders an empty Jobs[] array, which must yield zero jobs
// without an error (not a board-level failure).
func TestPaylocityFetchEmpty(t *testing.T) {
	empty := `<html><body><script>window.pageData = {"Jobs":[]};</script></body></html>`
	fake := (&routedHTTP{}).route("/Recruiting/Jobs/All/", empty)
	jobs, err := NewPaylocity(fake).Fetch(context.Background(), CompanyEntry{Board: "guid"})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("jobs = %d, want 0", len(jobs))
	}
}

// paylocityRemoteWorkerListingHTML carries two briefs captured verbatim from live listings
// (2026-10-18; descriptions truncated). LocationName is the tenant's own label for a location
// record — "Remote Worker", "Concord Office" — and names no place, so the dictionary reads
// nothing from it. JobLocation is the structured record behind the label, and its Country is
// the only place the platform states where the work is: a "Remote Worker" posting with
// Country "USA" is US remote, which the catalogue served as unrestricted (job_reports 2026-10,
// Vector Solutions "Product Manager").
const paylocityRemoteWorkerListingHTML = `<html><body>
<script>window.pageData = {"Jobs":[
{"JobId": 4594212, "JobTitle": "Manager, Intake", "LocationName": "Remote Worker", "ShouldDisplayLocation": true, "PublishedDate": "2026-10-16T15:33:57-05:00", "Description": "AdaptHealth Opportunity – Apply Today!Ab", "IsInternal": false, "HiringDepartment": null, "JobLocation": {"LocationId": 4519575, "ModuleId": 14333, "Name": "Remote Worker", "Address": null, "Address2": null, "Metro": null, "City": null, "State": null, "Zip": null, "Country": "USA", "County": "-", "SmartyAddressId": "f2ac88bb-0cb4-4295-b5d7-2a109fb79097"}, "IsRemote": false, "IndeedRemoteType": 2},
{"JobId":4417556,"JobTitle":"Two-Day Summit 2027","LocationName":"Concord Office","ShouldDisplayLocation":true,"PublishedDate":"2026-08-13T10:54:40-05:00","Description":"If you want to dive deeper into understanding what it is like to work as an accounting professional, Nathan We","IsInternal":false,"HiringDepartment":null,"JobLocation":{"LocationId":4342808,"ModuleId":15120,"Name":"Concord Office","Address":"70 Commercial St, 4th Floor","Address2":null,"Metro":null,"City":"Concord","State":"NH","Zip":"03301-5094","Country":"USA","County":"Merrimack","SmartyAddressId":"84faf7ae-a60e-4cc2-b8aa-b772114cbea9"},"IsRemote":false,"IndeedRemoteType":2}
]};</script>
</body></html>`

func TestPaylocityCountryFromJobLocation(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/Recruiting/Jobs/All/", paylocityRemoteWorkerListingHTML).
		route("/Recruiting/Jobs/Details/", paylocityDetailHTML)

	jobs, err := NewPaylocity(fake).Fetch(context.Background(), CompanyEntry{Company: "AdaptHealth", Board: "guid"})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	for _, id := range []string{"4594212", "4417556"} {
		j, ok := byID[id]
		if !ok {
			t.Fatalf("missing job %s", id)
		}
		if len(j.Countries) != 1 || j.Countries[0] != "us" {
			t.Errorf("job %s countries = %v, want [us]", id, j.Countries)
		}
	}
	if !byID["4594212"].Remote {
		t.Error(`"Remote Worker" posting not flagged remote`)
	}
}

// A brief with no JobLocation (the older listing shape above) states no country, and none
// is invented.
func TestPaylocityNoJobLocationNoCountry(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/Recruiting/Jobs/All/", paylocityListingHTML).
		route("/Recruiting/Jobs/Details/", paylocityDetailHTML)
	jobs, err := NewPaylocity(fake).Fetch(context.Background(), CompanyEntry{Board: "guid"})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	for _, j := range jobs {
		if j.Countries != nil {
			t.Errorf("job %s countries = %v, want nil", j.ExternalID, j.Countries)
		}
	}
}

// paylocityWorkModeListingHTML carries briefs captured from live listings (2026-10-23),
// trimmed to the fields the adapter reads, covering every (IsRemote, IndeedRemoteType) shape
// seen across 290 boards. Each page's label was checked by hand: 4597243 "Hybrid Remote •
// Cincinnati, OH" (freehire report 108, stored as remote before this fix), 4228296 "Hybrid
// Remote • Remote Worker - N/A", 4530110 "Fully Remote", 4528354 "Palm Beach" (no arrangement
// label), 4458200 "Green Valley, AZ" (no arrangement label, no IndeedRemoteType at all).
const paylocityWorkModeListingHTML = `<html><body>
<script>window.pageData = {"Jobs":[
{"JobId":4597243,"JobTitle":"Senior Network Administrator","LocationName":"Cincinnati","PublishedDate":"2026-10-20T09:00:00-04:00","IsRemote":true,"IndeedRemoteType":1},
{"JobId":4228296,"JobTitle":"Account Manager","LocationName":"Remote Worker - N/A","PublishedDate":"2026-05-01T09:00:00-04:00","IsRemote":true,"IndeedRemoteType":1},
{"JobId":4530110,"JobTitle":"Telecommunications Technician","LocationName":"","PublishedDate":"2026-09-01T09:00:00-04:00","IsRemote":true,"IndeedRemoteType":2},
{"JobId":4528354,"JobTitle":"System Administrator (Palm Beach, FL Area)","LocationName":"Palm Beach","PublishedDate":"2026-09-01T09:00:00-04:00","IsRemote":false,"IndeedRemoteType":2},
{"JobId":4458200,"JobTitle":"Caregiver","LocationName":"Green Valley, AZ","PublishedDate":"2026-08-01T09:00:00-04:00","IsRemote":false}
]};</script>
</body></html>`

// The work mode follows the posting page's own label: IsRemote alone filed every "Hybrid
// Remote" posting as remote. IsRemote false states nothing, so the mode is left for the
// location and description to decide rather than asserted onsite.
func TestPaylocityWorkMode(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/Recruiting/Jobs/All/", paylocityWorkModeListingHTML).
		route("/Recruiting/Jobs/Details/", paylocityDetailHTML)
	jobs, err := NewPaylocity(fake).Fetch(context.Background(), CompanyEntry{Company: "Powernet", Board: "guid"})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	cases := []struct {
		id         string
		wantMode   string
		wantRemote bool
	}{
		{"4597243", "hybrid", false},
		{"4228296", "hybrid", false},
		{"4530110", "remote", true},
		{"4528354", "", false},
		{"4458200", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			j, ok := byID[tc.id]
			if !ok {
				t.Fatalf("missing job %s", tc.id)
			}
			if j.WorkMode != tc.wantMode {
				t.Errorf("WorkMode = %q, want %q", j.WorkMode, tc.wantMode)
			}
			if j.Remote != tc.wantRemote {
				t.Errorf("Remote = %v, want %v", j.Remote, tc.wantRemote)
			}
		})
	}
}
