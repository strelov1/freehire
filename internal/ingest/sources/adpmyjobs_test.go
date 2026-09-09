package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// headerRoutedHTTP is routedHTTP's header-carrying sibling: it answers by URL substring and
// RECORDS the headers each call carried, because what authorises an ADP MyJobs listing is a
// request header and a fake that dropped it would let the adapter pass while sending nothing.
type headerRoutedHTTP struct {
	routes []struct{ match, body string }
	sent   []map[string]string
}

func (h *headerRoutedHTTP) route(match, body string) *headerRoutedHTTP {
	h.routes = append(h.routes, struct{ match, body string }{match, body})
	return h
}

func (h *headerRoutedHTTP) GetJSONWithHeaders(_ context.Context, url string, headers map[string]string, v any) error {
	h.sent = append(h.sent, headers)
	for _, r := range h.routes {
		if strings.Contains(url, r.match) {
			return json.Unmarshal([]byte(r.body), v)
		}
	}
	return fmt.Errorf("no route for %s", url)
}

// headersFor returns the headers the call whose URL matched substring carried.
func (h *headerRoutedHTTP) headersFor(t *testing.T, i int) map[string]string {
	t.Helper()
	if i >= len(h.sent) {
		t.Fatalf("only %d calls were made, wanted call %d", len(h.sent), i+1)
	}
	return h.sent[i]
}

const adpMyJobsSiteBody = `{"orgoid":"G2QKPTS5DSF914GQ","clientName":"Stellantis","name":"Stellantis External CX"}`

