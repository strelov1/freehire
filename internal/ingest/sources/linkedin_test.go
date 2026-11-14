package sources

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

const linkedInOpenPage = `<html><head><meta property="og:url" content="https://www.linkedin.com/jobs/view/123/"></head><body>
<section class="top-card-layout"><h1 class="top-card-layout__title">Go &amp; API Engineer</h1>
<a class="topcard__org-name-link">Acme</a>
<span class="topcard__flavor topcard__flavor--bullet">London, England</span>
<span class="num-applicants__caption topcard__flavor--bullet">85 applicants</span>
<div class="show-more-less-html__markup"><p>Build <strong>APIs</strong>.</p><script>bad()</script></div>
<button data-modal="job-details-topcard-apply-modal">Apply</button></section></body></html>`

func TestLinkedInPostingEvidence(t *testing.T) {
	closed := strings.Replace(linkedInOpenPage, `<button data-modal="job-details-topcard-apply-modal">Apply</button>`, `<figcaption class="closed-job__flavor--closed">No longer accepting applications</figcaption>`, 1)
	for _, tc := range []struct {
		name, page, url  string
		wantErr, removed bool
	}{
		{"open", linkedInOpenPage, "https://www.linkedin.com/jobs/view/engineer-123/", false, false},
		{"closed", closed, "https://www.linkedin.com/jobs/view/123/", false, true},
		{"wrong canonical", strings.ReplaceAll(closed, "view/123/", "view/456/"), "https://www.linkedin.com/jobs/view/123/", true, false},
		{"authwall", closed, "https://www.linkedin.com/authwall", true, false},
		{"wrong host", closed, "https://example.org/jobs/view/123/", true, false},
		{"login page", `<h1>Sign in</h1>`, "https://www.linkedin.com/jobs/view/123/", true, false},
		{"no verdict", strings.Replace(linkedInOpenPage, `data-modal="job-details-topcard-apply-modal"`, "", 1), "https://www.linkedin.com/jobs/view/123/", true, false},
		{"recommendation closed card", strings.Replace(linkedInOpenPage, "</body>", `<aside><figcaption class="closed-job__flavor--closed">No longer accepting applications</figcaption></aside></body>`, 1), "https://www.linkedin.com/jobs/view/123/", false, false},
		{"conflicting signals", strings.Replace(linkedInOpenPage, "</section>", `<figcaption class="closed-job__flavor--closed">No longer accepting applications</figcaption></section>`, 1), "https://www.linkedin.com/jobs/view/123/", true, false},
		{"recommendation prose", strings.Replace(linkedInOpenPage, "</body>", "<aside>No longer accepting applications</aside></body>", 1), "https://www.linkedin.com/jobs/view/123/", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, _ := html.Parse(strings.NewReader(tc.page))
			j, err := LinkedInPosting(root, tc.url, "123")
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v, want error %v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if j.Removed != tc.removed || j.Title != "Go & API Engineer" || j.Company != "Acme" || j.Location != "London, England" || j.ExternalID != "123" || j.URL != "https://www.linkedin.com/jobs/view/123/" {
				t.Fatalf("posting: %+v", j)
			}
			if !strings.Contains(j.Description, "<strong>APIs</strong>") || strings.Contains(j.Description, "script") {
				t.Fatalf("unsafe or lost description: %s", j.Description)
			}
		})
	}
}

func TestLinkedInFetchHydratesAndRefreshes(t *testing.T) {
	listing := `<li data-entity-urn="urn:li:jobPosting:123"><a class="base-card__full-link" href="https://www.linkedin.com/jobs/view/engineer-123/"></a><h3 class="base-search-card__title">Go Engineer</h3><h4 class="base-search-card__subtitle">Acme</h4></li>`
	fake := (&routedHTTP{}).route("start=0", listing).route("start=1", listing).route("/jobs/view/123/", linkedInOpenPage)
	s := NewLinkedIn(fake).(linkedIn)
	s.pace = &recordingWaiter{}
	entry := CompanyEntry{Board: "Go engineer", Region: "United Kingdom"}
	jobs, err := s.FetchNew(context.Background(), entry, func(string) bool { return false })
	if err != nil || len(jobs) != 1 || jobs[0].Description == "" {
		t.Fatalf("FetchNew: %+v, %v", jobs, err)
	}
	fake = (&routedHTTP{}).route("start=0", listing).route("start=1", listing)
	s = NewLinkedIn(fake).(linkedIn)
	s.pace = &recordingWaiter{}
	jobs, err = s.FetchNew(context.Background(), entry, func(id string) bool { return id == "123" })
	if err != nil || len(jobs) != 1 || !jobs[0].SeenRefresh {
		t.Fatalf("refresh: %+v, %v", jobs, err)
	}
}

func TestLinkedInRefusesUnprovenListings(t *testing.T) {
	for _, body := range []string{"", "<h1>Sign in</h1>", `<li data-entity-urn="urn:li:jobPosting:123"></li>`} {
		fake := (&routedHTTP{}).route("start=0", body)
		if _, err := NewLinkedIn(fake).Fetch(context.Background(), CompanyEntry{Board: "Go", Region: "London"}); err == nil {
			t.Fatalf("accepted unproven listing %q", body)
		}
	}
	if _, err := NewLinkedIn(nil).Fetch(context.Background(), CompanyEntry{Board: "Go"}); err == nil {
		t.Fatal("missing location accepted")
	}
}

