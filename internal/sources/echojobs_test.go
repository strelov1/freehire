package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"testing"
	"time"
)

// echojobsHTTP is a paging-and-detail-aware test JSONGetter: it serves pages[i] for ?page=i+1
// (recording every page number requested) and, for a detail request (/api/jobs/{id}), routes by
// the id in the path — mirroring getmanfredHTTP's list/detail split.
type echojobsHTTP struct {
	pages      []string
	failPage   int // 0 = never fail
	got        []int
	details    map[string]string
	detailErr  map[string]bool
	gotDetails []string
}

var echojobsDetailRE = regexp.MustCompile(`/jobs/([^/?]+)$`)

func (f *echojobsHTTP) GetJSON(_ context.Context, u string, v any) error {
	parsed, err := url.Parse(u)
	if err != nil {
		return err
	}
	if parsed.RawQuery == "" {
		if m := echojobsDetailRE.FindStringSubmatch(parsed.Path); m != nil {
			id := m[1]
			f.gotDetails = append(f.gotDetails, id)
			if f.detailErr[id] {
				return errors.New("echojobsHTTP: detail boom")
			}
			raw, ok := f.details[id]
			if !ok {
				return errors.New("echojobsHTTP: no detail for id")
			}
			return json.Unmarshal([]byte(raw), v)
		}
	}
	page, _ := strconv.Atoi(parsed.Query().Get("page"))
	f.got = append(f.got, page)
	if f.failPage != 0 && page == f.failPage {
		return errors.New("echojobsHTTP: boom")
	}
	if page < 1 || page > len(f.pages) {
		return fmt.Errorf("echojobsHTTP: no page %d", page)
	}
	return json.Unmarshal([]byte(f.pages[page-1]), v)
}

func echojobsPageJSON(jobs string) string {
	return fmt.Sprintf(`{"found":999,"page":1,"per_page":100,"jobs":[%s]}`, jobs)
}

func echojobsJobJSON(handle, postedAt string) string {
	return echojobsJobJSONWithID(handle, handle, postedAt)
}

func echojobsJobJSONWithID(id, handle, postedAt string) string {
	return fmt.Sprintf(`{
		"id":%q,"title":"Backend Engineer","company_name":"Acme","domain_name":"acme.com",
		"url":"https://boards.greenhouse.io/acme/jobs/123","job_handle":%q,
		"posted_at":%s,"locations":["California","New York"],"remote_type":"hybrid",
		"required_skills":["Go","NotARealSkill"]
	}`, id, handle, postedAt)
}

func TestEchojobsFetchMapsFields(t *testing.T) {
	now := time.Now().UTC()
	http := &echojobsHTTP{pages: []string{
		echojobsPageJSON(echojobsJobJSON("acme-swe-abc12", strconv.FormatInt(now.UnixMilli(), 10))),
		echojobsPageJSON(""), // empty page: the walk ends cleanly here, not on an error
	}}

	jobs, err := echojobs{http: http}.Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("want 1 job, got %d", len(jobs))
	}
	job := jobs[0]
	if job.ExternalID != "acme-swe-abc12" {
		t.Fatalf("ExternalID: %q", job.ExternalID)
	}
	if job.URL != "https://boards.greenhouse.io/acme/jobs/123" {
		t.Fatalf("URL should be the upstream ATS link, got %q", job.URL)
	}
	if job.Title != "Backend Engineer" || job.Company != "Acme" {
		t.Fatalf("identity mismatch: %+v", job)
	}
	if job.Location != "California; New York" {
		t.Fatalf("Location: %q", job.Location)
	}
	if job.WorkMode != "hybrid" || job.Remote {
		t.Fatalf("hybrid should not be Remote: mode=%q remote=%v", job.WorkMode, job.Remote)
	}
	if job.PostedAt == nil {
		t.Fatalf("PostedAt not parsed")
	}
	if !slices.Contains(job.Skills, "go") || slices.Contains(job.Skills, "NotARealSkill") {
		t.Fatalf("Skills: %v", job.Skills)
	}
}

