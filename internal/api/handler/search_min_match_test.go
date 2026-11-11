package handler

import (
	"context"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/identity/userprofile"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/search/search"
)

var (
	heldSkills   = []string{"go", "docker", "kubernetes", "aws", "python", "java"}
	otherSkills  = []string{"react", "postgresql", "redis", "kafka", "terraform", "linux"}
	minMatchUser = userprofile.Profile{Skills: heldSkills}
)

// posting asks for six skills, held of them the reader's: 6 is 100%, 5 is 83%, 4 is 67%.
func posting(id int64, held int) search.JobDocument {
	skills := append(append([]string{}, heldSkills[:held]...), otherSkills[:6-held]...)
	return search.JobDocument{ID: id, Job: jobview.Job{PublicSlug: fmt.Sprintf("job-%d", id), Skills: skills}}
}

// rankedSearcher serves a fixed ranking by window, as the index does for a vector sort.
type rankedSearcher struct {
	docs  []search.JobDocument
	calls int
	read  int
}

func (r *rankedSearcher) Search(_ context.Context, p search.SearchParams) (search.SearchResult, error) {
	r.calls++
	lo := min(p.Offset, len(r.docs))
	hi := min(p.Offset+p.Limit, len(r.docs))
	r.read += hi - lo
	return search.SearchResult{Hits: r.docs[lo:hi], Total: int64(len(r.docs))}, nil
}

func slugs(t *testing.T, body map[string]any) []string {
	t.Helper()
	data, _ := body["data"].([]any)
	out := make([]string, len(data))
	for i, d := range data {
		out[i], _ = d.(map[string]any)["public_slug"].(string)
	}
	return out
}

func metaOf(body map[string]any) map[string]any {
	m, _ := body["meta"].(map[string]any)
	return m
}

// ranking is three postings at each coverage from 100% down to 17%, best first.
func ranking() []search.JobDocument {
	var docs []search.JobDocument
	id := int64(1)
	for _, held := range []int{6, 5, 4, 3, 1} {
		for range 3 {
			docs = append(docs, posting(id, held))
			id++
		}
	}
	return docs
}

func TestMinSkillMatch_KeepsOnlyPostingsAtOrAboveTheThresholdAcrossPages(t *testing.T) {
	r := &rankedSearcher{docs: ranking()}
	app := matchSortApp(r, 7, fakeProfiles{profile: minMatchUser})

	// 70% keeps the 100% and 83% postings (ids 1-6) and not the 67% ones.
	status, body := doGet(t, app, "/api/v1/jobs/search?sort=match&min_skill_match=70&limit=4&offset=0")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d", status)
	}
	if got := slugs(t, body); fmt.Sprint(got) != "[job-1 job-2 job-3 job-4]" {
		t.Errorf("page 1 = %v", got)
	}
	meta := metaOf(body)
	if meta["total"] != float64(5) || meta["total_is_lower_bound"] != true {
		t.Errorf("page 1 meta = %v, want total 5 as a lower bound (one more exists)", meta)
	}

	_, body = doGet(t, app, "/api/v1/jobs/search?sort=match&min_skill_match=70&limit=4&offset=4")
	if got := slugs(t, body); fmt.Sprint(got) != "[job-5 job-6]" {
		t.Errorf("page 2 = %v", got)
	}
	meta = metaOf(body)
	if meta["total"] != float64(6) || meta["total_is_lower_bound"] != true {
		t.Errorf("page 2 meta = %v, want total 6 as a lower bound (the read stopped on the rule)", meta)
	}
	if _, named := meta["ignored_params"]; named {
		t.Errorf("honoured param reported as ignored: %v", meta["ignored_params"])
	}
}

func TestMinSkillMatch_StopsOnTheRuleWithoutReadingTheRest(t *testing.T) {
	r := &rankedSearcher{docs: ranking()}
	app := matchSortApp(r, 7, fakeProfiles{profile: minMatchUser})

	doGet(t, app, "/api/v1/jobs/search?sort=match&min_skill_match=70&limit=20")
	if r.calls != 1 {
		t.Errorf("calls = %d, want 1", r.calls)
	}
}

