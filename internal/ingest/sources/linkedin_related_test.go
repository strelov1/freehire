package sources

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func linkedInRelatedPosting(id string) string {
	page := strings.ReplaceAll(linkedInOpenPage, "123", id)
	return strings.Replace(page, `data-modal="job-details-topcard-apply-modal"`, `data-tracking-control-name="public_jobs_apply-link-onsite"`, 1)
}

func linkedInRelatedListing(id string) string {
	return `<li data-entity-urn="urn:li:jobPosting:` + id + `"><h3 class="base-search-card__title">Software Engineer</h3><h4 class="base-search-card__subtitle">Acme</h4></li>`
}

func TestLinkedInRelatedFetchUsesLiveCardFixtureWithBoundedDepthAndPacing(t *testing.T) {
	fixture, err := os.ReadFile("testdata/linkedin_related.html")
	if err != nil {
		t.Fatal(err)
	}
	var details []string
	publicReads := 0
	getter := linkedInGetterFunc(func(_ context.Context, raw string) (*html.Node, string, error) {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		var body string
		switch {
		case u.Path == "/jobs-guest/jobs/api/seeMoreJobPostings/search":
			if u.Query().Get("start") == "0" {
				body = linkedInRelatedListing("4490679811")
			}
		case strings.HasPrefix(u.Path, "/jobs-guest/jobs/api/jobPosting/"):
			id := strings.TrimPrefix(u.Path, "/jobs-guest/jobs/api/jobPosting/")
			details = append(details, id)
			body = linkedInRelatedPosting(id)
		case u.Path == "/jobs/view/4490679811/":
			publicReads++
			body = strings.Replace(linkedInRelatedPosting("4490679811"), "</body>", string(fixture)+"</body>", 1)
		default:
			return nil, "", fmt.Errorf("unexpected request %s", raw)
		}
		root, err := html.Parse(strings.NewReader(body))
		return root, raw, err
	})
	source := NewLinkedIn(getter).(linkedIn)
	pace := &recordingWaiter{}
	source.pace = pace
	jobs, err := source.Fetch(context.Background(), CompanyEntry{Board: "software engineer", Region: "Netherlands"})
	if err != nil || len(jobs) != 21 {
		t.Fatalf("jobs=%d error=%v; want primary and20 hydrated recommendations", len(jobs), err)
	}
	if jobs[0].ExternalID != "4490679811" || jobs[1].ExternalID != "4470468200" || jobs[20].ExternalID != "4487682985" {
		t.Fatalf("wrong recommendation order/limit: first=%s last=%s", jobs[1].ExternalID, jobs[20].ExternalID)
	}
	for _, j := range jobs {
		if j.Description == "" || j.ApplyMethod != ApplyOnSite {
			t.Fatalf("recommendation bypassed primary detail/application reader: %+v", j)
		}
	}
	if publicReads != 1 || len(details) != 21 || pace.calls != 24 {
		t.Fatalf("unbounded, recursive or ungated requests: public=%d details=%d paced=%d", publicReads, len(details), pace.calls)
	}
}

type linkedInRelatedSite struct {
	listings       map[string]string
	listingError   map[string]error
	companion      string
	companionFinal string
	companionError error
	detailPages    map[string]string
	detailError    map[string]error
	requests       []string
}

func (s *linkedInRelatedSite) GetHTMLResolved(_ context.Context, raw string) (*html.Node, string, error) {
	s.requests = append(s.requests, raw)
	u, err := url.Parse(raw)
	if err != nil {
		return nil, "", err
	}
	var body string
	final := raw
	switch {
	case u.Path == "/jobs-guest/jobs/api/seeMoreJobPostings/search":
		offset := u.Query().Get("start")
		if err := s.listingError[offset]; err != nil {
			return nil, "", err
		}
		body = s.listings[offset]
	case strings.HasPrefix(u.Path, "/jobs-guest/jobs/api/jobPosting/"):
		id := strings.TrimPrefix(u.Path, "/jobs-guest/jobs/api/jobPosting/")
		if err := s.detailError[id]; err != nil {
			return nil, "", err
		}
		body = linkedInRelatedPosting(id)
		if page, ok := s.detailPages[id]; ok {
			body = page
		}
	case u.Path == "/jobs/view/123/":
		if s.companionError != nil {
			return nil, "", s.companionError
		}
		body = s.companion
		if s.companionFinal != "" {
			final = s.companionFinal
		}
	default:
		return nil, "", fmt.Errorf("unexpected request %s", raw)
	}
	root, err := html.Parse(strings.NewReader(body))
	return root, final, err
}

func relatedTestCard(id, raw string) string {
	return `<div data-entity-urn="urn:li:jobPosting:` + id + `"><a class="base-card__full-link" data-tracking-control-name="public_jobs_similar-jobs" href="` + raw + `">Engineer</a><h3 class="base-main-card__title">Engineer</h3><h4 class="base-main-card__subtitle">Acme</h4></div>`
}

func relatedTestPage(cards string) string {
	return strings.Replace(linkedInRelatedPosting("123"), "</body>", `<section class="similar-jobs">`+cards+`</section></body>`, 1)
}

