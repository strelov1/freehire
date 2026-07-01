package sources

import (
	"context"
	"strings"
	"testing"
)

func TestErecruiterProvider(t *testing.T) {
	if got := NewErecruiter(nil).Provider(); got != "erecruiter" {
		t.Errorf("Provider() = %q, want %q", got, "erecruiter")
	}
}

// erecruiterRow builds one JSONP list row matching the real skk markup.
func erecruiterRow(offerID, ejoID, ejorID, title, city string) string {
	return "<tr jobOfferId='9' offerId='" + offerID + "' class='skk_row_odd' externalJobOfferId='" +
		ejoID + "' externalJobOfferRegionId='" + ejorID + "' comId='20022820' skkResult='offer' >" +
		"<td class='skk_positionName'>" + title + "</td><td>IT</td><td>" + city + "</td></tr>"
}

// erecruiterPage wraps rows in the JSONP envelope plus the hidden marker row carrying tr=total.
func erecruiterPage(total string, rows ...string) string {
	marker := "<tr tp='2' tr='" + total + "' pn='1' ps='20' style='display:none;'></tr>"
	return `({"htm":"` + strings.Join(rows, "") + marker + `"})`
}

func erecruiterDetail(title, body string) string {
	return `<html><body><div id="offCont"><div id="JobTitle">` + title +
		`</div><div id="WorkPlace">Miejsce pracy: Warszawa</div>` +
		`<div id="t1">` + body + `</div></body></html>`
}

func TestErecruiterFetchMapsFieldsAndPaginates(t *testing.T) {
	// Total is 3 across two pages: page 1 has two offers, page 2 has one. The walk stops once
	// it has collected the reported total, so no third page is fetched. The offer whose detail
	// page has no route (GetHTML errors) is dropped, leaving 2 jobs.
	page1 := erecruiterPage("3",
		erecruiterRow("4893718", "587372", "330822", "Senior Go Engineer", "Warszawa"),
		erecruiterRow("4893487", "591483", "330807", "Dropped Role", "Kraków"),
	)
	page2 := erecruiterPage("3",
		erecruiterRow("4899999", "599999", "330900", "Backend Developer", "Gdańsk"),
	)

	fake := (&routedHTTP{}).
		route("grid=rows&pn=1", page1).
		route("grid=rows&pn=2", page2).
		// Detail pages, keyed by offer id (oid=). "Dropped Role" (oid=4893487) has no route.
		route("oid=4893718", erecruiterDetail("Senior Go Engineer", "<p>Build things</p><script>evil()</script>")).
		route("oid=4899999", erecruiterDetail("Backend Developer", "<p>Write services</p>"))

	jobs, err := NewErecruiter(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "erecruiter", Board: "eac02250aa184ed4bf4950e74948549a",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2 (one posting dropped for a missing detail page)", len(jobs))
	}

	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	full, ok := byID["587372"]
	if !ok {
		t.Fatal("job 587372 missing — ExternalID must be the externalJobOfferId")
	}
	if full.Title != "Senior Go Engineer" {
		t.Errorf("Title = %q, want %q", full.Title, "Senior Go Engineer")
	}
	if full.Company != "Acme" {
		t.Errorf("Company = %q, want config company %q", full.Company, "Acme")
	}
	if full.Location != "Warszawa" {
		t.Errorf("Location = %q, want the row city %q", full.Location, "Warszawa")
	}
	if !strings.Contains(full.Description, "Build things") {
		t.Errorf("Description = %q, want the detail body", full.Description)
	}
	if strings.Contains(full.Description, "<script") {
		t.Errorf("Description not sanitized: %q", full.Description)
	}
	if !strings.Contains(full.URL, "Offer.aspx") ||
		!strings.Contains(full.URL, "oid=4893718") ||
		!strings.Contains(full.URL, "cfg=eac02250aa184ed4bf4950e74948549a") {
		t.Errorf("URL = %q, want the Offer.aspx detail URL with oid+cfg", full.URL)
	}

	if _, dropped := byID["591483"]; dropped {
		t.Error("posting 591483 has no detail page and must be dropped")
	}

	if _, ok := byID["599999"]; !ok {
		t.Error("job 599999 from page 2 missing — pagination must fetch the second page")
	}
}

func TestErecruiterFetchStopsAtReportedTotal(t *testing.T) {
	// Page 1 already carries the whole board (total=1), so the walk must not fetch page 2.
	page1 := erecruiterPage("1",
		erecruiterRow("100", "200", "300", "Only Role", "Wrocław"),
	)
	fake := (&routedHTTP{}).
		route("grid=rows&pn=1", page1).
		route("oid=100", erecruiterDetail("Only Role", "<p>Sole opening</p>"))

	jobs, err := NewErecruiter(fake).Fetch(context.Background(), CompanyEntry{Company: "Acme", Board: "brd"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1", len(jobs))
	}
	// One list call (pn=1) + one detail call (oid=100) = 2. A pn=2 fetch would have no route
	// and error, so reaching here proves the walk stopped at the reported total.
	if fake.calls != 2 {
		t.Fatalf("fake.calls = %d, want 2 (no second list page once the total is reached)", fake.calls)
	}
}

func TestErecruiterFetchStopsOnNonAdvancingPage(t *testing.T) {
	// The board over-reports the total (99) but ignores pn, serving the same first row on every
	// page. The walk must stop once a page repeats the previous page's first row instead of
	// re-collecting up to the page cap.
	same := erecruiterPage("99", erecruiterRow("100", "200", "300", "Only Role", "Wrocław"))
	fake := (&routedHTTP{}).
		route("grid=rows", same). // every pn returns the same page
		route("oid=100", erecruiterDetail("Only Role", "<p>Sole opening</p>"))

	jobs, err := NewErecruiter(fake).Fetch(context.Background(), CompanyEntry{Company: "Acme", Board: "brd"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1 (non-advancing pages must not re-collect)", len(jobs))
	}
}
