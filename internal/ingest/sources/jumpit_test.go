package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// The fixtures are live jumpit-api.saramin.co.kr responses fetched 2026-11-04: five listing rows
// verbatim under a total trimmed to five, and each one's detail with the long text sections cut
// to their first lines. The five were chosen for the mapping rules they exercise: an ordinary
// posting (55441892), one listed abroad whose office address is in Seoul (55366649), one open to
// new graduates (55451145), one filed under two categories that disagree (55425594), and one
// filed only under the unmapped HW/임베디드 (55482651).

func jumpitFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// jumpitFake serves fixed bodies by URL; an unknown URL is the platform's "Entity Not Found",
// which it answers with a 400. Safe for the detail workers' concurrent calls.
type jumpitFake struct {
	mu     sync.Mutex
	bodies map[string]string
	errs   map[string]error
	got    []string
}

func (f *jumpitFake) GetJSON(_ context.Context, url string, v any) error {
	f.mu.Lock()
	f.got = append(f.got, url)
	f.mu.Unlock()
	if err, ok := f.errs[url]; ok {
		return err
	}
	body, ok := f.bodies[url]
	if !ok {
		return &StatusError{Method: "GET", Code: 400, URL: url}
	}
	return json.Unmarshal([]byte(body), v)
}

func (f *jumpitFake) asked(url string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Contains(f.got, url)
}

var jumpitFixtureIDs = []int64{55441892, 55366649, 55451145, 55425594, 55482651}

func jumpitPage(page int) string { return fmt.Sprintf(jumpitListURL, page, jumpitPageSize) }

// newJumpitFake serves the listing fixture as page 1 and every fixture detail.
func newJumpitFake(t *testing.T) *jumpitFake {
	t.Helper()
	f := &jumpitFake{bodies: map[string]string{jumpitPage(1): jumpitFixture(t, "jumpit_list.json")}}
	for _, id := range jumpitFixtureIDs {
		f.bodies[fmt.Sprintf(jumpitDetailURL, id)] = jumpitFixture(t, fmt.Sprintf("jumpit_detail_%d.json", id))
	}
	return f
}

func jumpitByID(t *testing.T, jobs []Job) map[string]Job {
	t.Helper()
	out := map[string]Job{}
	for _, j := range jobs {
		if _, dup := out[j.ExternalID]; dup {
			t.Fatalf("posting %s yielded twice", j.ExternalID)
		}
		out[j.ExternalID] = j
	}
	return out
}

func TestJumpitListReadsTheWholeCatalogueInOneRequest(t *testing.T) {
	f := newJumpitFake(t)
	list, err := jumpit{http: f}.list(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != len(jumpitFixtureIDs) {
		t.Fatalf("listed %d, want %d", len(list), len(jumpitFixtureIDs))
	}
	// The total is reached on page 1, so the walk asks for nothing past it.
	if f.asked(jumpitPage(2)) {
		t.Errorf("walked past a page that already reached the stated total: %v", f.got)
	}
}

// The provider is swept by source, so every way a walk can come back short must fail the crawl
// rather than return what it has.
func TestJumpitListFailsEveryShortWalk(t *testing.T) {
	row := func(id int64) string {
		return fmt.Sprintf(`{"id":%d,"title":"t","companyName":"c","locations":["서울 강남구"]}`, id)
	}
	page := func(total int, ids ...int64) string {
		rows := make([]string, len(ids))
		for i, id := range ids {
			rows[i] = row(id)
		}
		return fmt.Sprintf(`{"status":200,"result":{"totalCount":%d,"positions":[%s]}}`, total, strings.Join(rows, ","))
	}
	empty := `{"status":200,"result":{"totalCount":0,"positions":[]}}`
	cases := []struct {
		name   string
		bodies map[string]string
		errs   map[string]error
		want   string
	}{
		{"first page fails", nil, map[string]error{jumpitPage(1): &StatusError{Method: "GET", Code: 503, URL: jumpitPage(1)}}, "listing page 1"},
		{"later page fails", map[string]string{jumpitPage(1): page(3, 1, 2)}, map[string]error{jumpitPage(2): &StatusError{Method: "GET", Code: 503, URL: jumpitPage(2)}}, "listing page 2"},
		{"ends short of its total", map[string]string{jumpitPage(1): page(3, 1, 2), jumpitPage(2): empty}, nil, "read 2 distinct postings of the 3"},
		{"a repeated row hides a lost one", map[string]string{jumpitPage(1): page(3, 1, 2), jumpitPage(2): page(3, 2), jumpitPage(3): empty}, nil, "read 2 distinct postings of the 3"},
		{"total moves mid-walk", map[string]string{jumpitPage(1): page(3, 1, 2), jumpitPage(2): page(4, 3)}, nil, "total moved from 3 to 4"},
		{"empty catalogue", map[string]string{jumpitPage(1): empty}, nil, "states -1 postings"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &jumpitFake{bodies: tc.bodies, errs: tc.errs}
			if _, err := (jumpit{http: f}).list(context.Background()); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want one containing %q", err, tc.want)
			}
		})
	}
}

