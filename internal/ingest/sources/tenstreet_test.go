package sources

import (
	"context"
	"errors"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"

	"golang.org/x/net/html"
)

// The fixtures are live Tenstreet responses fetched on 2026-10-20, with only Cloudflare's
// inline challenge-beacon script removed: Podium Logistics Express' Pulse profile (company id
// 1492393, three open postings, one of them carded twice) and its three posting pages; the
// profile the platform serves for an unknown company id; and the page a posting id answers
// once the posting is gone.

const tenstreetPodium = "1492393"

func tenstreetFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/tenstreet/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// tenstreetFake serves fixed bodies by URL; an unknown URL is a 404.
type tenstreetFake struct {
	finals map[string]string
	mu     sync.Mutex
	pages  map[string]string
	gets   []string
}

func (f *tenstreetFake) GetHTML(_ context.Context, u string) (*html.Node, error) {
	f.mu.Lock()
	f.gets = append(f.gets, u)
	f.mu.Unlock()
	body, ok := f.pages[u]
	if !ok {
		return nil, &StatusError{Method: "GET", Code: 404, URL: u}
	}
	return html.Parse(strings.NewReader(body))
}

func tenstreetPodiumPages(t *testing.T) map[string]string {
	return map[string]string{
		"https://pulse.tenstreet.com/share_app_index.php?company_id=" + tenstreetPodium: tenstreetFixture(t, "podium_listing.html"),
		tenstreetPostingURL(tenstreetPodium, "83575"):                                   tenstreetFixture(t, "podium_job_83575.html"),
		tenstreetPostingURL(tenstreetPodium, "83647"):                                   tenstreetFixture(t, "podium_job_83647.html"),
		tenstreetPostingURL(tenstreetPodium, "83960"):                                   tenstreetFixture(t, "podium_job_83960.html"),
	}
}

func TestTenstreetListingReadsCarrierAndDistinctPostings(t *testing.T) {
	f := &tenstreetFake{pages: tenstreetPodiumPages(t)}
	l, err := ReadTenstreetListing(context.Background(), f, tenstreetPodium)
	if err != nil {
		t.Fatal(err)
	}
	if l.Company != "Podium Logistics Express, LLC" {
		t.Errorf("company = %q", l.Company)
	}
	// 83960 is carded twice on the live page; it is one posting.
	if !slices.Equal(l.PostingIDs, []string{"83575", "83647", "83960"}) {
		t.Errorf("posting ids = %v", l.PostingIDs)
	}
}

func TestTenstreetListingIgnoresAnotherCarriersPosting(t *testing.T) {
	root, err := html.Parse(strings.NewReader(`<span id="carrier_detail_company_name">Acme</span>
<div onclick="window.location='https://pulse.tenstreet.com/share_app_show_active_jobs.php?active_job_id=1&company_id=10&portal_user_id=1'"></div>
<div onclick="window.location='https://pulse.tenstreet.com/share_app_show_active_jobs.php?active_job_id=2&company_id=11&portal_user_id=1'"></div>
<div onclick="window.location='https://pulse.tenstreet.com/share_app_index.php?company_id=10'"></div>`))
	if err != nil {
		t.Fatal(err)
	}
	l := parseTenstreetListing(root, "10")
	if l.Company != "Acme" || !slices.Equal(l.PostingIDs, []string{"1"}) {
		t.Errorf("listing = %+v", l)
	}
}

func TestTenstreetUnknownCompanyIsBoardGone(t *testing.T) {
	f := &tenstreetFake{pages: map[string]string{
		"https://pulse.tenstreet.com/share_app_index.php?company_id=999999999": tenstreetFixture(t, "unknown_company_listing.html"),
	}}
	_, err := NewTenstreet(f).Fetch(context.Background(), CompanyEntry{Board: "999999999", Company: "X"})
	if !errors.Is(err, ErrBoardGone) {
		t.Fatalf("err = %v, want ErrBoardGone", err)
	}
}

