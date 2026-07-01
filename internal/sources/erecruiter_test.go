package sources

import (
	"context"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// erecruiterFake routes GetText (JSONP list, matched by "pn=<n>") and GetHTML (per-offer
// detail, matched by "oid=<id>") to canned bodies, so the adapter is exercised without the
// network. An unrouted list page returns an empty JSONP body (past-the-end); an unrouted
// detail returns bare HTML (no #JobTitle → skipped).
type erecruiterFake struct {
	list        map[string]string
	detail      map[string]string
	listCalls   int
	detailCalls int
}

func (f *erecruiterFake) GetText(_ context.Context, url string) (string, error) {
	f.listCalls++
	for k, v := range f.list {
		if strings.Contains(url, k) {
			return v, nil
		}
	}
	return `({"htm":""})`, nil
}

func (f *erecruiterFake) GetHTML(_ context.Context, url string) (*html.Node, error) {
	f.detailCalls++
	for k, v := range f.detail {
		if strings.Contains(url, k) {
			return html.Parse(strings.NewReader(v))
		}
	}
	return html.Parse(strings.NewReader("<html><body></body></html>"))
}

// jsonp wraps an inner htm fragment in the ({"htm":"…"}) envelope the real endpoint returns,
// escaping the fragment as a JSON string.
func jsonp(htm string) string {
	return `({"htm":` + jsonString(htm) + `})`
}

// jsonString renders s as a JSON string literal (quotes + escapes), so an HTML fragment with
// quotes/backslashes embeds safely in the JSONP body.
func jsonString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// A detail page mirroring eRecruiter's Offer.aspx: #JobTitle / #WorkPlace header, #t1 body
// (with a <script> to prove sanitization), #CompanyDescription, and the Aplikuj button.
func erecruiterDetail(title, city string) string {
	return `<html><body>
<div class="job1"><div class="job">
  <div id="JobTitle">` + title + `</div>
  <div id="WorkPlace">Miejsce pracy: ` + city + `</div>
</div></div>
<div id="t1"><p><strong>Twój zakres obowiązków</strong></p><ul><li>Prowadzenie połączeń</li></ul><script>evil()</script></div>
<div id="CompanyDescription"><p>Od 40 lat realizujemy inwestycje.</p></div>
<div class="job1"><a href="https://system.erecruiter.pl/FormTemplates/RecruitmentForm.aspx?webid=B3107797-D274-4CA4-A08C-028E3A7AE1DE" class="button">Aplikuj</a></div>
</body></html>`
}

func TestExtractErecruiterCfg(t *testing.T) {
	withWidget := `<html><body><h1>Kariera</h1>
<script type='text/javascript' src='https://skk.erecruiter.pl/Code.ashx?cfg=4D3DD27FCA3D40C79E01BEE9745902B7'></script>
</body></html>`
	if got := ExtractErecruiterCfg(withWidget); got != "4D3DD27FCA3D40C79E01BEE9745902B7" {
		t.Errorf("ExtractErecruiterCfg = %q, want the 32-hex cfg", got)
	}

	if got := ExtractErecruiterCfg(`<html><body>No eRecruiter widget here.</body></html>`); got != "" {
		t.Errorf("ExtractErecruiterCfg = %q, want empty for a page without the widget", got)
	}
}

func TestErecruiterProvider(t *testing.T) {
	if got := NewErecruiter(nil).Provider(); got != "erecruiter" {
		t.Errorf("Provider() = %q, want %q", got, "erecruiter")
	}
}

func TestErecruiterFetchMapsFieldsAndStopsAtTotal(t *testing.T) {
	// Three offer rows, two of which share externalJobOfferId=570550 across cities (Łódź,
	// Kraków) with distinct offerId — the real board's multi-city shape. The marker row
	// reports tr=3 (total), so the walk stops after page 1 (no second list call).
	rows := `<tr jobOfferId='3479394' offerId='4891462' externalJobOfferId='574507' externalJobOfferRegionId='309263' comId='20002054' skkResult='offer'><td class='skk_positionName'>Staż w Dziale Obsługi Klienta</td><td>Wrocław</td></tr>` +
		`<tr jobOfferId='3471220' offerId='4882522' externalJobOfferId='570550' externalJobOfferRegionId='308328' comId='20002054' skkResult='offer'><td class='skk_positionName'>Specjalista ds. Wsparcia</td><td>Łódź</td></tr>` +
		`<tr jobOfferId='3471220' offerId='4882523' externalJobOfferId='570550' externalJobOfferRegionId='308329' comId='20002054' skkResult='offer'><td class='skk_positionName'>Specjalista ds. Wsparcia</td><td>Kraków</td></tr>` +
		`<tr tp='2' tr='3' pn='1' ps='15' style='display:none;'></tr>`

	fake := &erecruiterFake{
		list: map[string]string{"pn=1": jsonp(rows)},
		detail: map[string]string{
			"oid=4891462": erecruiterDetail("Staż w Dziale Obsługi Klienta", "Wrocław"),
			"oid=4882522": erecruiterDetail("Specjalista ds. Wsparcia", "Łódź"),
			"oid=4882523": erecruiterDetail("Specjalista ds. Wsparcia", "Kraków"),
		},
	}

	jobs, err := NewErecruiter(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Echo Investment", Provider: "erecruiter", Board: "4D3DD27FCA3D40C79E01BEE9745902B7",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if len(jobs) != 3 {
		t.Fatalf("len(jobs) = %d, want 3 (multi-city variants kept as distinct offerId postings)", len(jobs))
	}
	if fake.listCalls != 1 {
		t.Fatalf("listCalls = %d, want 1 (total reached after page 1)", fake.listCalls)
	}

	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	// The two multi-city variants must NOT collapse: distinct offerId → distinct ExternalID.
	if _, ok := byID["4882522"]; !ok {
		t.Error("offerId 4882522 (Łódź) missing — multi-city variant collapsed?")
	}
	if _, ok := byID["4882523"]; !ok {
		t.Error("offerId 4882523 (Kraków) missing — multi-city variant collapsed?")
	}

	j := byID["4891462"]
	if j.Title != "Staż w Dziale Obsługi Klienta" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "Echo Investment" {
		t.Errorf("Company = %q, want config company", j.Company)
	}
	if j.Location != "Wrocław" {
		t.Errorf("Location = %q, want %q (Miejsce pracy: prefix stripped)", j.Location, "Wrocław")
	}
	if !strings.Contains(j.Description, "Twój zakres") {
		t.Errorf("Description missing body: %q", j.Description)
	}
	if strings.Contains(j.Description, "<script") || strings.Contains(j.Description, "evil()") {
		t.Errorf("Description not sanitized: %q", j.Description)
	}
	if !strings.Contains(j.URL, "Offer.aspx") || !strings.Contains(j.URL, "oid=4891462") {
		t.Errorf("URL = %q, want the Offer.aspx detail URL with oid", j.URL)
	}
}

func TestErecruiterFetchSkipsUnparseableDetail(t *testing.T) {
	// A row whose detail lacks #JobTitle (dead/closed offer) is skipped without aborting the
	// board; the healthy row still comes through.
	rows := `<tr offerId='111' externalJobOfferId='11' externalJobOfferRegionId='1' comId='9' skkResult='offer'><td class='skk_positionName'>Good Role</td><td>Warszawa</td></tr>` +
		`<tr offerId='222' externalJobOfferId='22' externalJobOfferRegionId='2' comId='9' skkResult='offer'><td class='skk_positionName'>Dead Role</td><td>Kraków</td></tr>` +
		`<tr tp='2' tr='2' pn='1' ps='15'></tr>`

	fake := &erecruiterFake{
		list: map[string]string{"pn=1": jsonp(rows)},
		detail: map[string]string{
			"oid=111": erecruiterDetail("Good Role", "Warszawa"),
			"oid=222": `<html><body><div>Oferta wygasła</div></body></html>`,
		},
	}

	jobs, err := NewErecruiter(fake).Fetch(context.Background(), CompanyEntry{Company: "Acme", Board: "cfg"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "111" {
		t.Fatalf("jobs = %+v, want only the healthy offer 111", jobs)
	}
}

func TestErecruiterFetchStopsOnEmptyPage(t *testing.T) {
	// The marker over-reports the total (tr=999) but the board runs out on page 2. An empty
	// page must end the walk instead of spinning to the page cap.
	page1 := `<tr offerId='1' externalJobOfferId='1' externalJobOfferRegionId='1' comId='9' skkResult='offer'><td class='skk_positionName'>Role</td><td>Gdańsk</td></tr>` +
		`<tr tp='2' tr='999' pn='1' ps='1'></tr>`

	fake := &erecruiterFake{
		list:   map[string]string{"pn=1": jsonp(page1)}, // pn=2 falls through to the empty body
		detail: map[string]string{"oid=1": erecruiterDetail("Role", "Gdańsk")},
	}

	jobs, err := NewErecruiter(fake).Fetch(context.Background(), CompanyEntry{Company: "Acme", Board: "cfg"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1 (empty second page ends the walk)", len(jobs))
	}
	if fake.listCalls != 2 {
		t.Fatalf("listCalls = %d, want 2 (page 1 + the empty page 2)", fake.listCalls)
	}
}