func adpMyJobsListBody(count int, ids ...string) string {
	var b strings.Builder
	fmt.Fprintf(&b, `{"count":%d,"jobRequisitions":[`, count)
	for i, id := range ids {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"reqId":%q,"jobTitle":"Sr. SQE"}`, id)
	}
	b.WriteString(`]}`)
	return b.String()
}

func adpMyJobsDetailBody(id, descHTML string) string {
	return fmt.Sprintf(`{"count":1,"jobRequisitions":[{"reqId":%q,"jobTitle":"Sr. SQE",
		"postingDate":"2026-09-03T17:28:52Z","jobDescription":%q,
		"requisitionLocations":[{"address":{"cityName":"Auburn Hills",
		"country":{"longName":"United States"},
		"countrySubdivisionLevel1":{"longName":"Michigan"}}}]}]}`, id, descHTML)
}

func TestADPMyJobsResolvesOrgoidThenListsAndHydrates(t *testing.T) {
	fake := (&headerRoutedHTTP{}).
		route("career-site/stellantisexternalcx", adpMyJobsSiteBody).
		route("job-requisitions/5001", adpMyJobsDetailBody("5001", "<p>Do <b>quality</b>.</p><script>x</script>")).
		route("job-requisitions?", adpMyJobsListBody(1, "5001"))

	jobs, err := NewADPMyJobs(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Stellantis", Provider: "adpmyjobs", Board: "stellantisexternalcx",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "5001" {
		t.Errorf("ExternalID = %q", j.ExternalID)
	}
	if j.Title != "Sr. SQE" {
		t.Errorf("Title = %q", j.Title)
	}
	if want := "https://myjobs.adp.com/stellantisexternalcx/cx/job/5001"; j.URL != want {
		t.Errorf("URL = %q, want %q", j.URL, want)
	}
	// The description lives only on the detail call, so a non-empty one proves the hydration
	// actually happened rather than the list being mapped straight through.
	if !strings.Contains(j.Description, "quality") {
		t.Errorf("Description = %q, want the detail body", j.Description)
	}
	if strings.Contains(j.Description, "script") {
		t.Errorf("Description kept a script tag: %q", j.Description)
	}
	if j.PostedAt == nil || j.PostedAt.Year() != 2026 {
		t.Errorf("PostedAt = %v, want the detail's postingDate", j.PostedAt)
	}
}

// The listing is authorised by the orgoid the career site publishes, and by nothing else. A
// crawl that forgot the header would get a 400 that reads exactly like a board that has gone
// away, so the header matters more than the URL does.
func TestADPMyJobsSendsTheOrgoidHeaderOnEveryCallButTheFirst(t *testing.T) {
	fake := (&headerRoutedHTTP{}).
		route("career-site/stellantisexternalcx", adpMyJobsSiteBody).
		route("job-requisitions/5001", adpMyJobsDetailBody("5001", "<p>x</p>")).
		route("job-requisitions?", adpMyJobsListBody(1, "5001"))

	if _, err := NewADPMyJobs(fake).Fetch(context.Background(), CompanyEntry{
		Provider: "adpmyjobs", Board: "stellantisexternalcx",
	}); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if h := fake.headersFor(t, 0); h["orgoid"] != "" {
		t.Errorf("the career-site call carried orgoid %q; it is the call that LEARNS it", h["orgoid"])
	}
	for i := 1; i < len(fake.sent); i++ {
		if got := fake.headersFor(t, i)["orgoid"]; got != "G2QKPTS5DSF914GQ" {
			t.Errorf("call %d carried orgoid %q, want the one the career site published", i, got)
		}
	}
}

// A career site that answers without an orgoid must fail the board rather than list without it:
// an unauthorised listing returns an error that reads like an empty board, which the pipeline
// would take for a board whose postings have all closed.
func TestADPMyJobsFailsWhenTheCareerSitePublishesNoOrgoid(t *testing.T) {
	fake := (&headerRoutedHTTP{}).
		route("career-site/acme", `{"clientName":"Acme"}`).
		route("job-requisitions", adpMyJobsListBody(1, "5001"))

	_, err := NewADPMyJobs(fake).Fetch(context.Background(), CompanyEntry{Provider: "adpmyjobs", Board: "acme"})
	if err == nil {
		t.Fatal("Fetch succeeded with no orgoid; it must refuse")
	}
	if !strings.Contains(err.Error(), "orgoid") {
		t.Errorf("error = %v, want it to name the missing orgoid", err)
	}
	if len(fake.sent) != 1 {
		t.Errorf("made %d calls; it must stop after the career site", len(fake.sent))
	}
}

// The catalogue row's company wins; the career site's is the fallback for a board that carries
// none, and clientName is preferred over the site's own name because the site name reads as a
// board ("Stellantis External CX") rather than an employer.
func TestADPMyJobsFallsBackToTheCareerSiteCompany(t *testing.T) {
	fake := (&headerRoutedHTTP{}).
		route("career-site/stellantisexternalcx", adpMyJobsSiteBody).
		route("job-requisitions/5001", adpMyJobsDetailBody("5001", "<p>x</p>")).
		route("job-requisitions?", adpMyJobsListBody(1, "5001"))

	jobs, err := NewADPMyJobs(fake).Fetch(context.Background(), CompanyEntry{
		Provider: "adpmyjobs", Board: "stellantisexternalcx",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if jobs[0].Company != "Stellantis" {
		t.Errorf("Company = %q, want the career site's clientName", jobs[0].Company)
	}
}

// MyJobs gives the address in pieces, unlike Workforce Now's ready display string.
func TestADPMyJobsComposesLocationFromAddressParts(t *testing.T) {
	fake := (&headerRoutedHTTP{}).
		route("career-site/acme", adpMyJobsSiteBody).
		route("job-requisitions/5001", adpMyJobsDetailBody("5001", "<p>x</p>")).
		route("job-requisitions?", adpMyJobsListBody(1, "5001"))

	jobs, err := NewADPMyJobs(fake).Fetch(context.Background(), CompanyEntry{Provider: "adpmyjobs", Board: "acme"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if want := "Auburn Hills, Michigan, United States"; jobs[0].Location != want {
		t.Errorf("Location = %q, want %q", jobs[0].Location, want)
	}
}

// An employer that publishes no location leaves it empty rather than inventing one — measured on
// prod, jsmcareers publishes none for any posting, and a guess would poison the geo facets.
func TestADPMyJobsLeavesAnAbsentLocationEmpty(t *testing.T) {
	fake := (&headerRoutedHTTP{}).
		route("career-site/acme", adpMyJobsSiteBody).
		route("job-requisitions/5001", `{"count":1,"jobRequisitions":[{"reqId":"5001","jobTitle":"Merchandiser","requisitionLocations":[],"postingLocations":[],"workLocations":[]}]}`).
		route("job-requisitions?", adpMyJobsListBody(1, "5001"))

	jobs, err := NewADPMyJobs(fake).Fetch(context.Background(), CompanyEntry{Provider: "adpmyjobs", Board: "acme"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if jobs[0].Location != "" {
		t.Errorf("Location = %q, want empty", jobs[0].Location)
	}
	if jobs[0].Remote {
		t.Error("Remote = true with no location to read it from")
	}
}

func TestADPMyJobsRejectsAnEmptyBoard(t *testing.T) {
	fake := &headerRoutedHTTP{}
	if _, err := NewADPMyJobs(fake).Fetch(context.Background(), CompanyEntry{Provider: "adpmyjobs"}); err == nil {
		t.Fatal("Fetch accepted an empty board")
	}
	if len(fake.sent) != 0 {
		t.Errorf("made %d calls for an empty board", len(fake.sent))
	}
}

// The adapter must be reachable under its own provider key, or the board catalog's rows address
// nothing and every crawl of them is skipped.
func TestADPMyJobsIsRegistered(t *testing.T) {
	src, ok := Taxonomy()["adpmyjobs"]
	if !ok {
		t.Fatal("adpmyjobs is not in the registry")
	}
	if got := src.Provider(); got != "adpmyjobs" {
		t.Errorf("Provider() = %q, want adpmyjobs", got)
	}
	// It is a distinct product, not a second spelling of adp: a board written for one addresses
	// nothing on the other.
	if _, ok := Taxonomy()["adp"]; !ok {
		t.Error("adp disappeared from the registry")
	}
}
