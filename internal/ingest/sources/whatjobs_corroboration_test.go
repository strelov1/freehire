package sources

import (
	"context"
	"fmt"
	"slices"
	"testing"
)

// The feed's keyword RANKS rather than filters, so a posting has to prove it belongs to the keyword
// it was found under. Generic role words are stripped first: they appear in nearly every technical
// posting and would corroborate the radiology padding as readily as a real match.
func TestWhatJobsCorroborationTerms(t *testing.T) {
	cases := []struct {
		keyword string
		want    []string
	}{
		{"rust developer", []string{"rust"}},
		{"kubernetes engineer", []string{"kubernetes"}},
		{"golang", []string{"golang"}},
		{"node.js developer", []string{"node.js"}},
		{"machine learning engineer", []string{"machine", "learning"}},
		// Wholly generic: nothing distinguishes it, so there is nothing to corroborate on.
		{"developer", nil},
		{"software engineer", []string{"software"}},
	}
	for _, tc := range cases {
		got := whatjobsCorroborationTerms(tc.keyword)
		if !slices.Equal(got, tc.want) {
			t.Errorf("whatjobsCorroborationTerms(%q) = %v, want %v", tc.keyword, got, tc.want)
		}
	}
}

// Padding is what the first production run poured into the catalogue: one hospital group's radiology
// listings, returned under a developer keyword and waved through by the non-tech gate.
func TestWhatJobsDropsPostingsThatDoNotCorroborate(t *testing.T) {
	page := `{"data":[
	 {"title":"Senior Rust Engineer","company":"Acme","location":"Austin","snippet":"<p>Systems work.</p>",
	  "url":"https://www.whatjobs.com/pub_api__cpl__11__7065"},
	 {"title":"Senior CT Technologist - PRN","company":"Mercy","location":"Ardmore","snippet":"<p>Imaging.</p>",
	  "url":"https://www.whatjobs.com/pub_api__cpl__12__7065"},
	 {"title":"Backend Engineer","company":"Acme","location":"Austin","snippet":"<p>We use Rust heavily.</p>",
	  "url":"https://www.whatjobs.com/pub_api__cpl__13__7065"}
	]}`
	fake := (&recordingJSON{}).route("page=1", page).route("page=2", `{"data":[]}`)

	jobs, err := NewWhatJobs(fake, "7065").Fetch(context.Background(), CompanyEntry{Board: "rust developer"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	var got []string
	for _, j := range jobs {
		got = append(got, j.ExternalID)
	}
	// 11 names rust in the title, 13 in the description; 12 never does.
	if !slices.Equal(got, []string{"11", "13"}) {
		t.Errorf("kept %v, want [11 13] — the radiology posting must be dropped", got)
	}
}

// A term has to survive the feed's casing and punctuation: iOS is written "iOS", not "ios".
func TestWhatJobsCorroboratesCaseInsensitively(t *testing.T) {
	page := `{"data":[
	 {"title":"iOS Lead Developer","company":"Acme","location":"Plano","snippet":"<p>Swift.</p>",
	  "url":"https://www.whatjobs.com/pub_api__cpl__21__7065"},
	 {"title":"Node.JS Engineer","company":"Acme","location":"Plano","snippet":"<p>Server side.</p>",
	  "url":"https://www.whatjobs.com/pub_api__cpl__22__7065"}
	]}`
	for _, tc := range []struct{ keyword, wantID string }{
		{"ios developer", "21"},
		{"node.js developer", "22"},
	} {
		fake := (&recordingJSON{}).route("page=1", page).route("page=2", `{"data":[]}`)
		jobs, err := NewWhatJobs(fake, "7065").Fetch(context.Background(), CompanyEntry{Board: tc.keyword})
		if err != nil {
			t.Fatalf("Fetch(%q): %v", tc.keyword, err)
		}
		var ids []string
		for _, j := range jobs {
			ids = append(ids, j.ExternalID)
		}
		if !slices.Contains(ids, tc.wantID) {
			t.Errorf("keyword %q kept %v, want it to include %q", tc.keyword, ids, tc.wantID)
		}
	}
}

// A keyword with nothing distinguishing left cannot corroborate anything, so it must not filter
// everything away.
func TestWhatJobsGenericKeywordFiltersNothing(t *testing.T) {
	fake := (&recordingJSON{}).route("page=1", whatjobsRows(1, 5)).route("page=2", `{"data":[]}`)

	jobs, err := NewWhatJobs(fake, "7065").Fetch(context.Background(), CompanyEntry{Board: "developer"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 5 {
		t.Errorf("got %d jobs, want all 5 — a generic keyword has nothing to corroborate on", len(jobs))
	}
}

// Relevance does not taper, it collapses: 100% on page 1, 15% on page 2. Below the minimum share the
// feed has started padding, so every further page is mostly waste — and it was that wasted volume
// that rate-limited the first production run.
func TestWhatJobsStopsWhenAPageStopsCorroborating(t *testing.T) {
	// Page 1: 4 of 4 name rust. Page 2: 1 of 4. Page 3 would be more padding.
	page1 := `{"data":[
	 {"title":"Rust Engineer A","company":"A","location":"X","snippet":"<p>rust</p>","url":"https://www.whatjobs.com/pub_api__cpl__1__7065"},
	 {"title":"Rust Engineer B","company":"A","location":"X","snippet":"<p>rust</p>","url":"https://www.whatjobs.com/pub_api__cpl__2__7065"},
	 {"title":"Rust Engineer C","company":"A","location":"X","snippet":"<p>rust</p>","url":"https://www.whatjobs.com/pub_api__cpl__3__7065"},
	 {"title":"Rust Engineer D","company":"A","location":"X","snippet":"<p>rust</p>","url":"https://www.whatjobs.com/pub_api__cpl__4__7065"}
	]}`
	page2 := `{"data":[
	 {"title":"Rust Engineer E","company":"A","location":"X","snippet":"<p>rust</p>","url":"https://www.whatjobs.com/pub_api__cpl__5__7065"},
	 {"title":"CT Technologist","company":"M","location":"X","snippet":"<p>imaging</p>","url":"https://www.whatjobs.com/pub_api__cpl__6__7065"},
	 {"title":"Mammography Technologist","company":"M","location":"X","snippet":"<p>imaging</p>","url":"https://www.whatjobs.com/pub_api__cpl__7__7065"},
	 {"title":"Nuclear Medicine Technologist","company":"M","location":"X","snippet":"<p>imaging</p>","url":"https://www.whatjobs.com/pub_api__cpl__8__7065"}
	]}`
	fake := (&recordingJSON{}).route("page=1", page1).route("page=2", page2).route("page=3", whatjobsRows(90, 50))

	jobs, err := NewWhatJobs(fake, "7065").Fetch(context.Background(), CompanyEntry{Board: "rust developer"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if len(fake.urls) != 2 {
		t.Errorf("requested %d pages, want 2 — the collapsed page must end the crawl", len(fake.urls))
	}
	// The corroborated posting on the collapsed page is real and is kept.
	if len(jobs) != 5 {
		t.Errorf("got %d jobs, want 5 (4 from page 1 plus the one real posting on page 2)", len(jobs))
	}
}

// A keyword the feed serves honestly must still be crawled to the end.
func TestWhatJobsFullyCorroboratingKeywordCrawlsOn(t *testing.T) {
	full := `{"data":[
	 {"title":"Rust Engineer","company":"A","location":"X","snippet":"<p>rust</p>","url":"https://www.whatjobs.com/pub_api__cpl__%d__7065"}
	]}`
	fake := (&recordingJSON{}).
		route("page=1", fmt.Sprintf(full, 1)).
		route("page=2", fmt.Sprintf(full, 2)).
		route("page=3", fmt.Sprintf(full, 3)).
		route("page=4", `{"data":[]}`)

	jobs, err := NewWhatJobs(fake, "7065").Fetch(context.Background(), CompanyEntry{Board: "rust developer"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(fake.urls) != 4 {
		t.Errorf("requested %d pages, want 4 (three full, then the empty one)", len(fake.urls))
	}
	if len(jobs) != 3 {
		t.Errorf("got %d jobs, want 3", len(jobs))
	}
}