func runRelatedTest(t *testing.T, site *linkedInRelatedSite) []Job {
	t.Helper()
	source := NewLinkedIn(site).(linkedIn)
	source.pace, source.backoff = &recordingWaiter{}, 0
	jobs, err := source.Fetch(context.Background(), CompanyEntry{Board: "software engineer", Region: "Netherlands"})
	if err != nil {
		t.Fatal(err)
	}
	return jobs
}

func TestLinkedInRelatedRejectsUnsafeCardsAndDuplicateIdentities(t *testing.T) {
	card := func(id string) string {
		return relatedTestCard(id, "https://nl.linkedin.com/jobs/view/engineer-"+id+"/?trk=related")
	}
	cards := card("123") + card("456") + card("456") + card("0") + card("not-789") +
		relatedTestCard("790", "https://linkedin.com.evil.test/jobs/view/790/") +
		relatedTestCard("791", "https://evil@www.linkedin.com/jobs/view/791/") +
		relatedTestCard("792", "https://www.linkedin.com:443/jobs/view/792/") +
		relatedTestCard("793", "https://www.linkedin.com/jobs/view/794/") +
		relatedTestCard("795", "/jobs/view/795/") +
		strings.Replace(card("796"), "public_jobs_similar-jobs", "public_jobs_people_also_viewed", 1) +
		strings.Replace(card("797"), `<h4 class="base-main-card__subtitle">Acme</h4>`, "", 1)
	site := &linkedInRelatedSite{listings: map[string]string{"0": linkedInRelatedListing("123")}, companion: relatedTestPage(cards) + card("798")}
	jobs := runRelatedTest(t, site)
	if len(jobs) != 2 || jobs[1].ExternalID != "456" || jobs[1].URL != "https://www.linkedin.com/jobs/view/456/" {
		t.Fatalf("unsafe or duplicate related candidates admitted: %+v", jobs)
	}
	if len(site.requests) != 5 {
		t.Fatalf("unsafe cards cost requests: %v", site.requests)
	}
}

func TestLinkedInRelatedDeduplicatesAgainstEveryPrimaryPage(t *testing.T) {
	site := &linkedInRelatedSite{
		listings:  map[string]string{"0": linkedInRelatedListing("123"), "1": linkedInRelatedListing("456")},
		companion: relatedTestPage(relatedTestCard("456", "https://www.linkedin.com/jobs/view/456/") + relatedTestCard("789", "https://www.linkedin.com/jobs/view/789/")),
	}
	jobs := runRelatedTest(t, site)
	if len(jobs) != 3 || jobs[0].ExternalID != "123" || jobs[1].ExternalID != "456" || jobs[2].ExternalID != "789" {
		t.Fatalf("primary overlap was not deduplicated: %+v", jobs)
	}
	primaryReads, companionIndex := 0, -1
	for i, raw := range site.requests {
		if strings.Contains(raw, "/api/jobPosting/456") {
			primaryReads++
		}
		if strings.Contains(raw, "/jobs/view/123/") {
			companionIndex = i
		}
	}
	if primaryReads != 1 || companionIndex != 5 {
		t.Fatalf("recommendations preceded all primary pages or reread an overlap: %v", site.requests)
	}
}

func TestLinkedInRelatedDoesNotRecurseIntoRecommendations(t *testing.T) {
	site := &linkedInRelatedSite{
		listings:    map[string]string{"0": linkedInRelatedListing("123")},
		companion:   relatedTestPage(relatedTestCard("456", "https://www.linkedin.com/jobs/view/456/")),
		detailPages: map[string]string{"456": strings.Replace(linkedInRelatedPosting("456"), "</body>", `<section class="similar-jobs">`+relatedTestCard("789", "https://www.linkedin.com/jobs/view/789/")+`</section></body>`, 1)},
	}
	jobs := runRelatedTest(t, site)
	if len(jobs) != 2 || jobs[1].ExternalID != "456" || len(site.requests) != 5 {
		t.Fatalf("recommendations recursed: jobs=%+v requests=%v", jobs, site.requests)
	}
}

