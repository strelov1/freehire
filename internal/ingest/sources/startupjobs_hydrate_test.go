package sources

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
)

// startupjobsCatalogue is a small but complete site: postings in the sitemap (old) and in the
// window (new), some stored already, some not technical, one from a covered employer.
func startupjobsCatalogue(t *testing.T) (startupjobsFakeFeed, *startupjobsFakeIndex) {
	t.Helper()
	old := startupjobsSince - 7*86400
	recent := startupjobsSince + 3600
	records := []map[string]any{
		startupjobsRecord(t, 100, old, "Backend Engineer", "Acme"),     // stored
		startupjobsRecord(t, 101, old, "Data Engineer", "Acme"),        // backlog, technical
		startupjobsRecord(t, 102, old, "Registered Nurse", "Acme"),     // backlog
		startupjobsRecord(t, 200, recent, "Platform Engineer", "Acme"), // window, stored
		startupjobsRecord(t, 201, recent, "Frontend Engineer", "Acme"), // window, new
		startupjobsRecord(t, 202, recent, "SRE", "Covered Co"),         // window, covered employer
	}
	feed := startupjobsFakeFeed{posts: []string{
		"https://startup.jobs/posting-100", "https://startup.jobs/posting-101",
		"https://startup.jobs/posting-102",
		"https://startup.jobs/posting-99", // removed since the sitemap: the index no longer knows it
	}}
	return feed, &startupjobsFakeIndex{records: records}
}

// The whole run: the sitemap refreshes what is stored, the window and the backlog bring the
// rest with their metadata, and a body is bought only for a posting the pipeline will keep. A
// covered or rejected posting is yielded body-less so the pipeline still decides and counts it.
func TestStartupJobsFetchNewGatedBuysOnlyBodiesThePipelineKeeps(t *testing.T) {
	feed, idx := startupjobsCatalogue(t)
	site := newStartupjobsFakeSite(t)
	s := newStartupjobsTest(site, feed, idx, 0)

	var askedAbout []string
	gate := DetailGate{
		Covered: func(names []string) map[string]bool {
			askedAbout = names
			return map[string]bool{"Covered Co": true}
		},
	}
	seen := func(id string) bool { return id == "100" || id == "200" }
	jobs, err := s.FetchNewGated(context.Background(), CompanyEntry{Provider: "startupjobs"}, seen, gate)
	if err != nil {
		t.Fatalf("FetchNewGated: %v", err)
	}
	if len(askedAbout) != 4 {
		t.Errorf("coverage asked about %v, want the four candidates' employers", askedAbout)
	}
	want := []string{"https://startup.jobs/posting-101", "https://startup.jobs/posting-102", "https://startup.jobs/posting-201"}
	if got := site.postingsAsked(); !slices.Equal(got, want) {
		t.Errorf("posting pages fetched = %v, want only %v", got, want)
	}

	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	if len(jobs) != 6 {
		t.Fatalf("got %d jobs (%v), want 6", len(jobs), byID)
	}
	if j := byID["100"]; !j.SeenRefresh || j.URL != "https://startup.jobs/posting-100" {
		t.Errorf("stored sitemap posting = %+v, want a refresh by identity", j)
	}
	if j := byID["200"]; !j.SeenRefresh || j.Title != "Platform Engineer" {
		t.Errorf("stored window posting = %+v, want a refresh carrying its title", j)
	}
	for _, id := range []string{"101", "102", "201"} {
		if j := byID[id]; !strings.Contains(j.Description, "Fonzi Match Day") {
			t.Errorf("new posting %s description = %q, want the posting page's JobPosting body", id, j.Description)
		}
	}
	for _, id := range []string{"202"} {
		if j := byID[id]; j.Description != "" || j.SeenRefresh {
			t.Errorf("posting %s: description %q seenRefresh %v, want body-less and new", id, j.Description, j.SeenRefresh)
		}
	}
	if _, ok := byID["99"]; ok {
		t.Error("a posting the index no longer knows must not be yielded")
	}
	if idx.gotHeaders["X-Algolia-Application-Id"] != "4CQMTMMK73" || idx.gotHeaders["X-Algolia-API-Key"] != "test-secured-search-key" {
		t.Errorf("credentials sent = %v, want the homepage's", idx.gotHeaders)
	}
	if idx.gotURL != "https://4cqmtmmk73-dsn.algolia.net/1/indexes/Post_production/query" {
		t.Errorf("algolia url = %q", idx.gotURL)
	}
}