func TestJumpitHydratesOnlyWhatTheCatalogueLacks(t *testing.T) {
	f := newJumpitFake(t)
	seen := func(id string) bool { return id == "55441892" }
	jobs, err := jumpit{http: f}.FetchNew(context.Background(), CompanyEntry{}, seen)
	if err != nil {
		t.Fatal(err)
	}
	byID := jumpitByID(t, jobs)
	if len(byID) != len(jumpitFixtureIDs) {
		t.Fatalf("yielded %d postings, want %d", len(byID), len(jumpitFixtureIDs))
	}
	stored := byID["55441892"]
	if !stored.SeenRefresh || stored.Description != "" {
		t.Errorf("a stored posting was re-read instead of refreshed: %+v", stored)
	}
	if stored.Title != "SAP ABAP 개발 및 운영 담당자" || stored.Company != "지에스아이티엠" || stored.URL != "https://jumpit.saramin.co.kr/position/55441892" {
		t.Errorf("refresh identity = %q / %q / %q", stored.Title, stored.Company, stored.URL)
	}
	if f.asked(fmt.Sprintf(jumpitDetailURL, 55441892)) {
		t.Error("fetched the detail of a posting the catalogue already holds")
	}
}

func TestJumpitMapsAPosting(t *testing.T) {
	jobs, err := jumpit{http: newJumpitFake(t)}.Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatal(err)
	}
	j := jumpitByID(t, jobs)["55441892"]
	if j.Title != "SAP ABAP 개발 및 운영 담당자" || j.Company != "지에스아이티엠" {
		t.Errorf("title/company = %q / %q", j.Title, j.Company)
	}
	if j.Location != "Seoul, South Korea" || !slices.Equal(j.Countries, []string{"kr"}) {
		t.Errorf("place = %q %v", j.Location, j.Countries)
	}
	if j.Category != "software_engineering" || !j.IsTechHint {
		t.Errorf("category = %q, tech hint = %v", j.Category, j.IsTechHint)
	}
	if j.ExperienceYearsMin == nil || *j.ExperienceYearsMin != 1 {
		t.Errorf("experience = %v, want 1", j.ExperienceYearsMin)
	}
	if !slices.Contains(j.Skills, "sap") {
		t.Errorf("skills = %v, want the stated SAP stack canonicalised", j.Skills)
	}
	// "2026-10-31 00:00:00" is KST, nine hours ahead of UTC.
	if want := time.Date(2026, 10, 30, 15, 0, 0, 0, time.UTC); j.PostedAt == nil || !j.PostedAt.Equal(want) {
		t.Errorf("posted = %v, want %v", j.PostedAt, want)
	}
	if want := time.Date(2026, 11, 29, 14, 59, 59, 0, time.UTC); j.ApplicationDeadline == nil || !j.ApplicationDeadline.Equal(want) {
		t.Errorf("deadline = %v, want %v", j.ApplicationDeadline, want)
	}
	for _, heading := range []string{"<h3>주요업무</h3>", "<h3>자격요건</h3>", "<h3>우대사항</h3>", "<h3>기업/서비스 소개</h3>"} {
		if !strings.Contains(j.Description, heading) {
			t.Errorf("description lacks %s: %s", heading, j.Description)
		}
	}
	if !strings.Contains(j.Description, "<li>SAP ABAP 개발 경력 1년 이상</li>") {
		t.Errorf("a bulleted qualification was not rendered as a list item: %s", j.Description)
	}
}