// Pagination keeps walking while a page's postings are within the freshness window, and stops as
// soon as a page's LAST (oldest, since the feed is newest-first) item falls outside it — the
// earlier, still-fresh items on that same page are kept.
func TestEchojobsFetchStopsAtFreshnessWindow(t *testing.T) {
	now := time.Now().UTC()
	fresh := strconv.FormatInt(now.UnixMilli(), 10)
	stillFresh := strconv.FormatInt(now.Add(-10*24*time.Hour).UnixMilli(), 10)
	stale := strconv.FormatInt(now.Add(-20*24*time.Hour).UnixMilli(), 10)

	page1 := echojobsPageJSON(echojobsJobJSON("job-1", fresh) + "," + echojobsJobJSON("job-2", fresh))
	page2 := echojobsPageJSON(echojobsJobJSON("job-3", stillFresh) + "," + echojobsJobJSON("job-4", stale))
	http := &echojobsHTTP{pages: []string{page1, page2, echojobsPageJSON(echojobsJobJSON("job-5", fresh))}}

	jobs, err := echojobs{http: http}.Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	var ids []string
	for _, j := range jobs {
		ids = append(ids, j.ExternalID)
	}
	if !slices.Equal(ids, []string{"job-1", "job-2", "job-3"}) {
		t.Fatalf("want job-1,job-2,job-3 (job-4 stale, job-5 never reached), got %v", ids)
	}
	if !slices.Equal(http.got, []int{1, 2}) {
		t.Fatalf("page 3 should never be requested once page 2's last item is stale, got requests %v", http.got)
	}
}

// A failed first page is a board-level error.
func TestEchojobsFetchFirstPageFailure(t *testing.T) {
	http := &echojobsHTTP{failPage: 1, pages: []string{echojobsPageJSON("")}}
	if _, err := (echojobs{http: http}).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("want error on first-page failure")
	}
}

// A later page failing ends the walk with what was gathered so far, per the house pagination rule.
func TestEchojobsFetchLaterPageFailureReturnsPartial(t *testing.T) {
	now := time.Now().UTC()
	fresh := strconv.FormatInt(now.UnixMilli(), 10)
	page1 := echojobsPageJSON(echojobsJobJSON("job-1", fresh))
	http := &echojobsHTTP{pages: []string{page1, echojobsPageJSON(echojobsJobJSON("job-2", fresh))}, failPage: 2}

	jobs, err := echojobs{http: http}.Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "job-1" {
		t.Fatalf("want partial result [job-1], got %+v", jobs)
	}
}

// The list endpoint carries no description (verified live), so FetchNew hydrates it from the
// per-posting detail endpoint — but only for a posting the catalogue does not already have.
func TestEchojobsFetchNewHydratesUnseenPosting(t *testing.T) {
	now := time.Now().UTC()
	fresh := strconv.FormatInt(now.UnixMilli(), 10)
	http := &echojobsHTTP{
		pages:   []string{echojobsPageJSON(echojobsJobJSONWithID("det1", "acme-swe", fresh)), echojobsPageJSON("")},
		details: map[string]string{"det1": `{"description":"<p>Great role.</p>"}`},
	}

	jobs, err := echojobs{http: http}.FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("want 1 job, got %d", len(jobs))
	}
	if jobs[0].Description != "<p>Great role.</p>" {
		t.Fatalf("Description not hydrated: %q", jobs[0].Description)
	}
	if !slices.Equal(http.gotDetails, []string{"det1"}) {
		t.Fatalf("detail requests = %v, want [det1]", http.gotDetails)
	}
}

// A posting the catalogue already has must not cost a detail request: it is refreshed
// (SeenRefresh) with its list-only identity, preserving whatever description was hydrated when
// it was new — a content-less re-upsert would wipe it.
func TestEchojobsFetchNewSkipsDetailForSeenPosting(t *testing.T) {
	now := time.Now().UTC()
	fresh := strconv.FormatInt(now.UnixMilli(), 10)
	http := &echojobsHTTP{
		pages: []string{echojobsPageJSON(echojobsJobJSONWithID("det1", "acme-swe", fresh)), echojobsPageJSON("")},
	}

	jobs, err := echojobs{http: http}.FetchNew(context.Background(), CompanyEntry{}, func(externalID string) bool {
		return externalID == "acme-swe"
	})
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 1 || !jobs[0].SeenRefresh {
		t.Fatalf("want 1 job with SeenRefresh set, got %+v", jobs)
	}
	if len(http.gotDetails) != 0 {
		t.Fatalf("a seen posting must not cost a detail request, got %v", http.gotDetails)
	}
}

// A failed detail request must not drop the posting — it falls back to the list-only job.
func TestEchojobsFetchNewDetailFailureKeepsPosting(t *testing.T) {
	now := time.Now().UTC()
	fresh := strconv.FormatInt(now.UnixMilli(), 10)
	http := &echojobsHTTP{
		pages:     []string{echojobsPageJSON(echojobsJobJSONWithID("det1", "acme-swe", fresh)), echojobsPageJSON("")},
		detailErr: map[string]bool{"det1": true},
	}

	jobs, err := echojobs{http: http}.FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 1 || jobs[0].Description != "" || jobs[0].ExternalID != "acme-swe" {
		t.Fatalf("posting should survive a failed detail request: %+v", jobs)
	}
}
