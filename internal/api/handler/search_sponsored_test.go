package handler

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/engage/sponsored"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/search"
)

// adSearcher answers the organic search and, when the filter asks for sponsored = true, the
// paid one.
type adSearcher struct {
	organic, paid []search.JobDocument
	paidErr       error
	paidDelay     time.Duration // the paid lookup waits this long, or until its ctx ends
	paidCalls     int
	paidParams    search.SearchParams
}

func (f *adSearcher) Search(ctx context.Context, p search.SearchParams) (search.SearchResult, error) {
	if strings.Contains(fmt.Sprint(p.Filter), "sponsored = true") {
		select {
		case <-time.After(f.paidDelay):
		case <-ctx.Done():
			return search.SearchResult{}, ctx.Err()
		}
		f.paidCalls++
		f.paidParams = p
		return search.SearchResult{Hits: f.paid}, f.paidErr
	}
	return search.SearchResult{Hits: f.organic, Total: int64(len(f.organic))}, nil
}

type adServing map[int64]int64 // job id -> campaign id

func (s adServing) ListServingSponsoredCampaigns(_ context.Context, a db.ListServingSponsoredCampaignsParams) ([]db.ListServingSponsoredCampaignsRow, error) {
	var rows []db.ListServingSponsoredCampaignsRow
	for _, id := range a.JobIds {
		if c, ok := s[id]; ok {
			rows = append(rows, db.ListServingSponsoredCampaignsRow{JobID: id, CampaignID: c, CpcCents: 20})
		}
	}
	return rows, nil
}

var testClickKey = []byte("test-key")

func adApp(s searcher, serving search.ServingReader, key []byte, imp *sponsored.Impressions) *fiber.App {
	h := &searchHandlers{search: s, serving: serving, sponsoredKey: key, impressions: imp}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/jobs/search", newContract(contractHandlers{searchHandlers: h}))
	return app
}

func adDocs(ids ...int64) []search.JobDocument {
	out := make([]search.JobDocument, len(ids))
	for i, id := range ids {
		out[i] = search.JobDocument{ID: id, Job: jobview.Job{PublicSlug: fmt.Sprintf("job-%d", id), URL: fmt.Sprintf("https://x/%d", id)}}
	}
	return out
}

func adSlugs(body map[string]any) []string {
	var out []string
	for _, d := range body["data"].([]any) {
		out = append(out, d.(map[string]any)["public_slug"].(string))
	}
	return out
}

func TestSearchJobsPlacesSponsoredAtFirstAndSixth(t *testing.T) {
	s := &adSearcher{organic: adDocs(1, 2, 3, 4, 5, 6, 7, 8), paid: adDocs(50, 60)}
	app := adApp(s, adServing{50: 500, 60: 600}, testClickKey, nil)
	status, body := doGet(t, app, "/api/v1/jobs/search?q=go")
	if status != 200 {
		t.Fatalf("status %d", status)
	}
	want := []string{"job-50", "job-1", "job-2", "job-3", "job-4", "job-60", "job-5", "job-6", "job-7", "job-8"}
	if got := adSlugs(body); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("order = %v\nwant    %v", got, want)
	}
	first := body["data"].([]any)[0].(map[string]any)
	if first["sponsored"] != true || first["url"] != "https://x/50" {
		t.Errorf("sponsored view = %v", first)
	}
	url, _ := first["sponsored_url"].(string)
	claims, err := sponsored.VerifyToken(testClickKey, strings.TrimPrefix(url, sponsored.ClickPath), time.Now())
	if err != nil || claims.CampaignID != 500 || claims.JobID != 50 || claims.Placement != sponsored.PlacementSearch {
		t.Errorf("token claims = %+v, %v (url %q)", claims, err, url)
	}
	organic := body["data"].([]any)[1].(map[string]any)
	if _, ok := organic["sponsored"]; ok {
		t.Errorf("an organic hit carries the flag: %v", organic)
	}
	if s.paidParams.Limit != 2 || s.paidParams.Offset != 0 || s.paidParams.Sort[0] != "sponsored_cpc:desc" {
		t.Errorf("paid params = %+v", s.paidParams)
	}
}

