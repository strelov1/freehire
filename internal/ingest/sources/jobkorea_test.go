package sources

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/html"
)

//go:embed testdata/jobkorea
var jobkoreaFixtures embed.FS

// jobkoreaFake serves the live fixtures (fetched 2026-11-22: trimmed sitemaps, the JobPosting
// block of two postings and their body frames). 50342033 has a text body, 50398006 a body that
// is one image. An id with no fixture answers 404.
type jobkoreaFake struct {
	pageErr   error
	frameErr  error
	frameHTML string

	mu   sync.Mutex
	gets []string
}

func jobkoreaFixture(name string) []byte {
	raw, err := jobkoreaFixtures.ReadFile("testdata/jobkorea/" + name)
	if err != nil {
		panic(err)
	}
	return raw
}

func (f *jobkoreaFake) GetStream(_ context.Context, u string, accept string, fn func(io.Reader) error) error {
	if !strings.Contains(accept, "text/xml") {
		return fmt.Errorf("JobKorea sitemap requires accepting text/xml, got %q", accept)
	}
	names := map[string]string{
		jobkoreaSitemapIndex: "index.xml",
		jobkoreaBase + "/content/sitemapindex/agi/agi_0001.xml": "agi1.xml",
		jobkoreaBase + "/content/sitemapindex/agi/agi_0002.xml": "agi2.xml",
	}
	name, ok := names[u]
	if !ok {
		return &StatusError{Method: "GET", Code: 404, URL: u}
	}
	return fn(bytes.NewReader(jobkoreaFixture(name)))
}

func (f *jobkoreaFake) GetHTML(_ context.Context, u string) (*html.Node, error) {
	f.mu.Lock()
	f.gets = append(f.gets, u)
	f.mu.Unlock()
	var id int
	var kind, tail string
	if strings.Contains(u, "GI_Read_Comt_Ifrm") {
		if f.frameErr != nil {
			return nil, f.frameErr
		}
		if f.frameHTML != "" {
			return html.Parse(strings.NewReader(f.frameHTML))
		}
		kind, tail = "frame", u[strings.Index(u, "Gno=")+4:]
	} else {
		if f.pageErr != nil {
			return nil, f.pageErr
		}
		kind, tail = "page", u[strings.LastIndex(u, "/")+1:]
	}
	if _, err := fmt.Sscanf(tail, "%d", &id); err != nil {
		return nil, err
	}
	raw, err := jobkoreaFixtures.ReadFile(fmt.Sprintf("testdata/jobkorea/%s_%d.html", kind, id))
	if err != nil {
		return nil, &StatusError{Method: "GET", Code: 404, URL: u}
	}
	return html.Parse(bytes.NewReader(raw))
}

func newJobKoreaForTest(f jobkoreaHTTP) jobkorea {
	s := NewJobKorea(f).(jobkorea)
	s.limiter = noWait{}
	s.retryBackoff = time.Millisecond
	s.now = func() time.Time { return time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC) }
	return s
}

func jobkoreaFetchAll(t *testing.T, s jobkorea, seen func(string) bool) ([]Job, error) {
	t.Helper()
	return s.FetchNew(context.Background(), CompanyEntry{}, seen)
}

func TestJobKoreaIsABoardlessAggregator(t *testing.T) {
	s := NewJobKorea(nil)
	if s.Provider() != "jobkorea" {
		t.Errorf("Provider = %q", s.Provider())
	}
	if _, ok := s.(interface{ Boardless() }); !ok {
		t.Error("jobkorea must be boardless: the sitemaps name the whole catalogue")
	}
	if _, ok := s.(interface{ Aggregator() }); !ok {
		t.Error("jobkorea must be an aggregator: every posting names its own employer")
	}
	if _, ok := s.(HydratingSource); !ok {
		t.Error("jobkorea must hydrate: the sitemaps carry no body")
	}
}

