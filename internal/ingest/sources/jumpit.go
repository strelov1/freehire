package sources

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/strelov1/freehire/internal/dict/skilltag"
)

// jumpit adapts Jumpit (jumpit.saramin.co.kr), Saramin's IT-only job board for South Korea.
// The site's own frontend reads a keyless JSON API on jumpit-api.saramin.co.kr: one listing
// names the whole open catalogue (792 postings measured 2026-11-04) and a per-posting detail
// carries the body, so the provider is boardless and hydrates a posting only when the
// catalogue lacks it.
//
// The listing is read in pages of jumpitPageSize, large enough that today the whole catalogue
// is ONE page. That is not an optimisation: the listing's order is unstable between requests,
// so at its default 16 rows a page every walk measured repeated some rows and lost as many
// (779-791 distinct of 792, whatever the sort). One page has no boundary to drift across, and
// is what lets the adapter claim fullCatalog: any page failing, the stated total moving, or
// the distinct ids falling short of it fails the crawl. See the Jumpit traps in AGENTS.md.
type jumpit struct {
	http JSONGetter
}

const (
	jumpitAPI        = "https://jumpit-api.saramin.co.kr/api"
	jumpitListURL    = jumpitAPI + "/positions?sort=reg_dt&page=%d&size=%d"
	jumpitDetailURL  = jumpitAPI + "/position/%d"
	jumpitPostingURL = "https://jumpit.saramin.co.kr/position/%d"
	jumpitPageSize   = 1000
	// jumpitMaxPages is a ceiling, not an expected end: it admits 20,000 postings against
	// 792 measured. Reaching it fails the crawl.
	jumpitMaxPages = 20
)

// The budget is unmeasured: robots.txt states no Crawl-delay and no request was refused while
// the adapter was written. Every request is held to one a second; a first run is one listing
// request and ~800 detail requests, ~14 minutes. Tune from the first runs.
const (
	jumpitRequestInterval = time.Second
	jumpitRequestBurst    = 2
)

// jumpitBrokenRunMinWanted is how many new postings, all unread, make a run broken rather than
// merely unlucky.
const jumpitBrokenRunMinWanted = 5

// jumpitZone is the zone the platform's naive timestamps are written in (KST, no DST).
var jumpitZone = time.FixedZone("KST", 9*60*60)

func NewJumpit(c JSONGetter) Source { return jumpit{http: c} }

func (jumpit) Provider() string { return "jumpit" }
func (jumpit) boardless()       {}
func (jumpit) aggregator()      {}

// fullCatalog: one crawl lists the whole open catalogue, and list fails rather than returning
// a short one, so an open posting the run did not see is gone.
func (jumpit) fullCatalog() {}

type jumpitListPage struct {
	Result struct {
		TotalCount int             `json:"totalCount"`
		Positions  []jumpitListing `json:"positions"`
	} `json:"result"`
}

// jumpitListing is a listing row. It is also the only source of the posting's geography: the
// detail's workingPlaces is the employer's office address, which for a posting abroad
// ("[일본] FDE(SW)", listed under 아시아·중동 일본 도쿄) is still the Seoul office.
type jumpitListing struct {
	ID          int64    `json:"id"`
	Title       string   `json:"title"`
	CompanyName string   `json:"companyName"`
	Locations   []string `json:"locations"`
}

type jumpitDetailResponse struct {
	Result jumpitDetail `json:"result"`
}

type jumpitDetail struct {
	ID            int64  `json:"id"`
	Title         string `json:"title"`
	CompanyName   string `json:"companyName"`
	JobCategories []struct {
		Name string `json:"name"`
	} `json:"jobCategories"`
	TechStacks []struct {
		Stack string `json:"stack"`
	} `json:"techStacks"`
	MinCareer             int    `json:"minCareer"`
	Newcomer              bool   `json:"newcomer"`
	AlwaysOpen            bool   `json:"alwaysOpen"`
	ClosedAt              string `json:"closedAt"`
	PublishedAt           string `json:"publishedAt"`
	ServiceInfo           string `json:"serviceInfo"`
	Responsibility        string `json:"responsibility"`
	Qualifications        string `json:"qualifications"`
	PreferredRequirements string `json:"preferredRequirements"`
	Welfares              string `json:"welfares"`
	RecruitProcess        string `json:"recruitProcess"`
}