func TestLinkedInCannotCloseFromSearchDisappearance(t *testing.T) {
	reg := All(nil)
	if reg["linkedin"] == nil || !slices.Contains(FilterableProviders(), "linkedin") {
		t.Fatal("LinkedIn not registered as aggregator")
	}
	if !slices.Contains(UnseenSweepExemptProviders(reg), "linkedin") {
		t.Fatal("bounded search must not close unseen jobs")
	}
	if _, ok := UnseenWindows(reg, 1)["linkedin"]; ok {
		t.Fatal("live jobs may age out of search")
	}
	if SelfCloses(reg["linkedin"]) {
		t.Fatal("LinkedIn is not a change stream")
	}
}

func TestLinkedInDetailFailuresPreserveUncertainty(t *testing.T) {
	listing := `<div data-entity-urn="urn:li:jobPosting:123"><h3 class="base-search-card__title">Engineer</h3><h4 class="base-search-card__subtitle">Acme</h4></div>`
	for _, tc := range []struct {
		name, detail   string
		err            error
		wantUnreadable bool
	}{
		{"login", "<h1>Sign in</h1>", nil, true},
		{"missing", "", &StatusError{Code: 404}, true},
		{"timeout", "", context.DeadlineExceeded, true},
		{"refused", "", &StatusError{Code: 429}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := (&routedHTTP{}).route("start=0", listing).route("start=1", listing).route("/jobs/view/123/", tc.detail)
			if tc.err != nil {
				fake.routeErr("/jobs/view/123/", tc.err)
			}
			s := NewLinkedIn(fake).(linkedIn)
			s.pace = &recordingWaiter{}
			jobs, err := s.Fetch(context.Background(), CompanyEntry{Board: "Go", Region: "London"})
			if err == nil {
				t.Fatal("all-unreadable crawl reported success")
			}
			if tc.wantUnreadable && (len(jobs) != 1 || !jobs[0].Unreadable || jobs[0].Removed || jobs[0].SeenRefresh) {
				t.Fatalf("expected only an unreadable marker: %+v", jobs)
			}
			if tc.err != nil && !tc.wantUnreadable && !errors.Is(err, tc.err) {
				t.Fatalf("lost refusal: %v", err)
			}
		})
	}
}

type linkedInGetterFunc func(context.Context, string) (*html.Node, string, error)

func (f linkedInGetterFunc) GetHTMLResolved(ctx context.Context, raw string) (*html.Node, string, error) {
	return f(ctx, raw)
}

func TestLinkedInBoundsAndScopesItsSearch(t *testing.T) {
	pages := 0
	getter := linkedInGetterFunc(func(_ context.Context, raw string) (*html.Node, string, error) {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		q := u.Query()
		if q.Get("keywords") != "Go & API" || q.Get("location") != "United Kingdom" || q.Get("f_TPR") != "r604800" || q.Get("start") != strconv.Itoa(pages) {
			t.Fatalf("unscoped or incorrect search: %s", raw)
		}
		pages++
		body := fmt.Sprintf(`<div data-entity-urn="urn:li:jobPosting:%d"><h3 class="base-search-card__title">Engineer</h3><h4 class="base-search-card__subtitle">Acme</h4></div>`, 100+pages)
		root, err := html.Parse(strings.NewReader(body))
		return root, raw, err
	})
	s := NewLinkedIn(getter).(linkedIn)
	gate := &recordingWaiter{}
	s.pace = gate
	jobs, err := s.FetchNew(context.Background(), CompanyEntry{Board: "Go & API", Region: "United Kingdom"}, func(string) bool { return true })
	if err != nil || len(jobs) != 10 || pages != 10 || gate.calls != 10 {
		t.Fatalf("unbounded crawl or ungated request: jobs=%d pages=%d pace=%d err=%v", len(jobs), pages, gate.calls, err)
	}
}

func TestLinkedInPostingIDRejectsUnrelatedURLs(t *testing.T) {
	for _, raw := range []string{"https://linkedin.com.evil.test/jobs/view/123/", "https://www.linkedin.com/authwall", "https://www.linkedin.com/jobs/view/", "https://www.linkedin.com/jobs/view/123/other", "http://www.linkedin.com/jobs/view/123/"} {
		if got := LinkedInPostingID(raw); got != "" {
			t.Errorf("%q identified as %q", raw, got)
		}
	}
	if got := LinkedInPostingID("https://uk.linkedin.com/jobs/view/go-engineer-123/?trk=x"); got != "123" {
		t.Fatalf("native ID=%q", got)
	}
}

func TestLinkedInPreservesProgressWhenLaterRequestsFail(t *testing.T) {
	card := func(id string) string {
		return `<div data-entity-urn="urn:li:jobPosting:` + id + `"><h3 class="base-search-card__title">Engineer</h3><h4 class="base-search-card__subtitle">Acme</h4></div>`
	}
	for _, refused := range []bool{false, true} {
		t.Run(fmt.Sprintf("detail_refused=%v", refused), func(t *testing.T) {
			listing := card("123")
			if refused {
				listing += card("124")
			}
			fake := (&routedHTTP{}).route("start=0", listing).route("/jobs/view/123/", linkedInOpenPage)
			if refused {
				fake.routeErr("/jobs/view/124/", &StatusError{Code: 429})
			} else {
				fake.routeErr("start=1", context.DeadlineExceeded)
			}
			s := NewLinkedIn(fake).(linkedIn)
			s.pace = &recordingWaiter{}
			jobs, err := s.Fetch(context.Background(), CompanyEntry{Board: "Go", Region: "London"})
			if err != nil || len(jobs) != 1 || jobs[0].ExternalID != "123" || jobs[0].Description == "" {
				t.Fatalf("lost readable posting: jobs=%+v error=%v", jobs, err)
			}
		})
	}
}
