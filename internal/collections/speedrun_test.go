package collections

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A trimmed copy of a real /api/v1/companies response: one company per tier, the
// pagination envelope, and the fields the parser reads.
const speedrunPageFixture = `{
  "companies": [
    {"slug": "anduril-industries", "name": "Anduril Industries", "tier": "a16z", "cohort": null, "open_roles": 2134},
    {"slug": "aghanim", "name": "Aghanim", "tier": "speedrun", "cohort": "SR002", "open_roles": 25},
    {"slug": "walmart", "name": "Walmart", "tier": "market", "cohort": null, "open_roles": 217}
  ],
  "total": 876,
  "page": 0,
  "page_size": 100,
  "total_pages": 9,
  "source": "freehire"
}`

func TestParseSpeedrunPage_ReadsNamesTiersAndPageCount(t *testing.T) {
	page, err := parseSpeedrunPage([]byte(speedrunPageFixture))
	if err != nil {
		t.Fatalf("parseSpeedrunPage: %v", err)
	}
	if page.TotalPages != 9 {
		t.Errorf("TotalPages = %d, want 9", page.TotalPages)
	}
	if len(page.Records) != 3 {
		t.Fatalf("got %d records, want 3: %+v", len(page.Records), page.Records)
	}
	got := page.Records[1]
	if got.Name != "Aghanim" {
		t.Errorf("Name = %q, want Aghanim", got.Name)
	}
	if got.Meta["tier"] != "speedrun" {
		t.Errorf("tier = %q, want speedrun", got.Meta["tier"])
	}
}

func TestParseSpeedrunPage_DropsAnEntryMissingANameOrTier(t *testing.T) {
	// A nameless record can never match a company, and a tierless one cannot be
	// assigned to either a16z tag — keeping them would only pad the counts a dry run
	// is read for.
	const partial = `{"companies": [
	  {"slug": "x", "name": "", "tier": "a16z"},
	  {"slug": "y", "name": "Real Co", "tier": ""},
	  {"slug": "z", "name": "Keeper", "tier": "a16z"}
	], "total_pages": 1}`

	page, err := parseSpeedrunPage([]byte(partial))
	if err != nil {
		t.Fatalf("parseSpeedrunPage: %v", err)
	}
	if len(page.Records) != 1 || page.Records[0].Name != "Keeper" {
		t.Errorf("records = %+v, want only Keeper", page.Records)
	}
}

func TestParseSpeedrunPage_RejectsGarbage(t *testing.T) {
	if _, err := parseSpeedrunPage([]byte("<html>nope</html>")); err == nil {
		t.Error("garbage payload parsed without error")
	}
}

// directoryPage renders a page whose companies are named after their index, so a
// test can tell which pages were actually read.
func directoryPage(page, totalPages int) string {
	return fmt.Sprintf(
		`{"companies":[{"name":"Company %d","tier":"a16z"}],"total_pages":%d,"page":%d}`,
		page, totalPages, page)
}

func TestFetchSpeedrunDirectory_ReadsEveryPage(t *testing.T) {
	const totalPages = 3
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		if page == "" {
			page = "0"
		}
		var n int
		_, _ = fmt.Sscanf(page, "%d", &n)
		_, _ = fmt.Fprint(w, directoryPage(n, totalPages))
	}))
	defer srv.Close()

	got, err := fetchSpeedrunDirectory(context.Background(), srv.Client(), srv.URL)
	if err != nil {
		t.Fatalf("fetchSpeedrunDirectory: %v", err)
	}
	if len(got) != totalPages {
		t.Fatalf("got %d records, want one per page (%d): %+v", len(got), totalPages, got)
	}
	for i, r := range got {
		if want := fmt.Sprintf("Company %d", i); r.Name != want {
			t.Errorf("record %d = %q, want %q — pages read out of order or skipped", i, r.Name, want)
		}
	}
}

func TestSpeedrunMembers_AMarketTierCompanyEarnsNoA16zTag(t *testing.T) {
	// The directory's largest tier is the general market — Walmart, TikTok, Amazon,
	// P&G. They have no a16z relationship whatsoever. Importing that tier would put a
	// fund's badge on a supermarket chain: a false claim about a real company, on a
	// public page. This test is the guard.
	const mixed = `{"companies":[
	  {"name":"Walmart","tier":"market"},
	  {"name":"TikTok","tier":"market"},
	  {"name":"Anduril Industries","tier":"a16z"},
	  {"name":"Aghanim","tier":"speedrun","cohort":"SR002"}
	],"total_pages":1}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, mixed)
	}))
	defer srv.Close()

	for tier, want := range map[string]string{"a16z": "Anduril Industries", "speedrun": "Aghanim"} {
		got, err := speedrunMembersFrom(context.Background(), srv.Client(), srv.URL, tier)
		if err != nil {
			t.Fatalf("tier %s: %v", tier, err)
		}
		if len(got) != 1 || got[0].Name != want {
			t.Fatalf("tier %s = %+v, want only %q", tier, got, want)
		}
		for _, r := range got {
			if r.Name == "Walmart" || r.Name == "TikTok" {
				t.Errorf("tier %s admitted the market-tier company %q", tier, r.Name)
			}
		}
	}
}

func TestFetchSpeedrunDirectory_NamesItselfInTheUserAgent(t *testing.T) {
	// The directory rejects Go's default client outright — an unnamed
	// `Go-http-client/2.0` gets the connection dropped, while any identified agent is
	// served. Naming ourselves is both what unblocks the fetch and what lets the
	// network's operators see who is calling.
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("User-Agent")
		_, _ = fmt.Fprint(w, directoryPage(0, 1))
	}))
	defer srv.Close()

	if _, err := fetchSpeedrunDirectory(context.Background(), srv.Client(), srv.URL); err != nil {
		t.Fatalf("fetchSpeedrunDirectory: %v", err)
	}
	if !strings.Contains(got, "freehire") {
		t.Errorf("User-Agent = %q, want it to name freehire", got)
	}
	if strings.Contains(got, "Go-http-client") {
		t.Errorf("User-Agent = %q — the default client is what the directory blocks", got)
	}
}

func TestFetchSpeedrunDirectory_AMidWalkFailureIsAnError(t *testing.T) {
	// A short read is indistinguishable from a shrunken directory, and would
	// reconcile the tag off every company on the pages that were never reached.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.RawQuery, "page=2") {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = fmt.Fprint(w, directoryPage(0, 4))
	}))
	defer srv.Close()

	if _, err := fetchSpeedrunDirectory(context.Background(), srv.Client(), srv.URL); err == nil {
		t.Error("a failed page was swallowed — the walk returned a partial directory")
	}
}
