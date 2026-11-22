package sources

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/html"
)

// saraminFixtures holds trimmed live responses fetched on 2026-11-22: three rows of the IT
// category's first listing page (a company with no profile link, one with a group and a stock
// badge, and one with a homepage-apply button), the empty listing from page 107, and the body
// iframe of a posting written as text.
// The image-only body is a hand-cut stand-in for the common case of a posting published as
// pictures.
//
//go:embed testdata/saramin
var saraminFixtures embed.FS

func saraminFixture(t *testing.T, name string) *html.Node {
	t.Helper()
	b, err := saraminFixtures.ReadFile("testdata/saramin/" + name)
	if err != nil {
		t.Fatal(err)
	}
	root, err := html.Parse(strings.NewReader(string(b)))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// saraminFake serves the listing by category and page, and bodies by posting id.
type saraminFake struct {
	mu     sync.Mutex
	lists  map[string]*html.Node // "cat/page" -> page
	empty  *html.Node            // default for listing pages
	bodies map[string]*html.Node // id -> iframe; an absent id is a 404
	failID string                // answers 503; "*" answers every body with it
	asked  []string
}

func (f *saraminFake) GetHTML(_ context.Context, url string) (*html.Node, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.asked = append(f.asked, url)
	var cat, page int
	if _, err := fmt.Sscanf(url, saraminListURL, &cat, &page); err == nil {
		if n, ok := f.lists[fmt.Sprintf("%d/%d", cat, page)]; ok {
			return n, nil
		}
		return f.empty, nil
	}
	for id, n := range f.bodies {
		if url == fmt.Sprintf(saraminDetailURL, id) {
			return n, nil
		}
	}
	if f.failID == "*" || (f.failID != "" && url == fmt.Sprintf(saraminDetailURL, f.failID)) {
		return nil, &StatusError{Code: http.StatusServiceUnavailable}
	}
	return nil, &StatusError{Code: http.StatusNotFound}
}

func (f *saraminFake) detailAsked() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, u := range f.asked {
		if strings.Contains(u, "view-detail") {
			n++
		}
	}
	return n
}

func newSaraminTest(t *testing.T, f HTMLGetter) saramin {
	t.Helper()
	return saramin{
		http:         f,
		limiter:      noWait{},
		maxNewPerRun: 100,
		retryBackoff: time.Millisecond,
		now:          func() time.Time { return time.Date(2026, 11, 22, 3, 0, 0, 0, time.UTC) },
	}
}

func newSaraminFake(t *testing.T) *saraminFake {
	t.Helper()
	list := saraminFixture(t, "list.html")
	return &saraminFake{
		empty: saraminFixture(t, "list_empty.html"),
		// The same page under the IT category and another: its postings are deduplicated and
		// keep the tech hint from the IT one.
		lists: map[string]*html.Node{
			"2/1":  list,
			"11/1": list,
		},
		bodies: map[string]*html.Node{
			"55598463": saraminFixture(t, "detail_text.html"),
			"55611022": saraminFixture(t, "detail_images.html"),
		},
	}
}

func TestSaraminParseCards(t *testing.T) {
	cards := saraminParseCards(saraminFixture(t, "list.html"))
	if len(cards) != 3 {
		t.Fatalf("cards = %d, want 3", len(cards))
	}
	c := cards[0]
	if c.id != "55598463" || c.company != "(주)한림이앤지" ||
		c.title != "AI 기반 기술보고서 및 사내 업무 자동화 시스템 개발 담당자 채용" ||
		c.place != "서울 강남구 외" || c.career != "경력 1년↑ · 정규직" || c.education != "대학교(4년)↑" ||
		c.deadline != "D-3" || c.posted != "5일 전 등록" {
		t.Errorf("card 0 = %+v", c)
	}
	if len(c.sectors) != 5 || c.sectors[0] != "데이터엔지니어" {
		t.Errorf("sectors = %v", c.sectors)
	}
	if cards[1].company != "대주전자재료(주)" || cards[1].deadline != "~12.19(토)" {
		t.Errorf("card 1 = %+v", cards[1])
	}
}

func TestSaraminFetchNew(t *testing.T) {
	f := newSaraminFake(t)
	s := newSaraminTest(t, f)

	jobs, err := s.FetchNew(context.Background(), CompanyEntry{}, func(id string) bool { return id == "55617109" })
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	if len(byID) != 3 {
		t.Fatalf("jobs = %d, want 3 (each posting once)", len(byID))
	}
	if !byID["55617109"].SeenRefresh || byID["55617109"].Title != "" {
		t.Errorf("a stored posting is refreshed by identity: %+v", byID["55617109"])
	}
	if got := f.detailAsked(); got != 2 {
		t.Errorf("detail requests = %d, want 2 (the stored posting costs none)", got)
	}

	j := byID["55598463"]
	if j.Company != "(주)한림이앤지" || j.Location != "서울 강남구" || j.URL != "https://www.saramin.co.kr/zf_user/jobs/view?rec_idx=55598463" {
		t.Errorf("job = %+v", j)
	}
	if !j.IsTechHint {
		t.Error("a posting listed under the IT category carries IsTechHint")
	}
	if j.EmploymentType != "full_time" || j.ExperienceYearsMin == nil || *j.ExperienceYearsMin != 1 || j.EducationLevel != "bachelor" {
		t.Errorf("facets = %q %v %q", j.EmploymentType, j.ExperienceYearsMin, j.EducationLevel)
	}
	if got := j.ApplicationDeadline; got == nil || got.Format("2006-01-02") != "2026-11-25" {
		t.Errorf("deadline = %v", got)
	}
	if got := j.PostedAt; got == nil || got.Format("2006-01-02") != "2026-11-17" {
		t.Errorf("posted = %v", got)
	}
	if !strings.Contains(j.Description, "네트워크") {
		t.Errorf("description lacks the body: %.200s", j.Description)
	}
	if len(j.FallbackCountries) != 1 || j.FallbackCountries[0] != "kr" {
		t.Errorf("countries = %v", j.FallbackCountries)
	}

	img := byID["55611022"]
	if !strings.Contains(img.Description, "근무지역: 경기 시흥시") {
		t.Errorf("an image-only posting falls back to the card's facts: %q", img.Description)
	}
}