// fullCatalog lets the sweep close by source whatever a run did not list, so every way the
// listing can come back incomplete must fail the run instead.
func TestStartupJobsRefusesAnIncompleteListing(t *testing.T) {
	never := func(string) bool { return false }

	feed, idx := startupjobsCatalogue(t)
	idx.failOnQuery = 2
	if jobs, err := newStartupjobsTest(newStartupjobsFakeSite(t), feed, idx, 0).FetchNew(context.Background(), CompanyEntry{}, never); err == nil || jobs != nil {
		t.Errorf("a failed Algolia query: jobs=%d err=%v, want no jobs and an error", len(jobs), err)
	}

	feed, idx = startupjobsCatalogue(t)
	for i := range 40 { // the index holds far more pre-window postings than the sitemap lists
		idx.records = append(idx.records, startupjobsRecord(t, 300+i, startupjobsSince-86400, "Backend Engineer", "Acme"))
	}
	_, err := newStartupjobsTest(newStartupjobsFakeSite(t), feed, idx, 0).FetchNew(context.Background(), CompanyEntry{}, never)
	if err == nil || !strings.Contains(err.Error(), "sitemap lists") {
		t.Errorf("a short sitemap: err = %v, want the refusal", err)
	}

	feed, idx = startupjobsCatalogue(t)
	feed.lastMod = "2026-10-12T03:00"
	_, err = newStartupjobsTest(newStartupjobsFakeSite(t), feed, idx, 0).FetchNew(context.Background(), CompanyEntry{}, never)
	if err == nil || !strings.Contains(err.Error(), "sitemap generated") {
		t.Errorf("a stale sitemap: err = %v, want the refusal", err)
	}
}

// The budget bounds one run's posting-page requests; what it leaves out is not yielded, so it
// stays new and is offered again by a later run.
func TestStartupJobsHydrationBudgetDefersTheRest(t *testing.T) {
	feed, idx := startupjobsCatalogue(t)
	site := newStartupjobsFakeSite(t)
	jobs, err := newStartupjobsTest(site, feed, idx, 1).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(site.postingsAsked()) != 1 || len(jobs) != 1 {
		t.Errorf("fetched %d pages and yielded %d jobs, want 1 and 1", len(site.postingsAsked()), len(jobs))
	}
}

// A 404 is the platform saying the posting is gone: dropped, and not a reason to fail the run.
// A refusal is about the IP: the rest of the run's hydration stops, and a run that read nothing
// it needed fails, because "listed postings, read none" is how the Cloudflare wall looks.
func TestStartupJobsPostingPageFailures(t *testing.T) {
	never := func(string) bool { return false }

	feed, idx := startupjobsCatalogue(t)
	site := newStartupjobsFakeSite(t)
	site.status["https://startup.jobs/posting-101"] = http.StatusNotFound
	jobs, err := newStartupjobsTest(site, feed, idx, 0).FetchNew(context.Background(), CompanyEntry{}, never)
	if err != nil {
		t.Fatalf("a gone posting failed the run: %v", err)
	}
	if len(jobs) != 5 {
		t.Errorf("got %d jobs, want the 5 readable ones", len(jobs))
	}

	// Every page gone is an answer about every posting, not the wall.
	feed, idx = startupjobsCatalogue(t)
	gone := newStartupjobsFakeSite(t)
	for _, id := range []int{100, 101, 102, 200, 201, 202} {
		gone.status[fmt.Sprintf("https://startup.jobs/posting-%d", id)] = http.StatusGone
	}
	if _, err := newStartupjobsTest(gone, feed, idx, 0).FetchNew(context.Background(), CompanyEntry{}, never); err != nil {
		t.Errorf("every posting gone failed the run: %v", err)
	}

	feed, idx = startupjobsCatalogue(t)
	refused := newStartupjobsFakeSite(t)
	for _, id := range []int{100, 101, 102, 200, 201, 202} {
		refused.status[fmt.Sprintf("https://startup.jobs/posting-%d", id)] = http.StatusForbidden
	}
	_, err = newStartupjobsTest(refused, feed, idx, 0).FetchNew(context.Background(), CompanyEntry{}, never)
	if err == nil || !strings.Contains(err.Error(), "read none") {
		t.Errorf("err = %v, want the read-none failure", err)
	}
	if n := len(refused.postingsAsked()); n > startupjobsDetailWorkers {
		t.Errorf("asked for %d posting pages after the edge refused; want at most the %d already in flight", n, startupjobsDetailWorkers)
	}
}