func TestLinkedInRelatedRequiresVerifiedLiveCompanionPage(t *testing.T) {
	page := relatedTestPage(relatedTestCard("456", "https://www.linkedin.com/jobs/view/456/"))
	closed := strings.Replace(page, `<button data-tracking-control-name="public_jobs_apply-link-onsite">Apply</button>`, `<figcaption class="closed-job__flavor--closed">No longer accepting applications</figcaption>`, 1)
	for _, tc := range []struct {
		name, page, final string
		err               error
	}{
		{name: "login", page: "<h1>Sign in</h1>"},
		{name: "wrong final", page: page, final: "https://www.linkedin.com/jobs/view/999/"},
		{name: "offhost final", page: page, final: "https://evil.test/jobs/view/123/"},
		{name: "userinfo final", page: page, final: "https://user@www.linkedin.com/jobs/view/123/"},
		{name: "wrong og identity", page: strings.Replace(page, "view/123/", "view/999/", 1)},
		{name: "wrong canonical", page: strings.Replace(page, "<head>", `<head><link rel="canonical" href="https://www.linkedin.com/jobs/view/999/">`, 1)},
		{name: "userinfo canonical", page: strings.Replace(page, "<head>", `<head><link rel="canonical" href="https://user@www.linkedin.com/jobs/view/123/">`, 1)},
		{name: "canonical relation token", page: strings.Replace(page, "<head>", `<head><link rel="alternate canonical" href="https://www.linkedin.com/jobs/view/999/">`, 1)},
		{name: "closed", page: closed},
		{name: "refusal", err: &StatusError{Code: 999}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			site := &linkedInRelatedSite{listings: map[string]string{"0": linkedInRelatedListing("123")}, companion: tc.page, companionFinal: tc.final, companionError: tc.err}
			jobs := runRelatedTest(t, site)
			if len(jobs) != 1 || jobs[0].ExternalID != "123" || len(site.requests) != 4 {
				t.Fatalf("unverified page expanded or primary lost: jobs=%+v requests=%v", jobs, site.requests)
			}
		})
	}
}

func TestLinkedInRelatedRefusalsPreserveEarlierJobsAndStopTheBranch(t *testing.T) {
	cards := relatedTestCard("456", "https://www.linkedin.com/jobs/view/456/") + relatedTestCard("789", "https://www.linkedin.com/jobs/view/789/") + relatedTestCard("790", "https://www.linkedin.com/jobs/view/790/")
	for _, code := range []int{403, 429, 999} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			site := &linkedInRelatedSite{listings: map[string]string{"0": linkedInRelatedListing("123")}, companion: relatedTestPage(cards), detailError: map[string]error{"789": &StatusError{Code: code}}}
			jobs := runRelatedTest(t, site)
			// A 429 is retried before it counts as a refusal; 403 and 999 stop at once.
			want := 6
			if code == 429 {
				want += linkedInThrottleRetries
			}
			if len(jobs) != 2 || jobs[1].ExternalID != "456" || len(site.requests) != want {
				t.Fatalf("refusal discarded progress or fetched later cards: jobs=%+v requests=%v", jobs, site.requests)
			}
		})
	}
}

func TestLinkedInRelatedUnreadableDetailsStayUncertainAndClosedOnesDoNotPublish(t *testing.T) {
	cards := relatedTestCard("456", "https://www.linkedin.com/jobs/view/456/") + relatedTestCard("789", "https://www.linkedin.com/jobs/view/789/") + relatedTestCard("790", "https://www.linkedin.com/jobs/view/790/")
	closed := strings.Replace(linkedInRelatedPosting("789"), `<button data-tracking-control-name="public_jobs_apply-link-onsite">Apply</button>`, `<figcaption class="closed-job__flavor--closed">No longer accepting applications</figcaption>`, 1)
	site := &linkedInRelatedSite{listings: map[string]string{"0": linkedInRelatedListing("123")}, companion: relatedTestPage(cards), detailError: map[string]error{"456": &StatusError{Code: 404}}, detailPages: map[string]string{"789": closed}}
	jobs := runRelatedTest(t, site)
	if len(jobs) != 3 || jobs[1].ExternalID != "456" || !jobs[1].Unreadable || jobs[1].Removed || jobs[2].ExternalID != "790" {
		t.Fatalf("unreadable/closed related jobs misclassified: %+v", jobs)
	}
}

func TestLinkedInRelatedDoesNotExpandAfterAnIncompletePrimarySearch(t *testing.T) {
	site := &linkedInRelatedSite{listings: map[string]string{"0": linkedInRelatedListing("123")}, listingError: map[string]error{"1": context.DeadlineExceeded}, companion: relatedTestPage(relatedTestCard("456", "https://www.linkedin.com/jobs/view/456/"))}
	jobs := runRelatedTest(t, site)
	if len(jobs) != 1 || len(site.requests) != 3 {
		t.Fatalf("incomplete search was expanded: jobs=%+v requests=%v", jobs, site.requests)
	}
}

// A firehose search lists the whole market, newest first; one posting's recommendations add
// older, unrelated postings and up to 20 detail reads per search, so it skips them.
func TestLinkedInFirehoseSkipsRecommendations(t *testing.T) {
	site := &linkedInRelatedSite{listings: map[string]string{"0": linkedInRelatedListing("123")}, companion: relatedTestPage(relatedTestCard("456", "https://www.linkedin.com/jobs/view/456/"))}
	source := NewLinkedIn(site).(linkedIn)
	source.pace, source.backoff = &recordingWaiter{}, 0
	jobs, err := source.Fetch(context.Background(), CompanyEntry{Board: LinkedInAnyKeyword, Region: "Netherlands"})
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range site.requests {
		if strings.Contains(raw, "/jobs/view/") || strings.Contains(raw, "jobPosting/456") {
			t.Fatalf("firehose fetched recommendations: %v", site.requests)
		}
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "123" {
		t.Fatalf("firehose jobs=%+v", jobs)
	}
}