func TestMinSkillMatch_TotalIsExactWhenTheWholeRankingWasRead(t *testing.T) {
	r := &rankedSearcher{docs: []search.JobDocument{posting(1, 6), posting(2, 6)}}
	app := matchSortApp(r, 7, fakeProfiles{profile: minMatchUser})

	_, body := doGet(t, app, "/api/v1/jobs/search?sort=match&min_skill_match=70&limit=10")
	meta := metaOf(body)
	if meta["total"] != float64(2) {
		t.Errorf("total = %v, want 2", meta["total"])
	}
	if _, set := meta["total_is_lower_bound"]; set {
		t.Errorf("total_is_lower_bound = %v, want absent: nothing was left unread", meta["total_is_lower_bound"])
	}
}

func TestMinSkillMatch_DepthBoundCapsTheRead(t *testing.T) {
	docs := make([]search.JobDocument, minMatchScanDepth+500)
	for i := range docs {
		docs[i] = posting(int64(i+1), 6)
	}
	r := &rankedSearcher{docs: docs}
	app := matchSortApp(r, 7, fakeProfiles{profile: minMatchUser})

	// The filtered stream cannot reach past the depth, so this window is cut short.
	_, body := doGet(t, app, fmt.Sprintf("/api/v1/jobs/search?sort=match&min_skill_match=70&limit=20&offset=%d", minMatchScanDepth-5))
	if got := len(slugs(t, body)); got != 5 {
		t.Errorf("hits = %d, want 5", got)
	}
	meta := metaOf(body)
	if meta["total"] != float64(minMatchScanDepth) || meta["total_is_lower_bound"] != true {
		t.Errorf("meta = %v, want total %d as a lower bound", meta, minMatchScanDepth)
	}
	if r.read > minMatchScanDepth {
		t.Errorf("read %d postings, want at most %d", r.read, minMatchScanDepth)
	}
}

func TestMinSkillMatch_IsPrivateWhenApplied(t *testing.T) {
	app := matchSortApp(&rankedSearcher{docs: ranking()}, 7, fakeProfiles{profile: minMatchUser})
	resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/api/v1/jobs/search?sort=match&min_skill_match=70", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if got := resp.Header.Get(fiber.HeaderCacheControl); got != "private, no-store" {
		t.Errorf("Cache-Control = %q, want private", got)
	}
}

func TestMinSkillMatch_IgnoredWhereItCannotBeServed(t *testing.T) {
	cases := []struct {
		name    string
		userID  int64
		profile userprofile.Profile
		query   string
	}{
		{"anonymous", 0, minMatchUser, "sort=match&min_skill_match=70"},
		{"no skills", 7, userprofile.Profile{}, "sort=match&min_skill_match=70"},
		{"other sort", 7, minMatchUser, "sort=posted_at&min_skill_match=70"},
		{"no sort", 7, minMatchUser, "min_skill_match=70"},
		{"out of range", 7, minMatchUser, "sort=match&min_skill_match=101"},
		{"not a number", 7, minMatchUser, "sort=match&min_skill_match=lots"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := &rankedSearcher{docs: ranking()}
			app := matchSortApp(r, c.userID, fakeProfiles{profile: c.profile})
			status, body := doGet(t, app, "/api/v1/jobs/search?limit=20&"+c.query)
			if status != fiber.StatusOK {
				t.Fatalf("status = %d, want 200", status)
			}
			if !reportsParam(body, minSkillMatchParam) {
				t.Errorf("ignored_params = %v, want min_skill_match named", ignoredParamNames(body))
			}
			if got := len(slugs(t, body)); got != len(r.docs) {
				t.Errorf("hits = %d, want the unfiltered %d", got, len(r.docs))
			}
		})
	}
}