func TestJobKoreaMapsAPostingWithATextBody(t *testing.T) {
	jobs, err := jobkoreaFetchAll(t, newJobKoreaForTest(&jobkoreaFake{}), func(string) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want the 2 distinct postings of the sitemaps: %+v", len(jobs), jobs)
	}
	// Newest id first.
	if jobs[0].ExternalID != "50398006" {
		t.Fatalf("first job = %s, want the newest id", jobs[0].ExternalID)
	}
	j := jobs[1]
	if j.ExternalID != "50342033" {
		t.Fatalf("second job = %s", j.ExternalID)
	}
	if j.Title != "[관세법인에이원] 재경팀 채용 (3년 미만, 주임)" || j.Company != "관세법인에이원" {
		t.Errorf("title/company = %q / %q", j.Title, j.Company)
	}
	if j.URL != "https://www.jobkorea.co.kr/Recruit/GI_Read/50342033" {
		t.Errorf("URL = %q", j.URL)
	}
	if !strings.Contains(j.Location, "강남구") {
		t.Errorf("Location = %q", j.Location)
	}
	if !strings.Contains(j.Description, "포지션 및 자격요건") {
		t.Errorf("Description is not the poster's body: %q", j.Description)
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q", j.EmploymentType)
	}
	if j.SalaryMin == nil || *j.SalaryMin != 30000000 || *j.SalaryMax != 35010000 ||
		j.SalaryCurrency != "KRW" || j.SalaryPeriod != "year" {
		t.Errorf("salary = %v-%v %s/%s", j.SalaryMin, j.SalaryMax, j.SalaryCurrency, j.SalaryPeriod)
	}
	if j.PostedAt == nil || j.PostedAt.Format("2006-01-02") != "2026-11-10" {
		t.Errorf("PostedAt = %v", j.PostedAt)
	}
	if j.ApplicationDeadline == nil || !j.ApplicationDeadline.Equal(time.Date(2026, 12, 10, 14, 59, 0, 0, time.UTC)) {
		t.Errorf("ApplicationDeadline = %v, want 2026-12-10 23:59 KST", j.ApplicationDeadline)
	}
	if len(j.FallbackCountries) != 1 || j.FallbackCountries[0] != "kr" {
		t.Errorf("FallbackCountries = %v", j.FallbackCountries)
	}
	if j.IsTechHint {
		t.Error("the site states no technical signal; no tech hint")
	}
}

