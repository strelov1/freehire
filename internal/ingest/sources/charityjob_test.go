package sources

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// These captures are trimmed from CharityJob responses served to the crawl host. The listing
// fixtures keep the original cards and use a two-page pager with a repeated promoted card.
//
//go:embed testdata/charityjob
var charityjobFixtures embed.FS

const (
	charityjobOnsiteID = "1090300"
	charityjobHybridID = "1091785"
	charityjobRemoteID = "1090901"
)

type charityjobFake struct {
	t       *testing.T
	calls   []string
	fixture map[string]string
	err     map[string]error
}

func (f *charityjobFake) GetHTML(_ context.Context, pageURL string) (*html.Node, error) {
	f.calls = append(f.calls, pageURL)
	if err := f.err[pageURL]; err != nil {
		return nil, err
	}
	name, ok := f.fixture[pageURL]
	if !ok {
		return nil, fmt.Errorf("unexpected request: %s", pageURL)
	}
	content, err := charityjobFixtures.ReadFile("testdata/charityjob/" + name)
	if err != nil {
		f.t.Fatal(err)
	}
	return html.Parse(strings.NewReader(string(content)))
}

func newCharityJobFake(t *testing.T) *charityjobFake {
	t.Helper()
	return &charityjobFake{
		t: t,
		fixture: map[string]string{
			charityjobListingURL(1): "list-1.html",
			charityjobListingURL(2): "list-2.html",
			charityjobBase + "/jobs/firmfoundation/lead-project-worker/" + charityjobOnsiteID:                  "detail-1090300.html",
			charityjobBase + "/jobs/hestia/payroll-manager/" + charityjobHybridID:                              "detail-1091785.html",
			charityjobBase + "/jobs/back-up/regional-outreach-engagement-coordinator-ni/" + charityjobRemoteID: "detail-1090901.html",
		},
		err: map[string]error{},
	}
}

