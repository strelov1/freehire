package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// gqlHTTP is a body-aware test JSONPoster for the GraphQL adapters: Gem sends both its
// list and its detail request as POST to the same URL, distinguished only by the request
// body, so this fake routes the canned response on the operation name and (for detail)
// the extId in the variables — not the URL. A detail extId listed in failExtIDs returns
// an error so detail-isolation can be exercised.
type gqlHTTP struct {
	list       string            // canned JobBoardList response
	detail     map[string]string // extId -> canned ExternalJobPostingQuery response
	failExtIDs map[string]bool   // extIds whose detail request errors
	// The adapter fans its detail fetches out across a worker pool, so this recorder is
	// written from several goroutines at once and needs the lock. The assertions read it
	// after Fetch has joined the pool, which is already ordered.
	mu     sync.Mutex
	gotURL string
}

func (f *gqlHTTP) PostJSON(_ context.Context, url string, body, v any) error {
	f.mu.Lock()
	f.gotURL = url
	f.mu.Unlock()
	req, ok := body.(gemRequest)
	if !ok {
		return errors.New("gqlHTTP: body is not a gemRequest")
	}
	switch req.OperationName {
	case "JobBoardList":
		return json.Unmarshal([]byte(f.list), v)
	case "ExternalJobPostingQuery":
		ext, _ := req.Variables["extId"].(string)
		if f.failExtIDs[ext] {
			return errors.New("gqlHTTP: detail boom for " + ext)
		}
		raw, ok := f.detail[ext]
		if !ok {
			return errors.New("gqlHTTP: no canned detail for " + ext)
		}
		return json.Unmarshal([]byte(raw), v)
	default:
		return errors.New("gqlHTTP: unknown operation " + req.OperationName)
	}
}

// gemListResp builds a JobBoardList response from inline posting fragments.
func gemListResp(postings ...string) string {
	return `{"data":{"oatsExternalJobPostings":{"jobPostings":[` + strings.Join(postings, ",") + `]}}}`
}

func gemDetailResp(descHTML string, firstPublishedTsSec int64) string {
	desc, _ := json.Marshal(descHTML)
	return fmt.Sprintf(`{"data":{"oatsExternalJobPosting":{"descriptionHtml":%s,"firstPublishedTsSec":%d}}}`,
		desc, firstPublishedTsSec)
}

func TestGemProvider(t *testing.T) {
	if got := NewGem(nil).Provider(); got != "gem" {
		t.Errorf("Provider() = %q, want %q", got, "gem")
	}
}

func TestGemFetchListsThenFetchesDetailAndMaps(t *testing.T) {
	fake := &gqlHTTP{
		list: gemListResp(
			`{"extId":"X1","title":"Backend Engineer","locations":[{"city":"San Diego","isoCountry":"USA","isRemote":false}],"job":{"locationType":"IN_OFFICE"}}`,
			`{"extId":"X2","title":"Remote SRE","locations":[{"city":"","isoCountry":"COL","isRemote":true}],"job":{"locationType":"REMOTE"}}`,
		),
		detail: map[string]string{
			"X1": gemDetailResp(`<h2>Role</h2><p>Build it.</p><script>alert(1)</script>`, 1775770388),
			"X2": gemDetailResp(`<p>Keep it up.</p>`, 0),
		},
	}

	jobs, err := NewGem(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Cadre AI", Provider: "gem", Board: "go-cadre",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2", len(jobs))
	}

	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	x1 := byID["X1"]
	if x1.Title != "Backend Engineer" {
		t.Errorf("X1 Title = %q", x1.Title)
	}
	if x1.Company != "Cadre AI" {
		t.Errorf("X1 Company = %q", x1.Company)
	}
	if want := "https://jobs.gem.com/go-cadre/X1"; x1.URL != want {
		t.Errorf("X1 URL = %q, want %q", x1.URL, want)
	}
	if want := "San Diego, USA"; x1.Location != want {
		t.Errorf("X1 Location = %q, want %q", x1.Location, want)
	}
	if strings.Contains(x1.Description, "<script>") || !strings.Contains(x1.Description, "<h2>Role</h2>") {
		t.Errorf("X1 Description not sanitized/assembled: %q", x1.Description)
	}
	if x1.Remote {
		t.Errorf("X1 Remote = true, want false")
	}
	if x1.PostedAt == nil || !x1.PostedAt.Equal(time.Unix(1775770388, 0).UTC()) {
		t.Errorf("X1 PostedAt = %v, want 2026-04-10T...", x1.PostedAt)
	}

	x2 := byID["X2"]
	if !x2.Remote {
		t.Errorf("X2 Remote = false, want true (locationType REMOTE / isRemote)")
	}
	if want := "COL"; x2.Location != want {
		t.Errorf("X2 Location = %q, want %q (empty city skipped)", x2.Location, want)
	}
	if x2.PostedAt != nil {
		t.Errorf("X2 PostedAt = %v, want nil (zero firstPublishedTsSec)", x2.PostedAt)
	}
}