func (s jumpit) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, CompanyEntry{}, func(string) bool { return false })
}

func (s jumpit) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	list, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	return s.hydrate(ctx, list, seen)
}

// list reads the listing to the stated total. Any page failing fails the crawl, and so does a
// total that moves mid-walk or a walk whose distinct ids do not reach it: the provider is swept
// by source, and a short walk would close live postings.
func (s jumpit) list(ctx context.Context) ([]jumpitListing, error) {
	var out []jumpitListing
	ids := map[int64]bool{}
	total := -1
	for page := 1; total < 0 || len(ids) < total; page++ {
		if page > jumpitMaxPages {
			return nil, fmt.Errorf("jumpit: listing did not reach its total within %d pages", jumpitMaxPages)
		}
		var p jumpitListPage
		if err := s.http.GetJSON(ctx, fmt.Sprintf(jumpitListURL, page, jumpitPageSize), &p); err != nil {
			return nil, fmt.Errorf("jumpit: listing page %d: %w", page, err)
		}
		// Past the end the platform answers an empty page whose total is 0, so the total is
		// read only from a page that carries rows.
		if len(p.Result.Positions) == 0 {
			break
		}
		if total < 0 {
			total = p.Result.TotalCount
		} else if p.Result.TotalCount != total {
			return nil, fmt.Errorf("jumpit: listing total moved from %d to %d mid-walk", total, p.Result.TotalCount)
		}
		for _, r := range p.Result.Positions {
			if r.ID == 0 || ids[r.ID] {
				continue
			}
			ids[r.ID] = true
			out = append(out, r)
		}
	}
	// The platform always lists hundreds of postings, so an empty listing is a broken answer,
	// not a quiet day.
	if total <= 0 {
		return nil, fmt.Errorf("jumpit: listing states %d postings", total)
	}
	if len(ids) != total {
		return nil, fmt.Errorf("jumpit: listing read %d distinct postings of the %d it states", len(ids), total)
	}
	return out, nil
}

// hydrate buys a posting's detail only for a posting the catalogue lacks. A detail that could
// not be read is yielded as an Unreadable identity rather than dropped: a posting offered for
// re-reading (INGEST_REFETCH_ALL, BODY_REFRESH_DAYS) is stored already, and silently losing its
// identity would let the source-wide sweep close a live posting. A new posting is never stored
// body-less, so it stays unseen and is tried again on the next run. A 404/410 is the platform
// saying the posting is gone and is simply not yielded. Once the API refuses one request
// (403/429) the rest of the run's detail requests are cancelled and those postings are Unreadable
// too: the refusal is about the IP, and every further request would only prolong it.
func (s jumpit) hydrate(ctx context.Context, list []jumpitListing, seen func(string) bool) ([]Job, error) {
	hctx, stop := context.WithCancel(ctx)
	defer stop()
	var refused atomic.Bool
	var wanted, read atomic.Int64
	jobs := fetchDetails(list, defaultDetailWorkers, func(l jumpitListing) (Job, bool) {
		id := strconv.FormatInt(l.ID, 10)
		url := fmt.Sprintf(jumpitPostingURL, l.ID)
		if seen(id) {
			return Job{ExternalID: id, URL: url, Title: l.Title, Company: l.CompanyName, SeenRefresh: true}, true
		}
		wanted.Add(1)
		unreadable := unreadableDetail(id, url, l.CompanyName)
		if refused.Load() {
			return unreadable, true
		}
		var d jumpitDetailResponse
		if err := s.http.GetJSON(hctx, fmt.Sprintf(jumpitDetailURL, l.ID), &d); err != nil {
			if isRateLimited(err) && !refused.Swap(true) {
				log.Printf("jumpit: detail requests refused (%v); deferring the rest of this run's hydration", err)
				stop()
			}
			if !detailUnreadable(err) {
				return Job{}, false // 404/410: the posting is gone
			}
			log.Printf("jumpit: detail %d failed; deferring: %v", l.ID, err)
			return unreadable, true
		}
		job, ok := parseJumpitPosting(d.Result, l)
		if !ok {
			log.Printf("jumpit: detail %d carries no usable posting; deferring", l.ID)
			return unreadable, true
		}
		read.Add(1)
		return job, true
	})
	// A crawl that listed several new postings and read none of them is broken, not quiet. One
	// unreadable posting is not: it must not discard the liveness refresh of the rest.
	if wanted.Load() >= jumpitBrokenRunMinWanted && read.Load() == 0 {
		return nil, fmt.Errorf("jumpit: listed %d new postings and read none of their details", wanted.Load())
	}
	return jobs, nil
}