func TestSaraminUnreadableDetailKeepsListingIdentity(t *testing.T) {
	f := newSaraminFake(t)
	f.failID = "55598463"
	delete(f.bodies, "55598463")
	s := newSaraminTest(t, f)
	jobs, err := s.Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 {
		t.Fatalf("jobs = %+v, want a readable posting and an unreadable marker; the 404 is omitted", jobs)
	}
	for _, j := range jobs {
		if j.ExternalID == "55598463" && j.Unreadable && !j.SeenRefresh && j.Description == "" &&
			j.Company == "(주)한림이앤지" && j.URL == fmt.Sprintf(saraminPostingURL, j.ExternalID) {
			return
		}
	}
	t.Errorf("jobs lack the unreadable posting's listing identity: %+v", jobs)
}

func TestSaraminNothingAnsweredFails(t *testing.T) {
	f := newSaraminFake(t)
	f.bodies = nil
	f.failID = "*"
	s := newSaraminTest(t, f)
	if _, err := s.FetchNew(context.Background(), CompanyEntry{}, func(id string) bool { return id == "55617109" }); err == nil {
		t.Fatal("a crawl whose every body request failed must fail even with a seen refresh")
	}
}

func TestSaraminMissingRequiredCardFieldsFailHydration(t *testing.T) {
	f := newSaraminFake(t)
	list, err := html.Parse(strings.NewReader(`
<div id="default_list_wrap"><section class="list_recruiting"><div class="list_body">
    <div id="rec-11" class="list_item"><div class="company_nm"><span class="str_tit">Employer</span></div></div>
    <div id="rec-12" class="list_item"><a id="rec_link_12" class="str_tit">Posting</a></div>
</div></section></div>`))
	if err != nil {
		t.Fatal(err)
	}
	f.lists = map[string]*html.Node{"2/1": list}
	s := newSaraminTest(t, f)
	if _, err := s.Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("a crawl whose every identified new card is malformed must fail")
	}
	if got := f.detailAsked(); got != 0 {
		t.Errorf("detail requests = %d, want 0", got)
	}
}

func TestSaraminBoundsNewPostings(t *testing.T) {
	f := newSaraminFake(t)
	s := newSaraminTest(t, f)
	s.maxNewPerRun = 1
	if _, err := s.FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false }); err != nil {
		t.Fatal(err)
	}
	if got := f.detailAsked(); got != 1 {
		t.Errorf("detail requests = %d, want 1", got)
	}
}

func TestSaraminListPageFailureFailsCrawl(t *testing.T) {
	s := newSaraminTest(t, &failingListFake{})
	if _, err := s.Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("a failing listing page must fail the crawl")
	}
}

func TestSaraminUnexpectedHTMLDoesNotEndCategory(t *testing.T) {
	f := newSaraminFake(t)
	page, err := html.Parse(strings.NewReader(`<html><body><div class="info_empty">No suggestions</div></body></html>`))
	if err != nil {
		t.Fatal(err)
	}
	f.lists["2/2"] = page
	s := newSaraminTest(t, f)
	if _, err := s.Fetch(context.Background(), CompanyEntry{}); err == nil || !strings.Contains(err.Error(), "category 2: page 2:") {
		t.Fatalf("second listing page error = %v", err)
	}
}

type failingListFake struct{}

func (failingListFake) GetHTML(context.Context, string) (*html.Node, error) {
	return nil, errors.New("boom")
}

func TestSaraminEmptyListingFails(t *testing.T) {
	s := newSaraminTest(t, &saraminFake{empty: saraminFixture(t, "list_empty.html")})
	if _, err := s.Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("a listing with no posting must fail the crawl")
	}
}

func TestSaraminLabels(t *testing.T) {
	now := time.Date(2026, 12, 30, 3, 0, 0, 0, time.UTC)
	if d := saraminDeadline("~01.05(월)", now); d == nil || d.Format("2006-01-02") != "2027-01-05" {
		t.Errorf("a deadline across new year = %v", d)
	}
	if d := saraminDeadline("~12.31(목)", now); d == nil || d.Format("2006-01-02") != "2026-12-31" {
		t.Errorf("same-year deadline = %v", d)
	}
	for _, open := range []string{"상시채용", "채용시", ""} {
		if d := saraminDeadline(open, now); d != nil {
			t.Errorf("%q has no deadline, got %v", open, d)
		}
	}
	if y, ok := saraminMinYears("경력 3~5년"); !ok || y != 3 {
		t.Errorf("range years = %d %v", y, ok)
	}
	if _, ok := saraminMinYears("경력무관"); ok {
		t.Error("경력무관 states no minimum")
	}
	if _, ok := saraminMinYears("신입"); ok {
		t.Error("신입 states no minimum")
	}
	if saraminEmploymentType("병역특례") != "" || saraminEmploymentType("인턴") != "internship" {
		t.Error("employment types")
	}
	if saraminEducation("고졸↑") != "" || saraminEducation("학력무관") != "none" {
		t.Error("education")
	}
}