func TestGemListUsesBoardAsBoardId(t *testing.T) {
	fake := &gqlHTTP{list: gemListResp()}
	if _, err := NewGem(fake).Fetch(context.Background(), CompanyEntry{Board: "go-cadre"}); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if want := "https://jobs.gem.com/api/public/graphql"; fake.gotURL != want {
		t.Errorf("posted to %q, want %q", fake.gotURL, want)
	}
}

func TestGemFailedDetailDropsOnlyThatPosting(t *testing.T) {
	fake := &gqlHTTP{
		list: gemListResp(
			`{"extId":"OK","title":"Kept","locations":[],"job":{}}`,
			`{"extId":"BAD","title":"Dropped","locations":[],"job":{}}`,
		),
		detail:     map[string]string{"OK": gemDetailResp(`<p>ok</p>`, 1775770388)},
		failExtIDs: map[string]bool{"BAD": true},
	}

	jobs, err := NewGem(fake).Fetch(context.Background(), CompanyEntry{Board: "go-cadre"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "OK" {
		t.Fatalf("got %v, want only the OK posting", jobs)
	}
}

func TestGemListGraphQLErrorFailsBoard(t *testing.T) {
	// A GraphQL 200 carrying an errors[] (e.g. schema drift / server error) must fail the
	// board so the run records it, not look like an empty board.
	fake := &gqlHTTP{list: `{"errors":[{"message":"boom"}],"data":{"oatsExternalJobPostings":null}}`}
	if _, err := NewGem(fake).Fetch(context.Background(), CompanyEntry{Board: "go-cadre"}); err == nil {
		t.Fatal("Fetch: want error from list errors[], got nil")
	}
}

func TestGemDetailGraphQLErrorDropsPosting(t *testing.T) {
	fake := &gqlHTTP{
		list: gemListResp(
			`{"extId":"OK","title":"Kept","locations":[],"job":{}}`,
			`{"extId":"ERR","title":"Dropped","locations":[],"job":{}}`,
		),
		detail: map[string]string{
			"OK":  gemDetailResp(`<p>ok</p>`, 1775770388),
			"ERR": `{"errors":[{"message":"posting gone"}],"data":{"oatsExternalJobPosting":null}}`,
		},
	}
	jobs, err := NewGem(fake).Fetch(context.Background(), CompanyEntry{Board: "go-cadre"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "OK" {
		t.Fatalf("got %v, want only the OK posting (errors[] detail dropped)", jobs)
	}
}

func TestGemFloatTimestampSurvives(t *testing.T) {
	// firstPublishedTsSec is observed as an integer, but JSON numbers are untyped; a float
	// must still parse the date and keep the posting, never drop it (posted_at is nullable).
	fake := &gqlHTTP{
		list:   gemListResp(`{"extId":"F1","title":"Floaty","locations":[],"job":{}}`),
		detail: map[string]string{"F1": `{"data":{"oatsExternalJobPosting":{"descriptionHtml":"<p>x</p>","firstPublishedTsSec":1775770388.0}}}`},
	}
	jobs, err := NewGem(fake).Fetch(context.Background(), CompanyEntry{Board: "go-cadre"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (float timestamp must not drop the posting)", len(jobs))
	}
	if jobs[0].PostedAt == nil || !jobs[0].PostedAt.Equal(time.Unix(1775770388, 0).UTC()) {
		t.Errorf("PostedAt = %v, want parsed from float seconds", jobs[0].PostedAt)
	}
}

func TestGemEmptyBoardYieldsNoJobsNoError(t *testing.T) {
	fake := &gqlHTTP{list: gemListResp()}
	jobs, err := NewGem(fake).Fetch(context.Background(), CompanyEntry{Board: "go-cadre"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs, want 0", len(jobs))
	}
}

// Gem lists a board's whole posting set every run, so an unseen posting is already evidence of
// removal, not drift — the sweep should close it well before the 48h default.
func TestGemDeclaresNarrowSweepGrace(t *testing.T) {
	s, ok := NewGem(nil).(sweepGrace)
	if !ok {
		t.Fatal("gem should declare a sweepGrace window")
	}
	got := s.sweepGrace()
	if got != 24*time.Hour {
		t.Errorf("sweepGrace() = %v, want 24h", got)
	}
	if got >= DefaultSweepGrace {
		t.Errorf("sweepGrace() = %v, want narrower than the %v default", got, DefaultSweepGrace)
	}
}

func TestParseEpochSeconds(t *testing.T) {
	if got := parseEpochSeconds(0); got != nil {
		t.Errorf("parseEpochSeconds(0) = %v, want nil", got)
	}
	got := parseEpochSeconds(1775770388)
	if got == nil || !got.Equal(time.Unix(1775770388, 0).UTC()) {
		t.Errorf("parseEpochSeconds(1775770388) = %v", got)
	}
}
