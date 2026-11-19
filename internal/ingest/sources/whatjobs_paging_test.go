package sources

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// whatjobsRows builds a page of n postings with distinct tracked ids offset by base.
func whatjobsRows(base, n int) string {
	rows := make([]string, 0, n)
	for i := 0; i < n; i++ {
		rows = append(rows, fmt.Sprintf(
			`{"title":"Engineer %[1]d","company":"Acme","location":"Austin","snippet":"<p>Body.</p>",`+
				`"url":"https://www.whatjobs.com/pub_api__cpl__%[1]d__7065"}`, base+i))
	}
	return `{"data":[` + strings.Join(rows, ",") + `]}`
}

// The feed post-filters duplicates AFTER selecting the page, so a page requested with limit=50
// routinely comes back with 44 rows while more pages remain. Treating a short page as the last one
// would silently truncate every keyword — the most costly mistake available here.
func TestWhatJobsShortPageDoesNotEndPagination(t *testing.T) {
	fake := (&recordingJSON{}).
		route("page=1", whatjobsRows(100, 44)). // short, yet not the last page
		route("page=2", whatjobsRows(200, 50)).
		route("page=3", `{"data":[]}`)

	jobs, err := NewWhatJobs(fake, "7065").Fetch(context.Background(), CompanyEntry{Board: "engineer"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if len(jobs) != 94 {
		t.Errorf("got %d jobs, want 94 (44 + 50): a short page must not stop the crawl", len(jobs))
	}
	if len(fake.urls) != 3 {
		t.Errorf("requested %d pages, want 3 (stop on the EMPTY page, not the short one)", len(fake.urls))
	}
}

// An empty page is the real end-of-results signal.
func TestWhatJobsEmptyPageEndsPagination(t *testing.T) {
	fake := (&recordingJSON{}).
		route("page=1", whatjobsRows(1, 50)).
		route("page=2", `{"data":[]}`)

	jobs, err := NewWhatJobs(fake, "7065").Fetch(context.Background(), CompanyEntry{Board: "engineer"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 50 {
		t.Errorf("got %d jobs, want 50", len(jobs))
	}
	if len(fake.urls) != 2 {
		t.Errorf("requested %d pages, want 2", len(fake.urls))
	}
}

// The feed serves nothing past roughly two thousand pages while reporting a far larger total, so a
// crawl must be bounded rather than trusting the reported page count. A keyword that never runs dry
// stops at the budget.
func TestWhatJobsPageBudgetBoundsAKeyword(t *testing.T) {
	// Every page is full, so only the budget can stop this crawl.
	fake := (&recordingJSON{}).route("jobs.json", whatjobsRows(1, 50))

	jobs, err := NewWhatJobs(fake, "7065").Fetch(context.Background(), CompanyEntry{Board: "engineer"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(fake.urls) != whatjobsMaxPages {
		t.Errorf("requested %d pages, want the budget %d", len(fake.urls), whatjobsMaxPages)
	}
	if len(jobs) == 0 {
		t.Error("the bounded crawl should still return the pages it read")
	}
}

// The feed post-filters duplicates per page, which means the same posting can surface on more than
// one page of the same keyword. Returning it repeatedly would have the pipeline upsert one row
// hundreds of times over a deep crawl, so the adapter collapses a keyword's repeats itself.
func TestWhatJobsCollapsesRepeatsWithinAKeyword(t *testing.T) {
	// Both pages carry the SAME ids, then the feed runs dry.
	fake := (&recordingJSON{}).
		route("page=1", whatjobsRows(1, 10)).
		route("page=2", whatjobsRows(1, 10)).
		route("page=3", `{"data":[]}`)

	jobs, err := NewWhatJobs(fake, "7065").Fetch(context.Background(), CompanyEntry{Board: "engineer"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if len(jobs) != 10 {
		t.Errorf("got %d jobs, want 10 distinct ids (the repeated page collapsed)", len(jobs))
	}
	seen := map[string]bool{}
	for _, j := range jobs {
		if seen[j.ExternalID] {
			t.Errorf("external id %q returned twice", j.ExternalID)
		}
		seen[j.ExternalID] = true
	}
}

// The postings cannot be liveness-probed (the url is a billing landing page, not the employer's
// posting) and a crawl reads only the first pages of each keyword, so the sweep must wait out
// page drift instead of closing on the 48-hour default.
func TestWhatJobsDeclaresWideSweepGrace(t *testing.T) {
	s, ok := NewWhatJobs(nil, "7065").(sweepGrace)
	if !ok {
		t.Fatal("whatjobs should declare a sweepGrace window")
	}
	if got := s.SweepGrace(); got != 14*24*time.Hour {
		t.Errorf("sweepGrace() = %v, want 14 days", got)
	}
}

// A whatjobs keyword board is read only to a page budget, so it can never earn fullBoardListing —
// yet its stored ids are namespaced by keyword, so a posting a finished board has stopped listing
// can be closed per board once the grace window has ruled out mere page drift. Each market is its
// own provider, so each must carry the marker.
func TestWhatJobsMarketsCloseByBoardOnTheirGraceWindow(t *testing.T) {
	got := SliceBoardListingProviders(Taxonomy())
	for _, m := range whatjobsMarkets {
		if !got[m.provider] {
			t.Errorf("SliceBoardListingProviders() lacks %q", m.provider)
		}
	}
	if got["greenhouse"] {
		t.Error("SliceBoardListingProviders() must not contain a complete-listing provider like greenhouse")
	}
}

// The marker admits a truncated crawl to the board scope, so it is only sound on a window that
// outlasts the crawl's page drift: a member without its own sweepGrace would close on the 48-hour
// default every posting that merely sat past a keyword's page budget for two days.
func TestSliceBoardListingRequiresASweepGrace(t *testing.T) {
	reg := Taxonomy()
	grace := SweepGraceWindows(reg)
	for provider := range SliceBoardListingProviders(reg) {
		if _, ok := grace[provider]; !ok {
			t.Errorf("%q is a sliceBoardListing provider but declares no sweepGrace", provider)
		}
	}
}
