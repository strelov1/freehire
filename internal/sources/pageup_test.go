package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// fakePageup serves the search envelope from search and each job's detail envelope from
// detail (keyed by the job's full URL), recording every URL fetched and the XHR header
// sent — both the listing and the detail endpoint require it, or the tenant 302s to the
// full HTML page instead of the JSON envelope.
type fakePageup struct {
	search string
	detail map[string]string
	calls  []string
}

func (f *fakePageup) GetJSONWithHeaders(_ context.Context, url string, headers map[string]string, v any) error {
	f.calls = append(f.calls, url)
	if headers["X-Requested-With"] != "XMLHttpRequest" {
		return errors.New("fakePageup: missing XHR header")
	}
	if strings.Contains(url, "/search/") {
		return json.Unmarshal([]byte(f.search), v)
	}
	raw, ok := f.detail[url]
	if !ok {
		return fmt.Errorf("fakePageup: no canned detail for %s", url)
	}
	return json.Unmarshal([]byte(raw), v)
}

// A tenant's fragment is homogeneous — Monash renders a <tr> table, Melbourne Water a <li>
// list. Each is tested on its own valid fragment (mixing the two in one document would
// trigger the HTML parser's table foster-parenting and reorder nodes, which never happens in
// a real single-tenant response). Both must yield the same fields off the stable a.job-link.
// The real endpoint returns bare <tr> rows with NO <table> wrapper (the page's JS injects
// them into an existing table), so the parser must supply the table context itself — parsing
// bare rows directly triggers foster-parenting that drops the row structure.
const pageupTableFixture = `
  <tr>
    <td><a class="job-link" href="/513/cw/en/job/694504/senior-audiovisual-design-engineer">Senior Audiovisual Design Engineer</a></td>
    <td><span class="location">Clayton campus</span></td>
    <td>HEW 8</td>
    <td><span class="close-date"><time datetime="2026-08-02T13:55:00Z">2 Aug 2026</time></span></td>
  </tr>
  <tr class="summary"><td colspan="4">Support the University's audiovisual design team.</td></tr>
  <tr><td>No anchor here — ignored.</td></tr>`

const pageupListFixture = `
<ul>
  <li class="jobs-item">
    <a class="job-link" href="/391/cw/en/job/980277/education-and-events-lead">Education and Events Lead</a>
    <span class="jobs-info">
      <span class="label">Work type:</span> <span class="work-type permanent-part-time">Permanent Part Time</span>
      <span class="label">Location:</span> <span class="location">Melbourne - Docklands</span>
    </span>
    <p class="jobs-summary">Lead education and events programs.</p>
  </li>
</ul>`

// A third tenant shape (e.g. board 709): the summary is a bare, unclassed <p> — no
// "summary"/"jobs-summary" class anywhere — sitting right after the title, with the
// location tucked inside a classed <li> further down.
const pageupCardFixture = `
<div class="job-listing">
  <div class="job-listing-left">
    <h3 class="job-listing-title"><a class="job-link" href="/709/cw/en/job/501297/senior-quantity-surveyor-indonesia">Senior Quantity Surveyor, Indonesia</a></h3>
    <p>Currently, Leighton Contractors Indonesia is looking for a Quantity Surveyor.</p>
  </div>
  <ul class="job-listing-meta">
    <li><span class="categories">Commercial / Project Finance</span></li>
    <li><span class="location">Indonesia</span></li>
  </ul>
</div>`

func TestPageupParseListing(t *testing.T) {
	t.Run("table layout", func(t *testing.T) {
		jobs, err := pageupParseListing(pageupTableFixture, "513")
		if err != nil {
			t.Fatalf("pageupParseListing: %v", err)
		}
		if len(jobs) != 1 {
			t.Fatalf("got %d jobs, want 1: %+v", len(jobs), jobs)
		}
		j := jobs[0]
		if j.ExternalID != "694504" || j.Title != "Senior Audiovisual Design Engineer" {
			t.Errorf("id/title = %q/%q", j.ExternalID, j.Title)
		}
		if j.URL != "https://careers.pageuppeople.com/513/cw/en/job/694504/senior-audiovisual-design-engineer" {
			t.Errorf("URL = %q", j.URL)
		}
		if j.Location != "Clayton campus" {
			t.Errorf("Location = %q", j.Location)
		}
		if j.Description != "Support the University's audiovisual design team." {
			t.Errorf("Description = %q", j.Description)
		}
	})

	t.Run("list layout", func(t *testing.T) {
		jobs, err := pageupParseListing(pageupListFixture, "391")
		if err != nil {
			t.Fatalf("pageupParseListing: %v", err)
		}
		if len(jobs) != 1 {
			t.Fatalf("got %d jobs, want 1: %+v", len(jobs), jobs)
		}
		j := jobs[0]
		if j.ExternalID != "980277" || j.Location != "Melbourne - Docklands" {
			t.Errorf("id/loc = %q/%q", j.ExternalID, j.Location)
		}
		if j.Description != "Lead education and events programs." {
			t.Errorf("Description = %q", j.Description)
		}
	})

	t.Run("card layout with unclassed summary paragraph", func(t *testing.T) {
		jobs, err := pageupParseListing(pageupCardFixture, "709")
		if err != nil {
			t.Fatalf("pageupParseListing: %v", err)
		}
		if len(jobs) != 1 {
			t.Fatalf("got %d jobs, want 1: %+v", len(jobs), jobs)
		}
		j := jobs[0]
		if j.ExternalID != "501297" || j.Location != "Indonesia" {
			t.Errorf("id/loc = %q/%q", j.ExternalID, j.Location)
		}
		if j.Description != "Currently, Leighton Contractors Indonesia is looking for a Quantity Surveyor." {
			t.Errorf("Description = %q", j.Description)
		}
	})
}