func TestJobKoreaKeepsTheSummaryWhenTheBodyIsOneImage(t *testing.T) {
	jobs, err := jobkoreaFetchAll(t, newJobKoreaForTest(&jobkoreaFake{}), func(string) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	j := jobs[0]
	if j.ExternalID != "50398006" {
		t.Fatalf("unexpected order: %s", j.ExternalID)
	}
	if !strings.Contains(j.Description, "정규직 경력 채용을 진행합니다") {
		t.Errorf("an image-only body must leave the JobPosting summary, got %q", j.Description)
	}
	if j.SalaryMin != nil {
		t.Error("a posting that states no salary must carry none")
	}
}

func TestJobKoreaRefreshesStoredPostingsWithoutRequests(t *testing.T) {
	f := &jobkoreaFake{}
	jobs, err := jobkoreaFetchAll(t, newJobKoreaForTest(f), func(string) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs", len(jobs))
	}
	for _, j := range jobs {
		if !j.SeenRefresh || j.Title != "" {
			t.Errorf("stored posting %s must be a liveness refresh: %+v", j.ExternalID, j)
		}
	}
	if len(f.gets) != 0 {
		t.Errorf("no page may be read for a stored posting, got %v", f.gets)
	}
}

func TestJobKoreaKeepsDateOnlyDeadlineThroughItsKoreanCalendarDay(t *testing.T) {
	var ld jobkoreaLD
	ld.Title = "Engineer"
	ld.HiringOrganization.Name = "Acme"
	ld.ValidThrough = "2026-12-10"
	posting := jobkoreaPosting{id: 1}
	now := time.Date(2026, 12, 10, 12, 0, 0, 0, jobkoreaZone)
	job, ok := jobkoreaJob(ld, posting, now)
	if !ok || job.ApplicationDeadline == nil || job.ApplicationDeadline.Format(time.DateOnly) != ld.ValidThrough {
		t.Fatalf("posting should be open with deadline %s, got %v (open=%v)", ld.ValidThrough, job.ApplicationDeadline, ok)
	}
	if _, ok := jobkoreaJob(ld, posting, now.AddDate(0, 0, 1)); ok {
		t.Fatal("posting remained open beyond its deadline day")
	}
}

func TestJobKoreaBoundsNewPostingsPerRun(t *testing.T) {
	s := newJobKoreaForTest(&jobkoreaFake{})
	s.maxNewPerRun = 1
	jobs, err := jobkoreaFetchAll(t, s, func(string) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "50398006" {
		t.Fatalf("want only the newest posting read, got %+v", jobs)
	}
}

func TestJobKoreaBoundedRunsReachOlderPostingsBehindExpiredOnes(t *testing.T) {
	f := &jobkoreaFake{}
	s := newJobKoreaForTest(f)
	s.maxNewPerRun = 1
	now := time.Date(2026, 12, 5, 0, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }

	jobs, err := s.Fetch(context.Background(), CompanyEntry{})
	if err != nil || len(jobs) != 0 || len(f.gets) != 1 || f.gets[0] != "https://www.jobkorea.co.kr/Recruit/GI_Read/50398006" {
		t.Fatalf("first run should read only the expired page: jobs=%+v requests=%v err=%v", jobs, f.gets, err)
	}

	f.gets = nil
	now = now.Add(time.Hour)
	jobs, err = s.Fetch(context.Background(), CompanyEntry{})
	if err != nil || len(jobs) != 1 || jobs[0].ExternalID != "50342033" || len(f.gets) != 2 {
		t.Fatalf("next run should hydrate only the older posting and its body: jobs=%+v requests=%v err=%v", jobs, f.gets, err)
	}
}

func TestJobKoreaSkipsAnExpiredPosting(t *testing.T) {
	s := newJobKoreaForTest(&jobkoreaFake{})
	s.now = func() time.Time { return time.Date(2026, 12, 5, 0, 0, 0, 0, time.UTC) }
	jobs, err := jobkoreaFetchAll(t, s, func(string) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	// 50398006 closed 2026-11-30; 50342033 runs to 2026-12-10.
	if len(jobs) != 1 || jobs[0].ExternalID != "50342033" {
		t.Fatalf("an expired posting must not be stored: %+v", jobs)
	}
}

func TestJobKoreaDefersAPostingWhoseFrameFails(t *testing.T) {
	f := &jobkoreaFake{frameErr: &StatusError{Method: "GET", Code: 503, URL: "frame"}}
	jobs, err := jobkoreaFetchAll(t, newJobKoreaForTest(f), func(string) bool { return false })
	if err == nil {
		t.Fatalf("every posting failed to read its body; the crawl must fail, got %+v", jobs)
	}
}

func TestJobKoreaKeepsUnreadableIdentityWhenAnotherPostingExpired(t *testing.T) {
	f := &jobkoreaFake{frameErr: &StatusError{Method: "GET", Code: 503, URL: "frame"}}
	s := newJobKoreaForTest(f)
	s.now = func() time.Time { return time.Date(2026, 12, 5, 0, 0, 0, 0, time.UTC) }
	jobs, err := s.Fetch(context.Background(), CompanyEntry{})
	if err != nil || len(jobs) != 1 {
		t.Fatalf("got %d jobs and %v, want only the posting whose frame was unreadable", len(jobs), err)
	}
	j := jobs[0]
	if j.ExternalID != "50342033" || j.URL != "https://www.jobkorea.co.kr/Recruit/GI_Read/50342033" ||
		!j.Unreadable || j.SeenRefresh {
		t.Fatalf("unexpected unreadable marker: %+v", j)
	}
}

func TestJobKoreaDefersAPostingWhoseFrameLacksContent(t *testing.T) {
	f := &jobkoreaFake{frameHTML: "<html><body>Service temporarily unavailable</body></html>"}
	jobs, err := jobkoreaFetchAll(t, newJobKoreaForTest(f), func(string) bool { return false })
	if err == nil || len(jobs) != 0 {
		t.Fatalf("unrecognized body frames should defer both postings, got %d jobs and error %v", len(jobs), err)
	}
}

func TestJobKoreaStoresTheSummaryWhenTheFrameIsGone(t *testing.T) {
	f := &jobkoreaFake{frameErr: &StatusError{Method: "GET", Code: 404, URL: "frame"}}
	jobs, err := jobkoreaFetchAll(t, newJobKoreaForTest(f), func(string) bool { return false })
	if err != nil || len(jobs) != 2 {
		t.Fatalf("jobs=%d err=%v", len(jobs), err)
	}
}

func TestJobKoreaFailsWhenNoPageAnswers(t *testing.T) {
	s := newJobKoreaForTest(&jobkoreaFake{pageErr: errors.New("boom")})
	if _, err := jobkoreaFetchAll(t, s, func(string) bool { return false }); err == nil {
		t.Fatal("a crawl that read no page must fail")
	}
}

func TestJobKoreaGonePostingIsNotAnError(t *testing.T) {
	s := newJobKoreaForTest(&jobkoreaFake{pageErr: &StatusError{Method: "GET", Code: 404, URL: "page"}})
	jobs, err := jobkoreaFetchAll(t, s, func(string) bool { return false })
	if err != nil || len(jobs) != 0 {
		t.Fatalf("jobs=%d err=%v; a 404 is the site saying the posting is gone", len(jobs), err)
	}
}

type emptyIndexJobKorea struct{ jobkoreaFake }

func (*emptyIndexJobKorea) GetStream(_ context.Context, _ string, _ string, fn func(io.Reader) error) error {
	return fn(strings.NewReader(`<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"><sitemap><loc>https://www.jobkorea.co.kr/content/sitemapindex/qna/qstnno_001.xml</loc></sitemap></sitemapindex>`))
}

func TestJobKoreaListFailsWithoutPostingFiles(t *testing.T) {
	s := newJobKoreaForTest(&emptyIndexJobKorea{})
	if _, err := s.list(context.Background()); err == nil {
		t.Fatal("an index naming no posting file must fail the crawl")
	}
}