func TestCharityJobFetchMapsFacetsAndDeduplicates(t *testing.T) {
	fake := newCharityJobFake(t)
	jobs, err := NewCharityJob(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil || len(jobs) != 3 || len(fake.calls) != 5 {
		t.Fatalf("jobs = %v, requests = %v, error = %v", jobs, fake.calls, err)
	}
	for _, tc := range []struct {
		id, company, mode, title, posted, deadline string
		schedule                                   string
		salaryMin, salaryMax                       int
	}{
		{charityjobOnsiteID, "FirmFoundation", "onsite", "Lead Project Worker", "2026-11-09", "2026-12-09", "full_time", 31024, 31024},
		{charityjobHybridID, "Hestia", "hybrid", "Payroll Manager", "2026-11-22", "2026-12-02", "full_time", 47211, 47211},
		{charityjobRemoteID, "Back-Up", "remote", "Regional Outreach & Engagement Coordinator NI", "2026-11-13", "2026-11-30", "", 25500, 29500},
	} {
		index := slices.IndexFunc(jobs, func(j Job) bool { return j.ExternalID == tc.id })
		if index < 0 {
			t.Fatalf("missing listed job %s", tc.id)
		}
		j := jobs[index]
		if j.Company != tc.company || j.Title != tc.title || j.EmploymentType != tc.schedule ||
			j.WorkMode != tc.mode || j.Remote != (tc.mode == "remote") || !slices.Equal(j.Countries, []string{"gb"}) ||
			j.Description == "" || j.Unreadable || j.IsTechHint {
			t.Errorf("mapped posting = %+v", j)
		}
		if j.PostedAt == nil || j.PostedAt.Format("2006-01-02") != tc.posted ||
			j.ApplicationDeadline == nil || j.ApplicationDeadline.Format("2006-01-02") != tc.deadline {
			t.Errorf("posting %s posted %v, deadline %v", tc.id, j.PostedAt, j.ApplicationDeadline)
		}
		if j.SalaryMin == nil || *j.SalaryMin != tc.salaryMin || j.SalaryMax == nil || *j.SalaryMax != tc.salaryMax ||
			j.SalaryPeriod != "year" || j.SalaryCurrency != "GBP" {
			t.Errorf("posting %s salary = %v–%v %s per %s", tc.id, j.SalaryMin, j.SalaryMax, j.SalaryCurrency, j.SalaryPeriod)
		}
	}
}

func TestCharityJobUnreadableDetailsAreDistinctFromClosures(t *testing.T) {
	tests := []struct {
		name      string
		hybridErr error
		wantErr   bool
	}{
		{"partial outage", nil, false},
		{"confirmed closure", &StatusError{Code: http.StatusGone}, false},
		{"total outage", errors.New("down"), true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake := newCharityJobFake(t)
			// A valid but unrelated JobPosting is an unreadable response for this listing ID.
			onsiteURL := charityjobBase + "/jobs/firmfoundation/lead-project-worker/" + charityjobOnsiteID
			fake.fixture[onsiteURL] = "detail-1090901.html"
			hybridURL := charityjobBase + "/jobs/hestia/payroll-manager/" + charityjobHybridID
			fake.err[hybridURL] = tc.hybridErr
			jobs, err := NewCharityJob(fake).(HydratingSource).FetchNew(context.Background(), CompanyEntry{}, func(id string) bool {
				return id == charityjobRemoteID
			})
			if (err != nil) != tc.wantErr {
				t.Fatalf("jobs = %+v, error = %v", jobs, err)
			}
			if tc.wantErr {
				return
			}
			if !slices.ContainsFunc(jobs, func(j Job) bool {
				return j.ExternalID == charityjobOnsiteID && j.URL == onsiteURL && j.Company == "FirmFoundation" && j.Unreadable
			}) {
				t.Errorf("missing unreadable marker: %+v", jobs)
			}
			if !slices.ContainsFunc(jobs, func(j Job) bool { return j.ExternalID == charityjobRemoteID && j.SeenRefresh }) ||
				slices.Contains(fake.calls, charityjobBase+"/jobs/back-up/regional-outreach-engagement-coordinator-ni/"+charityjobRemoteID) {
				t.Errorf("missing identity-only refresh: jobs = %+v, requests = %v", jobs, fake.calls)
			}
			if slices.ContainsFunc(jobs, func(j Job) bool { return j.ExternalID == charityjobHybridID }) != (tc.hybridErr == nil) {
				t.Errorf("unexpected hybrid posting state: %+v", jobs)
			}
		})
	}
}

func TestCharityJobRejectsTruncatedWalk(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		file string
	}{
		{"later request failed", errors.New("timeout"), ""},
		{"wrong page", nil, "list-1.html"},
		{"successful response without a listing", nil, "detail-1090300.html"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := newCharityJobFake(t)
			fake.err[charityjobListingURL(2)] = tc.err
			fake.fixture[charityjobListingURL(2)] = tc.file
			jobs, err := NewCharityJob(fake).Fetch(context.Background(), CompanyEntry{})
			if err == nil || len(jobs) != 0 || !strings.Contains(err.Error(), "page 2") {
				t.Fatalf("jobs = %+v, error = %v, want failed listing", jobs, err)
			}
		})
	}
}

func TestCharityJobPostingLinkStaysOnItsOwnPublicPath(t *testing.T) {
	const own = "/jobs/firmfoundation/lead-project-worker/" + charityjobOnsiteID
	for _, tc := range []struct{ href, id, canonical string }{
		{own + "?source=search", charityjobOnsiteID, charityjobBase + own},
		{charityjobBase + own, charityjobOnsiteID, charityjobBase + own},
		{"https://example.org" + own, "", ""},
		{"/attachments/1", "", ""},
		{"/jobs/../feeds/1", "", ""},
		{"/jobs/%5C../feeds/1", "", ""},
		{"/jobs/firmfoundation/lead-project-worker/not-an-id", "", ""},
	} {
		id, canonical := charityjobPostingLink(tc.href)
		if id != tc.id || canonical != tc.canonical {
			t.Errorf("posting link %q = %q %q, want %q %q", tc.href, id, canonical, tc.id, tc.canonical)
		}
	}
}