func parseJumpitPosting(d jumpitDetail, l jumpitListing) (Job, bool) {
	if d.ID != l.ID {
		return Job{}, false
	}
	title := strings.Join(strings.Fields(d.Title), " ")
	company := strings.TrimSpace(d.CompanyName)
	description := sanitizeHTML(jumpitDescription(d))
	if title == "" || company == "" || strings.TrimSpace(textFromHTML(description)) == "" {
		return Job{}, false
	}
	place, countries := jumpitPlace(l.Locations)
	stacks := make([]string, 0, len(d.TechStacks))
	for _, t := range d.TechStacks {
		stacks = append(stacks, t.Stack)
	}
	categories := make([]string, 0, len(d.JobCategories))
	for _, c := range d.JobCategories {
		categories = append(categories, c.Name)
	}
	job := Job{
		ExternalID:         strconv.FormatInt(d.ID, 10),
		URL:                fmt.Sprintf(jumpitPostingURL, d.ID),
		Title:              title,
		Company:            company,
		Location:           place,
		Countries:          countries,
		Description:        description,
		PostedAt:           NotFuture(jumpitTime(d.PublishedAt)),
		Category:           jumpitCategory(categories),
		Skills:             skilltag.Canonicalize(stacks),
		ExperienceYearsMin: jumpitExperience(d),
		IsTechHint:         true, // Jumpit lists IT postings only; every category it files under is technical
	}
	if !d.AlwaysOpen {
		job.ApplicationDeadline = jumpitTime(d.ClosedAt)
	}
	return job, true
}

// jumpitSections are the detail's text fields in the order the posting page shows them, under
// the page's own headings.
var jumpitSections = []struct {
	heading string
	text    func(jumpitDetail) string
}{
	{"주요업무", func(d jumpitDetail) string { return d.Responsibility }},
	{"자격요건", func(d jumpitDetail) string { return d.Qualifications }},
	{"우대사항", func(d jumpitDetail) string { return d.PreferredRequirements }},
	{"복지 및 혜택", func(d jumpitDetail) string { return d.Welfares }},
	{"채용절차 및 기타 지원 유의사항", func(d jumpitDetail) string { return d.RecruitProcess }},
	{"기업/서비스 소개", func(d jumpitDetail) string { return d.ServiceInfo }},
}

// jumpitDescription rebuilds the body from the detail's plain-text sections, each under its
// heading. An empty section is left out.
func jumpitDescription(d jumpitDetail) string {
	var b strings.Builder
	for _, s := range jumpitSections {
		body := plainTextToHTML(s.text(d))
		if body == "" {
			continue
		}
		b.WriteString("<h3>" + s.heading + "</h3>" + body)
	}
	return b.String()
}

// jumpitCategories maps the platform's own job categories, a closed picklist of 22 measured
// 2026-11-04, each to the category the title dictionary gives its English equivalent. Two are
// deliberately absent: HW/임베디드 files hardware and embedded software under one name, and
// VR/AR/3D names a medium rather than a role, so neither says which category a posting is in.
var jumpitCategories = map[string]string{
	"서버/백엔드 개발자":      "backend",
	"게임 서버 개발자":       "backend",
	"프론트엔드 개발자":       "frontend",
	"웹퍼블리셔":           "frontend", // the HTML/CSS markup role
	"웹 풀스택 개발자":       "fullstack",
	"안드로이드 개발자":       "mobile",
	"iOS 개발자":         "mobile",
	"크로스플랫폼 앱개발자":     "mobile",
	"게임 클라이언트 개발자":    "software_engineering",
	"SW/솔루션":          "software_engineering",
	"DBA":             "devops",
	"devops/시스템 엔지니어": "devops",
	"인공지능/머신러닝":       "ml_ai",
	"빅데이터 엔지니어":       "data_engineering",
	"정보보안 담당자":        "security",
	"QA 엔지니어":         "qa",
	"개발 PM":           "project_management",
	"기술지원":            "it_support",
	"블록체인":            "blockchain",
}