func TestSearchJobsDedupesPromotedJobAndClampsSlots(t *testing.T) {
	s := &adSearcher{organic: adDocs(1, 2, 3), paid: adDocs(2, 90)}
	app := adApp(s, adServing{2: 20, 90: 900}, testClickKey, nil)
	_, body := doGet(t, app, "/api/v1/jobs/search?q=go")
	// Job 2 is promoted, so its organic copy goes; the second ad clamps to the end of a short page.
	want := []string{"job-2", "job-1", "job-3", "job-90"}
	if got := adSlugs(body); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

func TestSearchJobsPinsSponsoredOnCompanyPage(t *testing.T) {
	s := &adSearcher{organic: adDocs(1, 2, 3), paid: adDocs(50, 60)}
	app := adApp(s, adServing{50: 500, 60: 600}, testClickKey, nil)
	_, body := doGet(t, app, "/api/v1/jobs/search?company_slug=acme")
	if got := adSlugs(body); fmt.Sprint(got[:3]) != "[job-50 job-60 job-1]" {
		t.Fatalf("order = %v", got)
	}
	url := body["data"].([]any)[0].(map[string]any)["sponsored_url"].(string)
	c, err := sponsored.VerifyToken(testClickKey, strings.TrimPrefix(url, sponsored.ClickPath), time.Now())
	if err != nil || c.Placement != sponsored.PlacementCompany {
		t.Errorf("claims = %+v, %v", c, err)
	}
}

func TestSearchJobsNoSponsored(t *testing.T) {
	cases := []struct {
		name, url string
		key       []byte
	}{
		{"unfiltered feed", "/api/v1/jobs/search", testClickKey},
		{"later page", "/api/v1/jobs/search?q=go&offset=20", testClickKey},
		{"no click key", "/api/v1/jobs/search?q=go", nil},
		// The company OG image asks for one hit; a page that small has no room for paid slots.
		{"page smaller than the slots", "/api/v1/jobs/search?company_slug=co&limit=1", testClickKey},
	}
	for _, c := range cases {
		s := &adSearcher{organic: adDocs(1, 2), paid: adDocs(50)}
		app := adApp(s, adServing{50: 500}, c.key, nil)
		status, body := doGet(t, app, c.url)
		if status != 200 || fmt.Sprint(adSlugs(body)) != "[job-1 job-2]" || s.paidCalls != 0 {
			t.Errorf("%s: status=%d slugs=%v paidCalls=%d", c.name, status, adSlugs(body), s.paidCalls)
		}
	}
}

func TestSearchJobsDegradesWhenSponsoredFails(t *testing.T) {
	s := &adSearcher{organic: adDocs(1, 2), paid: adDocs(50), paidErr: errors.New("meili timeout")}
	app := adApp(s, adServing{50: 500}, testClickKey, nil)
	status, body := doGet(t, app, "/api/v1/jobs/search?q=go")
	if status != 200 || fmt.Sprint(adSlugs(body)) != "[job-1 job-2]" {
		t.Fatalf("status=%d slugs=%v", status, adSlugs(body))
	}
}

// A slow sponsored lookup costs the organic answer at most sponsoredLookupTimeout.
func TestSearchJobsDoesNotWaitLongForSlowSponsored(t *testing.T) {
	s := &adSearcher{organic: adDocs(1, 2), paid: adDocs(50), paidDelay: 10 * time.Second}
	app := adApp(s, adServing{50: 500}, testClickKey, nil)
	start := time.Now()
	status, body := doGet(t, app, "/api/v1/jobs/search?q=go")
	if took := time.Since(start); took > sponsoredLookupTimeout+time.Second {
		t.Fatalf("search took %v, want about %v", took, sponsoredLookupTimeout)
	}
	if status != 200 || fmt.Sprint(adSlugs(body)) != "[job-1 job-2]" {
		t.Fatalf("status=%d slugs=%v", status, adSlugs(body))
	}
}

func TestSearchJobsCountsImpressions(t *testing.T) {
	up := &impressionSink{}
	imp := sponsored.NewImpressions(up)
	s := &adSearcher{organic: adDocs(1, 2), paid: adDocs(50, 60)}
	app := adApp(s, adServing{50: 500, 60: 600}, testClickKey, imp)
	doGet(t, app, "/api/v1/jobs/search?q=go")
	if err := imp.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := map[int64]int64{}
	for _, r := range up.rows {
		got[r.CampaignID] += r.Impressions
	}
	if got[500] != 1 || got[600] != 1 || len(got) != 2 {
		t.Errorf("impressions = %v", got)
	}
}

type impressionSink struct {
	rows []db.UpsertSponsoredDailyParams
}

func (s *impressionSink) UpsertSponsoredDaily(_ context.Context, a db.UpsertSponsoredDailyParams) error {
	s.rows = append(s.rows, a)
	return nil
}