// pageupNoSummaryFixture mirrors board 798: the listing row carries nothing but the link
// and a department name — no summary of any shape — so the description can only come
// from a detail-page fetch.
const pageupNoSummaryFixture = `
  <tr>
    <td><a class="job-link" href="/798/cw/en/job/496049/senior-application-development-manager">Senior Application Development Manager</a></td>
    <td>Information Technology Department</td>
  </tr>`

func TestPageupFetchFallsBackToDetailPageWhenListingHasNoSummary(t *testing.T) {
	jobURL := "https://careers.pageuppeople.com/798/cw/en/job/496049/senior-application-development-manager"
	fake := &fakePageup{
		search: fmt.Sprintf(`{"results":%q}`, pageupNoSummaryFixture),
		detail: map[string]string{
			jobURL: `{"results":"<h2>Senior Application Development Manager</h2><div id=\"job-details\"><p>Own the delivery of trading systems.</p></div>"}`,
		},
	}

	jobs, err := NewPageUp(fake).Fetch(context.Background(), CompanyEntry{Company: "Bank", Provider: "pageup", Board: "798"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1: %+v", len(jobs), jobs)
	}
	if got := jobs[0].Description; got != "<p>Own the delivery of trading systems.</p>" {
		t.Errorf("Description = %q", got)
	}
	if len(fake.calls) != 2 || fake.calls[1] != jobURL {
		t.Errorf("calls = %v, want the search URL then exactly %q", fake.calls, jobURL)
	}
}

func TestPageupFetchSkipsDetailFetchWhenListingAlreadyHasSummary(t *testing.T) {
	fake := &fakePageup{
		search: fmt.Sprintf(`{"results":%q}`, pageupTableFixture),
	}

	jobs, err := NewPageUp(fake).Fetch(context.Background(), CompanyEntry{Company: "Monash", Provider: "pageup", Board: "513"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].Description != "Support the University's audiovisual design team." {
		t.Fatalf("jobs = %+v", jobs)
	}
	if len(fake.calls) != 1 {
		t.Errorf("calls = %v, want only the search request — no detail fetch when the listing already has a summary", fake.calls)
	}
}

func TestPageupFetchAlwaysFetchesDetailOnTeaserSummaryBoard(t *testing.T) {
	jobURL := "https://careers.pageuppeople.com/889/cw/en/job/532245/sales-travel-consultant"
	fixture := `
  <tr>
    <td><a class="job-link" href="/889/cw/en/job/532245/sales-travel-consultant">Sales Travel Consultant</a></td>
    <td><span class="location">South Australia</span></td>
  </tr>
  <tr class="summary"><td colspan="2">Start your travel career with Flight Centre!</td></tr>`
	fake := &fakePageup{
		search: fmt.Sprintf(`{"results":%q}`, fixture),
		detail: map[string]string{
			jobURL: `{"results":"<div id=\"job-details\"><p>The full posting body, much longer than the teaser.</p></div>"}`,
		},
	}

	jobs, err := NewPageUp(fake).Fetch(context.Background(), CompanyEntry{Company: "Flight Centre", Provider: "pageup", Board: "889"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1: %+v", len(jobs), jobs)
	}
	if got := jobs[0].Description; got != "<p>The full posting body, much longer than the teaser.</p>" {
		t.Errorf("Description = %q, want the detail page body, not the listing teaser", got)
	}
	if len(fake.calls) != 2 || fake.calls[1] != jobURL {
		t.Errorf("calls = %v, want the search URL then exactly %q", fake.calls, jobURL)
	}
}

func TestPageupFetchDetailFailureLeavesDescriptionEmptyRatherThanDroppingTheJob(t *testing.T) {
	fake := &fakePageup{
		search: fmt.Sprintf(`{"results":%q}`, pageupNoSummaryFixture),
		detail: map[string]string{}, // no canned detail → fakePageup errors on the fetch
	}

	jobs, err := NewPageUp(fake).Fetch(context.Background(), CompanyEntry{Company: "Bank", Provider: "pageup", Board: "798"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (a failed detail fetch must not drop the posting): %+v", len(jobs), jobs)
	}
	if jobs[0].Description != "" {
		t.Errorf("Description = %q, want empty", jobs[0].Description)
	}
}
