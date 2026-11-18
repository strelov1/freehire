package sources

import (
	"context"
	"strings"
	"testing"
)

func linkedInCard(id string) string {
	return `<li data-entity-urn="urn:li:jobPosting:` + id + `"><a class="base-card__full-link" href="https://www.linkedin.com/jobs/view/engineer-` + id + `/"></a><h3 class="base-search-card__title">Go Engineer</h3><h4 class="base-search-card__subtitle">Acme</h4></li>`
}

func TestLinkedInFetchNewReadsOnlyUnseenPostings(t *testing.T) {
	// Any request for the seen posting's detail would be refused, which would stop the board.
	fake := (&routedHTTP{}).
		routeErr("jobPosting/123", &StatusError{Code: 403}).
		route("start=0", linkedInCard("123")+linkedInCard("456")).
		route("start=2", "").
		route("/jobs-guest/jobs/api/jobPosting/456", strings.ReplaceAll(linkedInOpenPage, "123", "456"))
	s := NewLinkedIn(fake).(linkedIn)
	s.pace = &recordingWaiter{}
	jobs, err := s.FetchNew(context.Background(), CompanyEntry{Board: "Go", Region: "UK"}, func(id string) bool { return id == "123" })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	if j := byID["123"]; !j.SeenRefresh || j.Unreadable || j.Description != "" || j.URL != LinkedInPostingURL("123") || j.Title == "" || j.Company == "" {
		t.Errorf("seen posting = %+v, want identity-only SeenRefresh", j)
	}
	if j := byID["456"]; j.SeenRefresh || j.Description == "" {
		t.Errorf("unseen posting = %+v, want a fully read posting", j)
	}
}

// A board whose listed postings are all known is healthy, not an unreadable one.
func TestLinkedInFetchNewAllSeenIsNotAnUnreadableBoard(t *testing.T) {
	fake := (&routedHTTP{}).route("start=0", linkedInCard("123")+linkedInCard("456")).route("start=2", "")
	s := NewLinkedIn(fake).(linkedIn)
	s.pace = &recordingWaiter{}
	jobs, err := s.FetchNew(context.Background(), CompanyEntry{Board: "Go", Region: "UK"}, func(string) bool { return true })
	if err != nil || len(jobs) != 2 || !jobs[0].SeenRefresh || !jobs[1].SeenRefresh {
		t.Fatalf("all-seen board: %+v, %v", jobs, err)
	}
}

// Fetch stays the list-only fallback that reads every listed posting.
func TestLinkedInFetchWithoutSeenSetReadsEverything(t *testing.T) {
	fake := (&routedHTTP{}).route("start=0", linkedInCard("123")).route("start=1", "").
		route("/jobs-guest/jobs/api/jobPosting/123", linkedInOpenPage)
	s := NewLinkedIn(fake).(linkedIn)
	s.pace = &recordingWaiter{}
	jobs, err := s.Fetch(context.Background(), CompanyEntry{Board: "Go", Region: "UK"})
	if err != nil || len(jobs) != 1 || jobs[0].SeenRefresh || jobs[0].Description == "" {
		t.Fatalf("Fetch: %+v, %v", jobs, err)
	}
}

// A stored seed whose companion page shows the closed notice must not be touched this crawl:
// the touch would refresh last_seen_at on a row this crawl just saw closed.
func TestLinkedInFetchNewWithholdsTouchOfSeedShownClosed(t *testing.T) {
	closed := strings.Replace(linkedInOpenPage, `<button data-modal="job-details-topcard-apply-modal">Apply</button>`, `<figcaption class="closed-job__flavor--closed">No longer accepting applications</figcaption>`, 1)
	fake := (&routedHTTP{}).
		route("start=0", linkedInCard("123")).route("start=1", "").
		route("/jobs/view/123/", closed)
	s := NewLinkedIn(fake).(linkedIn)
	s.pace = &recordingWaiter{}
	jobs, err := s.FetchNew(context.Background(), CompanyEntry{Board: "Go", Region: "UK"}, func(string) bool { return true })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("closed-shown stored seed still emitted: %+v", jobs)
	}
}