func TestTenstreetRejectsNonNumericBoard(t *testing.T) {
	f := &tenstreetFake{}
	for _, b := range []string{"", "acme", "774&x=1", "../774"} {
		if _, err := NewTenstreet(f).Fetch(context.Background(), CompanyEntry{Board: b}); err == nil {
			t.Errorf("board %q accepted", b)
		}
	}
	if len(f.gets) != 0 {
		t.Errorf("requests sent for invalid boards: %v", f.gets)
	}
}

func TestTenstreetPostingMapsTitleBodyAndStates(t *testing.T) {
	f := &tenstreetFake{pages: tenstreetPodiumPages(t)}
	jobs, err := NewTenstreet(f).Fetch(context.Background(), CompanyEntry{Board: tenstreetPodium, Company: "Podium Logistics"})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 3 {
		t.Fatalf("jobs = %d, want 3", len(jobs))
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	j := byID["83960"]
	if j.Title != "CDL Company Driver" || j.Company != "Podium Logistics" {
		t.Errorf("83960 = %q / %q", j.Title, j.Company)
	}
	if j.URL != "https://pulse.tenstreet.com/share_app_show_active_jobs.php?active_job_id=83960&company_id=1492393&portal_user_id=1" {
		t.Errorf("url = %q", j.URL)
	}
	// Fifteen location rows over two states, deduplicated in page order.
	if j.Location != "SC, GA, US" {
		t.Errorf("location = %q", j.Location)
	}
	if !slices.Equal(j.Countries, []string{"us"}) {
		t.Errorf("countries = %v", j.Countries)
	}
	if !strings.Contains(j.Description, "At Podium Logistics, our drivers are the engine") ||
		!strings.Contains(j.Description, "<h3>Requirements</h3>") ||
		!strings.Contains(j.Description, "Valid Commercial Driver") {
		t.Errorf("description = %.300s", j.Description)
	}
	if j.PostedAt != nil || j.WorkMode != "" {
		t.Errorf("a fact was read from nowhere: posted=%v mode=%q", j.PostedAt, j.WorkMode)
	}

	if j := byID["83575"]; j.Title != "CDL A Regional Flatbed Owner Operator" || j.Location != "GA, US" {
		t.Errorf("83575 = %q / %q", j.Title, j.Location)
	}
	// 83647 carries no requirements (null), so no empty section is added.
	if j := byID["83647"]; strings.Contains(j.Description, "Requirements") || j.Description == "" {
		t.Errorf("83647 description = %.300s", j.Description)
	}
}

func TestTenstreetFetchHydratesOnlyNewPostings(t *testing.T) {
	f := &tenstreetFake{pages: tenstreetPodiumPages(t)}
	seen := func(id string) bool { return id != "83647" }
	jobs, err := tenstreet{http: f}.FetchNew(context.Background(), CompanyEntry{Board: tenstreetPodium, Company: "Podium"}, seen)
	if err != nil {
		t.Fatal(err)
	}
	var refreshed, hydrated int
	for _, j := range jobs {
		if j.SeenRefresh {
			refreshed++
			if j.URL == "" || j.Company != "Podium" {
				t.Errorf("seen refresh lacks identity: %+v", j)
			}
		} else {
			hydrated++
		}
	}
	if refreshed != 2 || hydrated != 1 {
		t.Errorf("refreshed=%d hydrated=%d", refreshed, hydrated)
	}
	if len(f.gets) != 2 { // the listing and the one new posting
		t.Errorf("requests = %v", f.gets)
	}
}

func TestTenstreetGonePostingIsDeferred(t *testing.T) {
	pages := tenstreetPodiumPages(t)
	pages[tenstreetPostingURL(tenstreetPodium, "83647")] = tenstreetFixture(t, "posting_gone.html")
	delete(pages, tenstreetPostingURL(tenstreetPodium, "83575")) // a 404
	f := &tenstreetFake{pages: pages}
	jobs, err := NewTenstreet(f).Fetch(context.Background(), CompanyEntry{Board: tenstreetPodium, Company: "Podium"})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "83960" {
		t.Errorf("jobs = %+v", jobs)
	}

	// Every new posting unreadable fails the crawl rather than reporting an empty board.
	f = &tenstreetFake{pages: map[string]string{
		"https://pulse.tenstreet.com/share_app_index.php?company_id=" + tenstreetPodium: tenstreetFixture(t, "podium_listing.html"),
	}}
	if _, err := NewTenstreet(f).Fetch(context.Background(), CompanyEntry{Board: tenstreetPodium, Company: "Podium"}); err == nil {
		t.Error("a crawl that read no posting succeeded")
	}
}

func TestTenstreetPostingUnderAnotherIDIsNotRead(t *testing.T) {
	root, err := html.Parse(strings.NewReader(tenstreetFixture(t, "podium_job_83960.html")))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := parseTenstreetPosting(root, "83575"); ok {
		t.Error("a page carrying posting 83960 was read as 83575")
	}
}

// A posting the platform publishes with no description and no requirements (seen live on
// Dupré Logistics, 2026-10-20) is yielded body-less, not deferred forever.
func TestTenstreetPostingWithoutBodyIsYielded(t *testing.T) {
	root, err := html.Parse(strings.NewReader(`<html><body><script>
		var map_array = [];
		var active_job_array = [{"job_title":"EDI Integration Analyst","job_description":null,"job_requirements":null,"job_pay":null,"location_type":"zip_radius","zip":"70508","zip_within":"25","ident":"zip","value":"705","state":"LA","company_id":"2337095","active_job_id":"63879"}];
		map_array.push(['State', 'Number of Jobs']);
	</script></body></html>`))
	if err != nil {
		t.Fatal(err)
	}
	j, ok := parseTenstreetPosting(root, "63879")
	if !ok || j.Title != "EDI Integration Analyst" || j.Description != "" || j.Location != "LA, US" {
		t.Errorf("job = %+v, ok = %v", j, ok)
	}
}

func TestTenstreetLocationOnlyArrayUsesRenderedPosting(t *testing.T) {
	root, err := html.Parse(strings.NewReader(`<td id="job_title">CDL A Company Driver</td><table><tr><td id="job_title">CDL A Company Driver</td><td id="job_description"><p>Great pay</p></td><td id="job_requirements">License required</td></tr></table><script>var active_job_array = [{"location_type":"select_states","value":"AL"}];</script>`))
	if err != nil {
		t.Fatal(err)
	}
	j, ok := parseTenstreetPosting(root, "6343")
	if !ok || j.Title != "CDL A Company Driver" || !strings.Contains(j.Description, "Great pay") || j.Location != "AL, US" {
		t.Fatalf("%+v ok=%v", j, ok)
	}
}

func (f *tenstreetFake) GetHTMLResolved(ctx context.Context, u string) (*html.Node, string, error) {
	root, err := f.GetHTML(ctx, u)
	if final, ok := f.finals[u]; ok {
		return root, final, err
	}
	return root, u, err
}
func TestTenstreetRedirectCannotReplacePostingIdentity(t *testing.T) {
	listing := "https://pulse.tenstreet.com/share_app_index.php?company_id=10"
	target := tenstreetPostingURL("10", "6343")
	fake := &tenstreetFake{pages: map[string]string{
		listing: `<span id="carrier_detail_company_name">Carrier</span><div onclick="window.location='` + target + `'"></div>`,
		target:  `<table><tr><td id="job_title">Wrong posting</td><td id="job_description">Wrong body</td></tr></table><script>var active_job_array = [{"location_type":"select_states","value":"AL"}];</script>`,
	}, finals: map[string]string{target: tenstreetPostingURL("10", "999")}}
	if _, err := NewTenstreet(fake).(HydratingSource).FetchNew(context.Background(), CompanyEntry{Board: "10"}, func(string) bool { return false }); err == nil {
		t.Fatal("different final posting identity accepted")
	}
}
