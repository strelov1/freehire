package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// apploiListJSON is an api.apploi.com/v1/jobs?employer=<id> page: one live posting plus an
// archived one (which the adapter must drop), with the description inline (no detail fetch).
const apploiListJSON = `{"data":[
{"id":"1882468","name":"Car Rental Cleaner","description":"<p>Clean cars.</p><script>x()<\/script>","city":"Koloa","state":"Hawaii","country":"United States","published_date":"2026-06-18T17:23:00+00:00","brand_name_with_company_only":"OnTray","published":true,"archived":false,"private":false},
{"id":"9999","name":"Archived Role","published":true,"archived":true,"private":false}
],"limit":100,"offset":0}`

func TestApploiProvider(t *testing.T) {
	if got := NewApploi(nil).Provider(); got != "apploi" {
		t.Errorf("Provider() = %q, want %q", got, "apploi")
	}
}

func TestApploiFetch(t *testing.T) {
	fake := (&routedHTTP{}).route("/v1/jobs", apploiListJSON)

	jobs, err := NewApploi(fake).Fetch(context.Background(),
		CompanyEntry{Company: "OnTray", Board: "41350"})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("jobs = %d, want 1 (archived posting must be dropped)", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "1882468" {
		t.Errorf("external_id = %q", j.ExternalID)
	}
	if j.Title != "Car Rental Cleaner" {
		t.Errorf("title = %q", j.Title)
	}
	if j.Company != "OnTray" {
		t.Errorf("company = %q", j.Company)
	}
	if j.Location != "Koloa, Hawaii, United States" {
		t.Errorf("location = %q", j.Location)
	}
	if j.URL != "https://jobs.apploi.com/view/1882468" {
		t.Errorf("url = %q", j.URL)
	}
	if j.PostedAt == nil {
		t.Error("posted_at not parsed")
	}
	if !strings.Contains(j.Description, "Clean cars") || strings.Contains(j.Description, "x()") {
		t.Errorf("description not sanitized: %q", j.Description)
	}
}

// An employer with no live openings returns an empty data array, yielding zero jobs and no
// error (not a board-level failure).
func TestApploiFetchEmpty(t *testing.T) {
	fake := (&routedHTTP{}).route("/v1/jobs", `{"data":[],"limit":100,"offset":0}`)
	jobs, err := NewApploi(fake).Fetch(context.Background(), CompanyEntry{Board: "1"})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("jobs = %d, want 0", len(jobs))
	}
}

// A later page failing must fail the whole crawl, not return the pages gathered so far —
// the fullBoardListing bar (source.go).
func TestApploiFetchFailsOnALaterPageError(t *testing.T) {
	fullPage := apploiFullPageJSON(apploiPageSize)
	fake := (&routedHTTP{}).
		route("offset=0", fullPage).
		routeErr("offset=100", errors.New("boom"))

	_, err := NewApploi(fake).Fetch(context.Background(), CompanyEntry{Board: "1"})
	if err == nil {
		t.Fatal("Fetch succeeded despite page 2 failing — a later-page failure must not be treated as the board's natural end")
	}
}

// apploiFullPageJSON renders a page of exactly n live postings, all with distinct ids — used
// to build a page that is NOT short (n == apploiPageSize), so the walk must keep going.
func apploiFullPageJSON(n int) string {
	var b strings.Builder
	b.WriteString(`{"data":[`)
	for i := range n {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"id":"%d","name":"Role","published":true,"archived":false,"private":false}`, 10000+i)
	}
	b.WriteString(`],"limit":100,"offset":0}`)
	return b.String()
}

// apploiEndlessFake serves a full (never-short) page for any offset requested, so it never
// yields the "last page" proof — used to prove the page-cap ceiling fails loudly rather than
// succeeding partially. Mirrors taleoEndlessFake / gustoEndlessFake / baytEndlessListingFake.
type apploiEndlessFake struct{ calls int }

func (f *apploiEndlessFake) GetJSON(_ context.Context, _ string, v any) error {
	f.calls++
	return json.Unmarshal([]byte(apploiFullPageJSON(apploiPageSize)), v)
}

func TestApploiFetchFailsWhenListingExceedsThePageCap(t *testing.T) {
	fake := &apploiEndlessFake{}

	_, err := NewApploi(fake).Fetch(context.Background(), CompanyEntry{Board: "1"})
	if err == nil {
		t.Fatal("expected reaching the page cap to fail the Fetch")
	}
	if fake.calls != apploiMaxPages {
		t.Errorf("got %d listing calls, want exactly %d (the cap, no more)", fake.calls, apploiMaxPages)
	}
}

func TestApploiRegisteredAsFullBoardListing(t *testing.T) {
	if _, ok := NewApploi(nil).(fullBoardListing); !ok {
		t.Error("apploi should implement the fullBoardListing marker")
	}
	if !FullBoardListingProviders(All(nil))["apploi"] {
		t.Error("FullBoardListingProviders(All(nil)) should include apploi")
	}
}
