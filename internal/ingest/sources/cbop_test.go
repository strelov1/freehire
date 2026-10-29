package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// cbopFake serves a catalogue of total offers built from the real rows in testdata/cbop/page.json
// (fetched 2026-10-29: a named employer, a part-time teacher, a pay range with prose, a withheld
// employer, an internship), repeated under fresh ids. refuse lists the offsets of rows the
// service cannot render: any page containing one answers status 500 with a null payload, as
// the real service does.
type cbopFake struct {
	rows   []cbopOffer
	total  int
	refuse map[int]bool
	err    error
	mu     sync.Mutex
	asked  []string
}

func newCBOPFake(t *testing.T, total int) *cbopFake {
	t.Helper()
	raw, err := os.ReadFile("testdata/cbop/page.json")
	if err != nil {
		t.Fatal(err)
	}
	var resp cbopResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatal(err)
	}
	return &cbopFake{rows: resp.Payload.Page.Content, total: total, refuse: map[int]bool{}}
}

func (f *cbopFake) PostJSON(_ context.Context, raw string, _, v any) error {
	f.mu.Lock()
	f.asked = append(f.asked, raw)
	f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	u, _ := url.Parse(raw)
	p, _ := strconv.Atoi(u.Query().Get("page"))
	size, _ := strconv.Atoi(u.Query().Get("size"))
	var content []map[string]any
	for off := p * size; off < (p+1)*size && off < f.total; off++ {
		if f.refuse[off] {
			return json.Unmarshal([]byte(`{"status":500,"msg":"Błąd wewnętrzny systemu!","payload":null}`), v)
		}
		row := f.rows[off%len(f.rows)]
		b, _ := json.Marshal(row)
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		m["id"] = fmt.Sprintf("%s-%d", row.ID, off)
		m["stanowisko"] = row.Title
		m["pracodawca"] = row.Employer
		m["miejscePracy"] = row.Place
		m["zakresObowiazkow"] = row.Duties
		m["wymagania"] = row.Requirements
		m["rodzajUmowy"] = row.Contract
		m["wymiarZatrud"] = row.Hours
		m["wynagrodzenie"] = row.Pay
		m["typOfertyEnum"] = row.Kind
		m["dataDodaniaCbop"] = row.Added
		content = append(content, m)
	}
	b, _ := json.Marshal(map[string]any{"status": 200, "msg": "OK", "payload": map[string]any{
		"ofertyPracyPage": map[string]any{"content": content, "totalElements": f.total},
	}})
	return json.Unmarshal(b, v)
}

func newTestCBOP(f *cbopFake) cbop { return cbop{http: f, limiter: noWait{}} }

func TestCBOPMapsOffers(t *testing.T) {
	f := newCBOPFake(t, 5)
	jobs, err := newTestCBOP(f).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 4 {
		t.Fatalf("got %d jobs, want 4 (the withheld employer dropped)", len(jobs))
	}
	for _, j := range jobs {
		if j.Company == "kontakt przez PUP" {
			t.Error("an offer whose employer is withheld must be dropped")
		}
		if strings.Join(j.FallbackCountries, ",") != "pl" {
			t.Errorf("%s: FallbackCountries = %v", j.ExternalID, j.FallbackCountries)
		}
		if j.SalaryMin != nil {
			t.Errorf("%s: pay is prose, it must not become a structured salary", j.ExternalID)
		}
	}
	geo := jobs[0]
	if geo.Title != "Technik geodeta (K/M)" || geo.Location != "Pawłosiów, podkarpackie" || geo.EmploymentType != "full_time" {
		t.Errorf("first offer = %q %q %q", geo.Title, geo.Location, geo.EmploymentType)
	}
	if !strings.HasPrefix(geo.URL, "https://oferty.praca.gov.pl/portal/lista-ofert/szczegoly-oferty/9847dbbbe36983a61b0f1257aecc62a8") {
		t.Errorf("url = %q", geo.URL)
	}
	if !strings.Contains(geo.Description, "Zakres obowiązków") || !strings.Contains(geo.Description, "Wynagrodzenie brutto: od 4 806 PLN") {
		t.Errorf("description = %q", geo.Description)
	}
	if geo.PostedAt == nil || geo.PostedAt.Format("2006-01-02") != "2026-10-28" {
		t.Errorf("posted = %v", geo.PostedAt)
	}
	if jobs[1].EmploymentType != "part_time" {
		t.Errorf("a 0.56 post is part-time, got %q", jobs[1].EmploymentType)
	}
	if jobs[3].EmploymentType != "internship" {
		t.Errorf("praktyki is an internship, got %q", jobs[3].EmploymentType)
	}
}

func TestCBOPReadsAroundARefusedRow(t *testing.T) {
	f := newCBOPFake(t, 250)
	f.refuse[137] = true
	jobs, err := newTestCBOP(f).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, j := range jobs {
		ids[j.ExternalID] = true
	}
	// 250 rows, one in five withheld (50), and the refused row 137 lost.
	if len(jobs) != 199 {
		t.Fatalf("got %d jobs, want 199", len(jobs))
	}
	if !ids["cfb51882055650f1e2a7587cda9ce6ed-139"] || !ids["d1806d5d4c5f57864798ab3b6dadb6de-136"] {
		t.Error("the rows around the refused one must still be read")
	}
}

func TestCBOPFailsOnAnIncompleteListing(t *testing.T) {
	f := newCBOPFake(t, 300)
	for off := 100; off < 110; off++ {
		f.refuse[off] = true
	}
	if _, err := newTestCBOP(f).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("losing more than 1% of the catalogue must fail the crawl")
	}
	f = newCBOPFake(t, 300)
	f.err = errors.New("boom")
	if _, err := newTestCBOP(f).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("a transport failure must fail the crawl")
	}
}

func TestCBOPShare(t *testing.T) {
	for in, want := range map[string]string{"1": "full_time", "1.5": "full_time", "0.5": "part_time", "3/4": "part_time", "nie dotyczy": "", "": ""} {
		if got := cbopEmploymentType(cbopOffer{Kind: "OFERTA_PRACY", Hours: in}); got != want {
			t.Errorf("%q -> %q, want %q", in, got, want)
		}
	}
}
