package sources

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// Fixture mirrors the real NEOGOV listing fragment (schooljobs/governmentjobs share it):
// li.list-item[data-job-id] cards, each with an item-details-link, a list-meta whose first
// <li> is the location, and a list-entry description snippet.
const neogovListingFixture = `
<ul class="list-items">
  <li class="list-item" data-job-id="5371857">
    <h3 class="job-item-link-container">
      <a class="item-details-link" data-department-name="FT--Student Services"
         href="/careers/cochisecollege/jobs/5371857/ft-academic-career-advisor-svc">FT Academic/Career Advisor SVC</a>
    </h3>
    <ul class="list-meta">
      <li>Sierra Vista Campus, AZ</li>
      <li>Full-time <span>-</span> $50,585.60 Annually</li>
    </ul>
    <div class="list-entry">Position Summary: provide student-centered academic advising.</div>
    <div class="list-published"><span class="list-entry-starts"><span>Posted 3 weeks ago</span></span></div>
  </li>
  <li class="list-item" data-job-id="5379875">
    <h3><a class="item-details-link" href="/careers/cochisecollege/jobs/5379875/ft-accountant">FT Accountant</a></h3>
    <ul class="list-meta"><li>Douglas Campus, AZ</li></ul>
    <div class="list-entry">Maintain the college's financial records.</div>
  </li>
  <li class="list-item">No job id — skipped.</li>
</ul>`

func TestNeogovParseListing(t *testing.T) {
	jobs, err := neogovParseListing(neogovListingFixture, "schooljobs.com", "cochisecollege")
	if err != nil {
		t.Fatalf("neogovParseListing: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2: %+v", len(jobs), jobs)
	}
	j := jobs[0]
	if j.ExternalID != "5371857" {
		t.Errorf("ExternalID = %q, want 5371857", j.ExternalID)
	}
	if j.Title != "FT Academic/Career Advisor SVC" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.URL != "https://www.schooljobs.com/careers/cochisecollege/jobs/5371857/ft-academic-career-advisor-svc" {
		t.Errorf("URL = %q", j.URL)
	}
	if j.Location != "Sierra Vista Campus, AZ" {
		t.Errorf("Location = %q", j.Location)
	}
	if j.Description != "Position Summary: provide student-centered academic advising." {
		t.Errorf("Description = %q", j.Description)
	}
	if jobs[1].ExternalID != "5379875" || jobs[1].Location != "Douglas Campus, AZ" {
		t.Errorf("job1 = %q/%q", jobs[1].ExternalID, jobs[1].Location)
	}
}

// neogovDetailFixture mirrors the real NEOGOV detail page: the full posting body lives in
// <div id="details-info" class="tab-pane active fr-view"> as a <dl>/<dt>/<dd> structure, with
// separate benefit/question tabs the adapter must NOT capture.
const neogovDetailFixture = `<html><body>
  <div class="container entity-details-content tab-content">
    <div id="details-info" class="tab-pane active fr-view">
      <dl>
        <dt><h2>Definition</h2></dt>
        <dd><p>Provide advanced clinical services to youth in secure care.</p></dd>
        <dt><h2>Minimum Qualifications</h2></dt>
        <dd><p>A master's degree in social work.</p></dd>
      </dl>
    </div>
    <div id="details-benefits" class="tab-pane fr-view"><p>Benefits blurb — must be excluded.</p></div>
  </div>
</body></html>`

func TestNeogovDetailDescription(t *testing.T) {
	desc := neogovDetailDescription(neogovDetailFixture)
	if !strings.Contains(desc, "Minimum Qualifications") {
		t.Errorf("description missing full body: %q", desc)
	}
	if !strings.Contains(desc, "advanced clinical services") {
		t.Errorf("description missing definition: %q", desc)
	}
	if strings.Contains(desc, "Benefits blurb") {
		t.Errorf("description leaked the benefits tab: %q", desc)
	}
	// Sanitized HTML structure is preserved, not flattened to text.
	if !strings.Contains(desc, "<h2>") {
		t.Errorf("description not HTML: %q", desc)
	}
}

func TestNeogovDetailDescriptionAbsent(t *testing.T) {
	if desc := neogovDetailDescription(`<html><body><div>no details-info here</div></body></html>`); desc != "" {
		t.Errorf("want empty when container absent, got %q", desc)
	}
}

func TestFirstByID(t *testing.T) {
	root, err := html.Parse(strings.NewReader(`<div><p id="target">hit</p><p id="other">miss</p></div>`))
	if err != nil {
		t.Fatal(err)
	}
	n := firstByID(root, "target")
	if n == nil || textContent(n) != "hit" {
		t.Errorf("firstByID = %v", n)
	}
	if firstByID(root, "missing") != nil {
		t.Errorf("firstByID(missing) should be nil")
	}
}

// neogovFakeHTTP routes by URL: the listing endpoint returns a fragment, each /jobs/<id>/
// detail URL returns its mapped body (or an error), and records whether the XHR header was
// sent so the test can assert the detail fetch is a plain GET.
type neogovFakeHTTP struct {
	listing    string
	details    map[string]string
	detailErr  map[string]bool
	detailXHR  map[string]bool // id -> whether X-Requested-With was sent on its detail call
	listingXHR bool
}

var neogovJobIDRe = regexp.MustCompile(`/jobs/(\d+)/`)

func (f *neogovFakeHTTP) GetTextWithHeaders(_ context.Context, url string, headers map[string]string) (string, error) {
	_, xhr := headers["X-Requested-With"]
	if strings.Contains(url, "careers/home/index") {
		f.listingXHR = xhr
		return f.listing, nil
	}
	m := neogovJobIDRe.FindStringSubmatch(url)
	id := ""
	if m != nil {
		id = m[1]
	}
	if f.detailXHR == nil {
		f.detailXHR = map[string]bool{}
	}
	f.detailXHR[id] = xhr
	if f.detailErr[id] {
		return "", errors.New("neogovFakeHTTP: detail boom")
	}
	return f.details[id], nil
}

func TestNeogovFetchStoresDetailBody(t *testing.T) {
	// A count span forces the walk to stop after one listing page.
	listing := `<span id="job-postings-number">2</span>` + neogovListingFixture
	f := &neogovFakeHTTP{
		listing: listing,
		details: map[string]string{"5371857": neogovDetailFixture},
		// 5379875 has no detail body → its fetch errors, so the snippet must survive.
		detailErr: map[string]bool{"5379875": true},
	}
	jobs, err := neogov{http: f}.Fetch(context.Background(), CompanyEntry{Company: "Cochise College", Board: "schooljobs.com/cochisecollege"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2", len(jobs))
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	if got := byID["5371857"].Description; !strings.Contains(got, "Minimum Qualifications") {
		t.Errorf("job 5371857 description = %q, want full detail body", got)
	}
	if got := byID["5379875"].Description; got != "Maintain the college's financial records." {
		t.Errorf("job 5379875 description = %q, want listing snippet fallback", got)
	}
	// The detail fetch must be a plain GET (no XHR header); the listing must carry it.
	if !f.listingXHR {
		t.Errorf("listing request missing X-Requested-With header")
	}
	if f.detailXHR["5371857"] {
		t.Errorf("detail request must NOT carry X-Requested-With header")
	}
}

func TestNeogovTotal(t *testing.T) {
	if n := neogovTotal(`<span id="job-postings-number">20</span>`); n != 20 {
		t.Errorf("neogovTotal = %d, want 20", n)
	}
	if n := neogovTotal(`<div>no count here</div>`); n != 0 {
		t.Errorf("neogovTotal(absent) = %d, want 0", n)
	}
}
