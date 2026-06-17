package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// phenomFake is a JSONPoster that routes by the request body's ddoKey: a refineSearch
// returns the canned page for its "from" offset, a jobDetail returns the canned detail
// for its jobSeqNo (or errors when absent, so the adapter skips that posting).
type phenomFake struct {
	pages   map[int]string
	details map[string]string
}

func (f phenomFake) PostJSON(_ context.Context, _ string, body, v any) error {
	m := body.(map[string]any)
	switch m["ddoKey"] {
	case "refineSearch":
		return json.Unmarshal([]byte(f.pages[m["from"].(int)]), v)
	case "jobDetail":
		js, ok := f.details[m["jobSeqNo"].(string)]
		if !ok {
			return fmt.Errorf("no detail for %v", m["jobSeqNo"])
		}
		return json.Unmarshal([]byte(js), v)
	}
	return fmt.Errorf("unexpected ddoKey %v", m["ddoKey"])
}

// phenomListPage renders a refineSearch page from a list of jobSeqNos.
func phenomListPage(seqs ...string) string {
	jobs := make([]string, len(seqs))
	for i, s := range seqs {
		jobs[i] = fmt.Sprintf(`{"jobSeqNo":%q,"title":"Engineer %s","cityState":"Bonn, NRW","locale":"en_GLOBAL","postedDate":"2026-05-17T22:00:00.000+0000"}`, s, s)
	}
	return `{"refineSearch":{"data":{"jobs":[` + strings.Join(jobs, ",") + `]}}}`
}

func phenomDetail(desc string) string {
	return fmt.Sprintf(`{"jobDetail":{"data":{"job":{"description":%q,"title":"Detail Title"}}}}`, desc)
}

func TestPhenomProvider(t *testing.T) {
	if got := NewPhenom(nil).Provider(); got != "phenom" {
		t.Errorf("Provider() = %q, want %q", got, "phenom")
	}
}

func TestPhenomFetchPaginatesAndFetchesDetail(t *testing.T) {
	// A full first page (phenomPageSize seqs) forces a second request; the short second
	// page stops the loop.
	first := make([]string, phenomPageSize)
	for i := range first {
		first[i] = fmt.Sprintf("S%d", i)
	}
	fake := phenomFake{
		pages: map[int]string{
			0:              phenomListPage(first...),
			phenomPageSize: phenomListPage("LAST", "NODETAIL"),
		},
		details: map[string]string{},
	}
	for _, s := range first {
		fake.details[s] = phenomDetail("<p>full body for " + s + "</p>")
	}
	fake.details["LAST"] = phenomDetail("<p>last one</p>")
	// "NODETAIL" has no detail route -> its detail fetch errors and it is skipped.

	jobs, err := NewPhenom(fake).Fetch(context.Background(), CompanyEntry{
		Company: "DHL", Provider: "phenom", Board: "careers.dhl.com",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if want := phenomPageSize + 1; len(jobs) != want {
		t.Fatalf("len(jobs) = %d, want %d (full page + LAST, NODETAIL skipped)", len(jobs), want)
	}

	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	if _, present := byID["NODETAIL"]; present {
		t.Error("NODETAIL should have been skipped (no detail)")
	}

	j, ok := byID["S0"]
	if !ok {
		t.Fatal("posting S0 missing")
	}
	if j.Title != "Engineer S0" {
		t.Errorf("Title = %q, want the list title", j.Title)
	}
	if j.URL != "https://careers.dhl.com/global/en/job/S0" {
		t.Errorf("URL = %q, want locale-derived public page", j.URL)
	}
	if j.Location != "Bonn, NRW" {
		t.Errorf("Location = %q, want cityState", j.Location)
	}
	if !strings.Contains(j.Description, "full body for S0") {
		t.Errorf("Description = %q, want the detail body", j.Description)
	}
	if j.PostedAt == nil || j.PostedAt.UTC().Year() != 2026 {
		t.Errorf("PostedAt = %v, want parsed postedDate (2026)", j.PostedAt)
	}
}

func TestPhenomJobURL(t *testing.T) {
	cases := []struct{ board, locale, seq, want string }{
		{"careers.dhl.com", "en_GLOBAL", "X1", "https://careers.dhl.com/global/en/job/X1"},
		{"careers.dhl.com", "en_us", "X2", "https://careers.dhl.com/us/en/job/X2"},
		{"careers.dhl.com", "", "X3", "https://careers.dhl.com/job/X3"},
	}
	for _, c := range cases {
		if got := phenomJobURL(c.board, c.locale, c.seq); got != c.want {
			t.Errorf("phenomJobURL(%q,%q,%q) = %q, want %q", c.board, c.locale, c.seq, got, c.want)
		}
	}
}