func TestJumpitMappingRules(t *testing.T) {
	jobs, err := jumpit{http: newJumpitFake(t)}.Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatal(err)
	}
	byID := jumpitByID(t, jobs)

	// Listed abroad, office in Seoul: the listing's region is the work location, and a label
	// the adapter cannot map states no country rather than the office's.
	abroad := byID["55366649"]
	if abroad.Location != "아시아·중동 일본 도쿄" || abroad.Countries != nil {
		t.Errorf("abroad place = %q %v, want the platform's label and no country", abroad.Location, abroad.Countries)
	}
	if abroad.Category != "devops" {
		t.Errorf("abroad category = %q", abroad.Category)
	}

	newcomer := byID["55451145"]
	if newcomer.ExperienceYearsMin == nil || *newcomer.ExperienceYearsMin != 0 {
		t.Errorf("newcomer experience = %v, want 0", newcomer.ExperienceYearsMin)
	}
	if newcomer.Category != "ml_ai" {
		t.Errorf("newcomer category = %q", newcomer.Category)
	}

	// Backend and frontend disagree, so the posting names no category.
	if mixed := byID["55425594"]; mixed.Category != "" || mixed.ExperienceYearsMin == nil || *mixed.ExperienceYearsMin != 6 {
		t.Errorf("mixed category = %q, experience = %v", mixed.Category, mixed.ExperienceYearsMin)
	}

	// HW/임베디드 is unmapped; the posting is still technical and placed.
	hw := byID["55482651"]
	if hw.Category != "" || !hw.IsTechHint || hw.Location != "Gyeonggi-do, South Korea" {
		t.Errorf("hw = category %q, tech %v, place %q", hw.Category, hw.IsTechHint, hw.Location)
	}

	for id, j := range byID {
		if !j.IsTechHint {
			t.Errorf("%s: every Jumpit posting is technical", id)
		}
	}
}

func TestJumpitCategoryNeedsEveryMappedCategoryToAgree(t *testing.T) {
	cases := []struct {
		names []string
		want  string
	}{
		{[]string{"서버/백엔드 개발자"}, "backend"},
		{[]string{"서버/백엔드 개발자", "게임 서버 개발자"}, "backend"},
		{[]string{"서버/백엔드 개발자", "프론트엔드 개발자"}, ""},
		{[]string{"HW/임베디드", "인공지능/머신러닝"}, "ml_ai"},
		{[]string{"VR/AR/3D"}, ""},
		{nil, ""},
	}
	for _, tc := range cases {
		if got := jumpitCategory(tc.names); got != tc.want {
			t.Errorf("jumpitCategory(%v) = %q, want %q", tc.names, got, tc.want)
		}
	}
}

func TestJumpitPlaceMapsEveryMeasuredRegion(t *testing.T) {
	// Every region label the catalogue carried on 2026-11-04, with a district after it.
	for _, label := range []string{"서울", "경기", "대전", "인천", "충남", "충북", "대구", "제주", "전남광주", "울산", "경남", "강원", "부산", "세종특별자치시", "경북"} {
		place, countries := jumpitPlace([]string{label + " 어딘가구"})
		if !strings.HasSuffix(place, ", South Korea") || !slices.Equal(countries, []string{"kr"}) {
			t.Errorf("%s -> %q %v", label, place, countries)
		}
	}
	if place, countries := jumpitPlace([]string{"서울 강남구", "서울 종로구", "경기 성남시"}); place != "Seoul, Gyeonggi-do, South Korea" || len(countries) != 1 {
		t.Errorf("two districts of one region = %q %v", place, countries)
	}
}

func TestJumpitFailsWhenItReadsNoneOfWhatItListed(t *testing.T) {
	f := newJumpitFake(t)
	f.errs = map[string]error{}
	for _, id := range jumpitFixtureIDs {
		u := fmt.Sprintf(jumpitDetailURL, id)
		f.errs[u] = &StatusError{Method: "GET", Code: 503, URL: u}
	}
	if _, err := (jumpit{http: f}).Fetch(context.Background(), CompanyEntry{}); err == nil || !strings.Contains(err.Error(), "read none") {
		t.Fatalf("err = %v, want the listed-but-unread failure", err)
	}
}

func TestJumpitKeepsTheIdentityOfAnUnreadableDetail(t *testing.T) {
	f := newJumpitFake(t)
	f.errs = map[string]error{}
	bad, gone := jumpitFixtureIDs[0], jumpitFixtureIDs[1]
	badURL, goneURL := fmt.Sprintf(jumpitDetailURL, bad), fmt.Sprintf(jumpitDetailURL, gone)
	f.errs[badURL] = &StatusError{Method: "GET", Code: 503, URL: badURL}
	f.errs[goneURL] = &StatusError{Method: "GET", Code: 404, URL: goneURL}

	jobs, err := (jumpit{http: f}).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("one unreadable posting must not fail the crawl: %v", err)
	}
	var unreadable, hydrated int
	for _, j := range jobs {
		switch {
		case j.ExternalID == strconv.FormatInt(gone, 10):
			t.Errorf("a 404 posting is gone and must not be yielded: %+v", j)
		case j.Unreadable:
			if j.ExternalID != strconv.FormatInt(bad, 10) {
				t.Errorf("unexpected unreadable %s", j.ExternalID)
			}
			unreadable++
		case j.Description != "":
			hydrated++
		}
	}
	if unreadable != 1 || hydrated != len(jumpitFixtureIDs)-2 {
		t.Errorf("unreadable = %d, hydrated = %d", unreadable, hydrated)
	}
}