// jumpitCategory is the posting's category when every category it is filed under that maps
// agrees; a posting filed under two that disagree (backend and frontend) names none, and the
// title dictionary decides. An unmapped category is ignored, not a conflict.
func jumpitCategory(names []string) string {
	var category string
	for _, name := range names {
		c, ok := jumpitCategories[strings.TrimSpace(name)]
		if !ok {
			continue
		}
		if category == "" {
			category = c
		} else if category != c {
			return ""
		}
	}
	return category
}

// jumpitRegions maps the region a listing row's location starts with — the platform's own
// picklist, measured over the whole catalogue 2026-11-04 — to the English name the location
// dictionary reads. 전남광주 is the platform's label for the merged South Jeolla and Gwangju
// region. The special self-governing provinces are listed under both of their labels.
var jumpitRegions = map[string]string{
	"서울":      "Seoul",
	"경기":      "Gyeonggi-do",
	"인천":      "Incheon",
	"대전":      "Daejeon",
	"대구":      "Daegu",
	"부산":      "Busan",
	"울산":      "Ulsan",
	"광주":      "Gwangju",
	"세종":      "Sejong",
	"세종특별자치시": "Sejong",
	"강원":      "Gangwon-do",
	"강원특별자치도": "Gangwon-do",
	"충북":      "Chungcheongbuk-do",
	"충남":      "Chungcheongnam-do",
	"전북":      "Jeollabuk-do",
	"전북특별자치도": "Jeollabuk-do",
	"전남":      "Jeollanam-do",
	"전남광주":    "Jeollanam-do",
	"경북":      "Gyeongsangbuk-do",
	"경남":      "Gyeongsangnam-do",
	"제주":      "Jeju-do",
	"제주특별자치도": "Jeju-do",
}

// jumpitPlace names each distinct Korean region a posting lists, then the country, and states
// Korea as the country. A location starting with anything else (아시아·중동 일본 도쿄, a posting
// abroad) is kept as the platform wrote it and states no country: the label names a continent
// group, not a country this adapter can map.
func jumpitPlace(locations []string) (string, []string) {
	var regions, other []string
	seen := map[string]bool{}
	for _, loc := range locations {
		loc = strings.Join(strings.Fields(loc), " ")
		if loc == "" {
			continue
		}
		head, _, _ := strings.Cut(loc, " ")
		region, ok := jumpitRegions[head]
		switch {
		case !ok:
			other = append(other, loc)
		case !seen[region]:
			seen[region] = true
			regions = append(regions, region)
		}
	}
	if len(regions) == 0 {
		return strings.Join(other, "; "), nil
	}
	place := strings.Join(regions, ", ") + ", South Korea"
	if len(other) > 0 {
		place += "; " + strings.Join(other, "; ")
	}
	return place, []string{"kr"}
}

// jumpitExperience is the stated minimum: a posting open to new graduates (newcomer) asks for
// none, which the platform also writes as minCareer 0; any other posting states its minimum.
func jumpitExperience(d jumpitDetail) *int {
	switch {
	case d.Newcomer:
		zero := 0
		return &zero
	case d.MinCareer > 0:
		n := d.MinCareer
		return &n
	}
	return nil
}

// jumpitTime parses the platform's zone-less "2006-01-02 15:04:05" KST timestamp.
func jumpitTime(s string) *time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04:05", strings.TrimSpace(s), jumpitZone)
	if err != nil {
		return nil
	}
	u := t.UTC()
	return &u
}
