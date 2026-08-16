package sources

import (
	"context"
	"testing"
)

// The whole point: a posting the catalogue already holds costs no detail request. Teamtailor
// re-fetched every description every hour — 36771 detail pages for the ~100 postings that were
// actually new — which is what made the crawl a ~37k-request burst its edge turned away.
func TestTeamtailorFetchNewSkipsDetailForSeenPostings(t *testing.T) {
	seenURL := "https://jobs.tibber.com/jobs/1111111-known-role"
	newURL := "https://jobs.tibber.com/jobs/2222222-fresh-role"
	fake := (&routedHTTP{}).
		route("page=1", ttListingHTML(seenURL, newURL)).
		route("page=2", ttListingHTML()).
		route("/jobs/2222222", ttDetailHTML("Fresh Role", "&lt;p&gt;New&lt;/p&gt;",
			"2026-08-01T00:00:00+02:00", "Stockholm", "SE", ""))
	// Deliberately no route for /jobs/1111111: if the adapter fetches the seen posting's
	// detail the fake errors, and the posting is dropped instead of refreshed.

	jobs, err := NewTeamtailor(fake).(HydratingSource).FetchNew(context.Background(),
		CompanyEntry{Company: "Tibber", Provider: "teamtailor", Board: "jobs.tibber.com"},
		func(id string) bool { return id == "1111111" })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2 (one refresh, one hydrated)", len(jobs))
	}

	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	refresh, ok := byID["1111111"]
	if !ok {
		t.Fatal("the seen posting was dropped; it must still be re-listed as a liveness refresh")
	}
	if !refresh.SeenRefresh {
		t.Error("seen posting not marked SeenRefresh — the pipeline would re-upsert it content-less " +
			"and wipe the description and facets hydrated when it was new")
	}
	if refresh.Description != "" {
		t.Error("a refresh must carry no content")
	}
	if refresh.URL != seenURL {
		t.Errorf("refresh URL = %q, want %q — the identity still has to resolve", refresh.URL, seenURL)
	}

	hydrated, ok := byID["2222222"]
	if !ok {
		t.Fatal("the unseen posting was not hydrated")
	}
	if hydrated.SeenRefresh {
		t.Error("a newly hydrated posting must not be marked SeenRefresh")
	}
	if hydrated.Description == "" || hydrated.Title != "Fresh Role" {
		t.Errorf("unseen posting not hydrated: title=%q descLen=%d", hydrated.Title, len(hydrated.Description))
	}
}

// The request count is the deliverable, so assert it directly: two listing pages and exactly one
// detail fetch for the one unseen posting, out of three postings listed.
func TestTeamtailorFetchNewIssuesOneDetailRequestPerUnseenPosting(t *testing.T) {
	fake := (&routedHTTP{}).
		route("page=1", ttListingHTML(
			"https://jobs.tibber.com/jobs/1-a",
			"https://jobs.tibber.com/jobs/2-b",
			"https://jobs.tibber.com/jobs/3-c")).
		route("page=2", ttListingHTML()).
		route("/jobs/3", ttDetailHTML("C", "&lt;p&gt;c&lt;/p&gt;", "", "", "", ""))

	if _, err := NewTeamtailor(fake).(HydratingSource).FetchNew(context.Background(),
		CompanyEntry{Company: "Tibber", Provider: "teamtailor", Board: "jobs.tibber.com"},
		func(id string) bool { return id == "1" || id == "2" }); err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	// 2 listing pages + 1 detail. Unpaced, the old crawl would have made 5.
	if fake.calls != 3 {
		t.Errorf("made %d requests, want 3 (2 listing + 1 detail for the single unseen posting)", fake.calls)
	}
}

// Every posting already known means a run that touches no detail page at all — the steady state
// this change exists to reach, since only ~1 posting an hour is genuinely new.
func TestTeamtailorFetchNewFetchesNoDetailWhenAllSeen(t *testing.T) {
	fake := (&routedHTTP{}).
		route("page=1", ttListingHTML("https://jobs.tibber.com/jobs/1-a", "https://jobs.tibber.com/jobs/2-b")).
		route("page=2", ttListingHTML())

	jobs, err := NewTeamtailor(fake).(HydratingSource).FetchNew(context.Background(),
		CompanyEntry{Company: "Tibber", Provider: "teamtailor", Board: "jobs.tibber.com"},
		func(string) bool { return true })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2 refreshes", len(jobs))
	}
	if fake.calls != 2 {
		t.Errorf("made %d requests, want 2 (listing only)", fake.calls)
	}
}

// A board whose listing fails must fail the board, not report it empty — an empty result would
// let the unseen sweep close every posting the board still has.
func TestTeamtailorFetchNewPropagatesListingFailure(t *testing.T) {
	fake := &routedHTTP{} // no routes: the first listing GET errors

	if _, err := NewTeamtailor(fake).(HydratingSource).FetchNew(context.Background(),
		CompanyEntry{Company: "Tibber", Provider: "teamtailor", Board: "jobs.tibber.com"},
		func(string) bool { return false }); err == nil {
		t.Fatal("FetchNew returned nil error for a board whose listing failed")
	}
}

// Fetch stays the list-and-hydrate-everything fallback the pipeline uses when it cannot supply a
// seen set, so it must keep hydrating regardless.
func TestTeamtailorFetchStillHydratesEverything(t *testing.T) {
	fake := (&routedHTTP{}).
		route("page=1", ttListingHTML("https://jobs.tibber.com/jobs/1-a")).
		route("page=2", ttListingHTML()).
		route("/jobs/1", ttDetailHTML("A", "&lt;p&gt;a&lt;/p&gt;", "", "", "", ""))

	jobs, err := NewTeamtailor(fake).Fetch(context.Background(),
		CompanyEntry{Company: "Tibber", Provider: "teamtailor", Board: "jobs.tibber.com"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].Description == "" || jobs[0].SeenRefresh {
		t.Errorf("Fetch no longer hydrates: %+v", jobs)
	}
}
