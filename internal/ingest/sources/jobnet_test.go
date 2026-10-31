package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"testing"
)

// jobnetCatalogue is a test HeaderJSONGetter that behaves like the live BFF search: it filters
// by the occupations query parameters (OR, as the live API does), pages newest-first, reports
// the filtered total and the unfiltered occupations facet, and — like the live API — serves only
// the first jobnetWindow results of any query, answering an empty page past it.
type jobnetCatalogue struct {
	ads        []jobnetFakeAd
	extraOcc   int          // added to the facet total, to model a facet that misses ads
	sliceEmpty bool         // a slice (occupation-filtered) query states its total but lists no ads
	failAt     map[int]bool // the nth request (1-based) fails
	requests   int
	gotCSRF    string
}

type jobnetFakeAd struct {
	ad         jobnetAd
	occupation string
}

func (f *jobnetCatalogue) GetJSONWithHeaders(_ context.Context, raw string, headers map[string]string, v any) error {
	f.requests++
	f.gotCSRF = headers["x-csrf"]
	if f.failAt[f.requests] {
		return errors.New("jobnetCatalogue: boom")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	q := u.Query()
	page, _ := strconv.Atoi(q.Get("PageNumber"))
	size, _ := strconv.Atoi(q.Get("ResultsPerPage"))
	occs := q["occupations"]

	var matched []jobnetAd
	facet := map[string]int{}
	for _, a := range f.ads {
		facet[a.occupation]++
		if len(occs) == 0 || slices.Contains(occs, a.occupation) {
			matched = append(matched, a.ad)
		}
	}
	var served []jobnetAd
	listsNothing := f.sliceEmpty && len(occs) > 0
	if start := (page - 1) * size; start < len(matched) && start+size <= jobnetWindow && !listsNothing {
		served = matched[start:min(start+size, len(matched))]
	}
	resp := map[string]any{"totalJobAdCount": len(matched), "jobAds": served}
	var facets []map[string]any
	for id, n := range facet {
		facets = append(facets, map[string]any{"identifier": id, "jobAdCount": n})
	}
	if f.extraOcc != 0 && len(facets) > 0 {
		facets[0]["jobAdCount"] = facets[0]["jobAdCount"].(int) + f.extraOcc
	}
	resp["searchFacets"] = map[string]any{"occupations": facets}
	b, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// jobnetFakeAds builds n ads in one occupation, ids prefixed so occupations never collide.
func jobnetFakeAds(occupation string, n int) []jobnetFakeAd {
	ads := make([]jobnetFakeAd, n)
	for i := range ads {
		ads[i] = jobnetFakeAd{occupation: occupation, ad: jobnetAd{
			JobAdID: fmt.Sprintf("%s-%d", occupation, i), Title: "T", HiringOrgName: "Acme", Description: "x",
		}}
	}
	return ads
}

func fetchJobnet(t *testing.T, f *jobnetCatalogue) ([]Job, error) {
	t.Helper()
	return NewJobnet(f).Fetch(context.Background(), CompanyEntry{Company: "Jobnet", Provider: "jobnet"})
}

func TestJobnetProvider(t *testing.T) {
	if got := NewJobnet(nil).Provider(); got != "jobnet" {
		t.Errorf("Provider() = %q, want %q", got, "jobnet")
	}
}

// jobnet lists its whole catalogue or fails, so it is swept by source: an ad gone from the
// listing closes whatever employer it belonged to.
func TestJobnetRegisteredFullCatalog(t *testing.T) {
	if !slices.Contains(FullCatalogProviders(All(nil)), "jobnet") {
		t.Error("jobnet should be fullCatalog")
	}
}

func TestJobnetFetchMapsAd(t *testing.T) {
	fake := &jobnetCatalogue{ads: []jobnetFakeAd{{occupation: "o1", ad: jobnetAd{
		JobAdID: "c1dc33fa-9c57-4592-a050-36ecda5bdfc1", Title: "IT-supportelev",
		HiringOrgName: "ipnordic A/S", PostalDistrictName: "Gråsten", Country: "Danmark",
		PublicationDate: "2026-07-07T00:00:00+02:00",
		Description:     "<p>Er du nysgerrig</p><script>x()</script>",
	}}}}

	jobs, err := fetchJobnet(t, fake)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "c1dc33fa-9c57-4592-a050-36ecda5bdfc1" {
		t.Errorf("ExternalID = %q", j.ExternalID)
	}
	if j.Title != "IT-supportelev" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "ipnordic A/S" {
		t.Errorf("Company = %q", j.Company)
	}
	// jobAdUrl empty → the canonical id-based posting page is the link.
	if want := "https://job.jobnet.dk/CV/FindWork/Details/c1dc33fa-9c57-4592-a050-36ecda5bdfc1"; j.URL != want {
		t.Errorf("URL = %q, want %q", j.URL, want)
	}
	if j.Location != "Gråsten, Danmark" {
		t.Errorf("Location = %q, want %q", j.Location, "Gråsten, Danmark")
	}
	// HTML is sanitized: the <script> is stripped, the text kept.
	if got := j.Description; got == "" || regexp.MustCompile(`(?i)<script`).MatchString(got) {
		t.Errorf("Description not sanitized: %q", got)
	}
	if j.PostedAt == nil {
		t.Fatal("PostedAt = nil, want parsed date")
	}
	if fake.gotCSRF != "1" {
		t.Errorf("x-csrf header = %q, want %q", fake.gotCSRF, "1")
	}
}

// An external ad carries its own destination URL; the adapter must prefer it over the
// synthesized jobnet page so the link points at the real posting.
func TestJobnetFetchPrefersAdURL(t *testing.T) {
	fake := &jobnetCatalogue{ads: []jobnetFakeAd{{occupation: "o1", ad: jobnetAd{
		JobAdID: "abc", Title: "Dev", HiringOrgName: "Acme", Country: "Danmark",
		JobAdURL: "https://acme.example/jobs/1", Description: "<p>x</p>",
	}}}}
	jobs, err := fetchJobnet(t, fake)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].URL != "https://acme.example/jobs/1" {
		t.Fatalf("URL = %v, want the ad's own jobAdUrl", jobs)
	}
}

// An ad with no id to key on or no employer name (which would break the company slug) is
// dropped rather than emitted, mirroring the other aggregator adapters.
func TestJobnetFetchDropsIncompleteAds(t *testing.T) {
	fake := &jobnetCatalogue{ads: []jobnetFakeAd{
		{occupation: "o1", ad: jobnetAd{JobAdID: "", Title: "No id", HiringOrgName: "Acme"}},
		{occupation: "o1", ad: jobnetAd{JobAdID: "has-id", Title: "No company"}},
		{occupation: "o1", ad: jobnetAd{JobAdID: "ok", Title: "Good", HiringOrgName: "Acme"}},
	}}
	jobs, err := fetchJobnet(t, fake)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "ok" {
		t.Fatalf("got %v, want only the complete ad", jobs)
	}
}

// The search serves only the first 10,000 results of a query, and the catalogue is larger
// (23,169 ads on 2026-10-31): a walk of the unfiltered listing read the newest 10k and called it
// the end. Walking it in occupation slices, each under the window, reads every ad.
func TestJobnetFetchReadsPastTheResultWindow(t *testing.T) {
	var ads []jobnetFakeAd
	ads = append(ads, jobnetFakeAds("big", 5000)...)
	ads = append(ads, jobnetFakeAds("mid", 4500)...)
	ads = append(ads, jobnetFakeAds("small", 3000)...)
	jobs, err := fetchJobnet(t, &jobnetCatalogue{ads: ads})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != len(ads) {
		t.Fatalf("got %d jobs, want all %d", len(jobs), len(ads))
	}
	ids := map[string]bool{}
	for _, j := range jobs {
		ids[j.ExternalID] = true
	}
	if len(ids) != len(ads) {
		t.Errorf("got %d distinct ids, want %d", len(ids), len(ads))
	}
}

// One occupation larger than the window cannot be read whole by any slice, so the crawl fails
// rather than reporting the window as the end.
func TestJobnetFetchFailsWhenAnOccupationOutgrowsTheWindow(t *testing.T) {
	// +50 is less than a page past the window, which the drift tolerance alone would accept.
	for _, over := range []int{50, 500} {
		if _, err := fetchJobnet(t, &jobnetCatalogue{ads: jobnetFakeAds("huge", jobnetWindow+over)}); err == nil {
			t.Errorf("Fetch: want an error when one occupation holds %d ads more than the search serves", over)
		}
	}
}

// Ads the occupation facet does not count would be in no slice and never read, so the crawl
// fails rather than closing them.
func TestJobnetFetchFailsWhenTheFacetMissesAds(t *testing.T) {
	if _, err := fetchJobnet(t, &jobnetCatalogue{ads: jobnetFakeAds("o1", 50), extraOcc: -1}); err == nil {
		t.Fatal("Fetch: want an error when the occupations facet accounts for fewer ads than the total")
	}
}

// A slice that states ads and lists none was not read, however small it is: no drift empties a
// slice, so the crawl fails rather than letting the sweep close the slice's ads.
func TestJobnetFetchFailsOnASliceThatListsNothing(t *testing.T) {
	if _, err := fetchJobnet(t, &jobnetCatalogue{ads: jobnetFakeAds("o1", 50), sliceEmpty: true}); err == nil {
		t.Fatal("Fetch: want an error when a slice states ads and lists none")
	}
}

// Any page failing fails the crawl: a partial list would let the source-scoped sweep close every
// ad the walk did not reach.
func TestJobnetFetchFailsOnAnyPageError(t *testing.T) {
	for req := 1; req <= 3; req++ {
		fake := &jobnetCatalogue{ads: jobnetFakeAds("o1", 250), failAt: map[int]bool{req: true}}
		if _, err := fetchJobnet(t, fake); err == nil {
			t.Errorf("Fetch with request %d failing: want an error", req)
		}
	}
}

// An empty catalogue is what a broken answer looks like, never a real state of the national
// job portal, so it fails rather than letting the sweep close everything.
func TestJobnetFetchFailsOnAnEmptyCatalogue(t *testing.T) {
	if _, err := fetchJobnet(t, &jobnetCatalogue{}); err == nil {
		t.Fatal("Fetch: want an error when the listing states no ads")
	}
}

func TestJobnetSlices(t *testing.T) {
	facet := []jobnetFacet{{"a", 6000}, {"b", 3000}, {"c", 2000}, {"d", 10}, {"e", 10}, {"f", 10}}
	got := jobnetSlices(facet, 9000, 2)
	want := [][]string{{"a", "b"}, {"c", "d"}, {"e", "f"}}
	if !slices.EqualFunc(got, want, slices.Equal) {
		t.Errorf("jobnetSlices = %v, want %v", got, want)
	}
}

// compile-time guard that the fake satisfies the role the adapter depends on.
var _ HeaderJSONGetter = (*jobnetCatalogue)(nil)
