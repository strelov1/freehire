package sources

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	xhtml "golang.org/x/net/html"
	"golang.org/x/time/rate"
)

// saramin adapts Saramin (www.saramin.co.kr), South Korea's largest general job board, every
// trade. It is boardless and an aggregator: one crawl covers the whole catalogue, the employer
// read per posting. See providers/saramin.md for what was measured and why each route was chosen.
//
// The posting set comes from the site's own category listing, /zf_user/jobs/list/job-category,
// walked one top-level category (cat_mcls) at a time, 100 cards a page. A card already carries
// the title, employer, place, experience and employment type, the deadline and the posting's own
// occupation tags, so a stored posting is refreshed from the listing alone. Only a posting the
// catalogue lacks costs one more request, for its body: the same iframe the posting page embeds,
// /zf_user/jobs/relay/view-detail. Neither path is disallowed by robots.txt. The category the
// site files under "IT개발·데이터" is the source's own statement that a posting is technical, so
// those postings carry IsTechHint; the rest are left to the dictionaries.
type saramin struct {
	http         HTMLGetter
	limiter      waiter
	maxNewPerRun int
	// retryBackoff is the first pause before a refused page is asked again; it doubles.
	retryBackoff time.Duration
	now          func() time.Time
}

const (
	saraminBase = "https://www.saramin.co.kr"
	// saraminListURL names a page of one category's listing. page_count is honoured up to 100;
	// each page costs ~270 KB on the wire however few cards it holds, so the widest is cheapest.
	saraminListURL = saraminBase + "/zf_user/jobs/list/job-category?cat_mcls=%d&page=%d&page_count=100"
	// saraminDetailURL is the iframe the posting page embeds its body in. It answers a plain GET.
	saraminDetailURL = saraminBase + "/zf_user/jobs/relay/view-detail?rec_idx=%s&rec_seq=0"
	// saraminPostingURL is the posting's public page, the form its own rel=canonical names.
	saraminPostingURL = saraminBase + "/zf_user/jobs/view?rec_idx=%s"

	// saraminTechCategory is the listing's "IT개발·데이터" category.
	saraminTechCategory = 2

	// saraminMaxPages ends a category walk that has not reached its end: the largest category
	// held ~450 pages on 2026-11-22. Reaching it fails the crawl rather than ending it softly, so
	// a runaway walk never reads as a shrunken catalogue.
	saraminMaxPages = 1000

	// saraminSweepGrace keeps a posting open through a failed run or two: the walk is the only
	// evidence of what is live, and it is long enough that a run can be missed.
	saraminSweepGrace = 72 * time.Hour

	saraminRetries      = 2
	saraminRetryBackoff = 2 * time.Second

	// saraminRequestInterval paces every request of a run at four a second, under the 300 a
	// minute the edge announces in X-RateLimit-Limit.
	saraminRequestInterval = 250 * time.Millisecond
	// saraminListWorkers walk categories side by side; a page takes ~2.6 s, so one worker would
	// need two hours for the ~2,900 pages while the limiter admits four a second.
	saraminListWorkers   = 5
	saraminDetailWorkers = 3
	// saraminMaxNewPerRun bounds how many unseen postings one run reads a body for, newest id
	// first: the walk itself takes ~25 minutes of the run, and the cold catalogue drains over
	// the following runs.
	saraminMaxNewPerRun = 3000
)

// saraminCategories are the top-level occupation categories (cat_mcls) the listing answers for,
// largest first so the longest walk starts at once. 1, 23 and 24 answer an empty listing.
var saraminCategories = []int{11, 8, 6, 10, 22, 4, 18, 16, 14, 2, 3, 19, 7, 9, 15, 12, 21, 20, 13, 5, 17}

// NewSaramin builds the Saramin adapter over the given client.
func NewSaramin(c HTMLGetter) Source {
	return saramin{
		http:         c,
		limiter:      rate.NewLimiter(rate.Every(saraminRequestInterval), 1),
		maxNewPerRun: saraminMaxNewPerRun,
		retryBackoff: saraminRetryBackoff,
		now:          time.Now,
	}
}

func (saramin) Provider() string { return "saramin" }

// boardless: one crawl covers the whole national catalogue.
func (saramin) Boardless() {}

// Every posting names its own employer, so saramin stays in the source facet and in the
// cross-source duplicate suppression set (jumpit, Saramin's IT board, lists the same postings
// under its own key).
func (saramin) Aggregator() {}

func (saramin) SweepGrace() time.Duration { return saraminSweepGrace }

// saraminCard is one listing row.
type saraminCard struct {
	id        string
	title     string
	company   string
	place     string
	career    string // "경력 1년↑ · 정규직"
	education string
	deadline  string // "~12.19(토)", "D-3", "상시채용"
	posted    string // "5일 전 등록"
	sectors   []string
	tech      bool
}

// Fetch is the list-only fallback HydratingSource specifies: everything is unseen.
func (s saramin) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew walks the whole listing, refreshes seen postings by identity, and reads a body for
// up to maxNewPerRun of the rest, newest id first. Failed reads preserve the listing's liveness
// evidence so a stored posting can stay open while its body is retried.
func (s saramin) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	cards, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	var jobs []Job
	var unseen []saraminCard
	deferred := 0
	for _, c := range cards {
		if seen(c.id) {
			jobs = append(jobs, Job{ExternalID: c.id, URL: fmt.Sprintf(saraminPostingURL, c.id), SeenRefresh: true})
			continue
		}
		if len(unseen) >= s.maxNewPerRun {
			deferred++
			continue
		}
		unseen = append(unseen, c)
	}
	var answered atomic.Int64
	read := fetchDetails(unseen, saraminDetailWorkers, func(c saraminCard) (Job, bool) {
		job, ok, err := s.detail(ctx, c)
		if err != nil {
			log.Printf("saramin: posting %s failed; deferring: %v", c.id, err)
			return unreadableDetail(c.id, fmt.Sprintf(saraminPostingURL, c.id), c.company), true
		}
		answered.Add(1)
		return job, ok
	})
	if len(unseen) > 0 && answered.Load() == 0 {
		return nil, fmt.Errorf("saramin: attempted to read %d new postings and none answered", len(unseen))
	}
	if deferred > 0 {
		log.Printf("saramin: %d new postings past this run's bound of %d, deferred to later runs", deferred, s.maxNewPerRun)
	}
	return append(jobs, read...), nil
}

// list walks every category to its end and returns the distinct postings, newest id first. Any
// page failing fails the crawl: a short list would leave the postings it missed unrefreshed.
func (s saramin) list(ctx context.Context) ([]saraminCard, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		mu       sync.Mutex
		byID     = make(map[string]*saraminCard)
		firstErr error
		wg       sync.WaitGroup
	)
	work := make(chan int)
	for range saraminListWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for cat := range work {
				cards, err := s.walkCategory(ctx, cat)
				mu.Lock()
				if err != nil && firstErr == nil {
					firstErr = fmt.Errorf("saramin: category %d: %w", cat, err)
					cancel()
				}
				for _, c := range cards {
					if have, ok := byID[c.id]; ok {
						have.tech = have.tech || cat == saraminTechCategory
						continue
					}
					c.tech = cat == saraminTechCategory
					byID[c.id] = &c
				}
				mu.Unlock()
			}
		}()
	}
feed:
	for _, cat := range saraminCategories {
		select {
		case work <- cat:
		case <-ctx.Done():
			break feed
		}
	}
	close(work)
	wg.Wait()

	if firstErr != nil {
		return nil, firstErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(byID) == 0 {
		return nil, errors.New("saramin: the listing carries no posting")
	}
	out := make([]saraminCard, 0, len(byID))
	for _, c := range byID {
		out = append(out, *c)
	}
	// Ids are numeric and of one width today, but compare by length first so a digit gained
	// later cannot reorder the newest postings behind the old.
	sort.Slice(out, func(i, j int) bool {
		if len(out[i].id) != len(out[j].id) {
			return len(out[i].id) > len(out[j].id)
		}
		return out[i].id > out[j].id
	})
	return out, nil
}

// walkCategory reads one category's pages until the listing explicitly reports no results.
func (s saramin) walkCategory(ctx context.Context, cat int) ([]saraminCard, error) {
	var out []saraminCard
	for page := 1; page <= saraminMaxPages; page++ {
		cards, err := s.listPage(ctx, cat, page)
		if err != nil {
			return nil, fmt.Errorf("page %d: %w", page, err)
		}
		if len(cards) == 0 {
			return out, nil
		}
		out = append(out, cards...)
	}
	return nil, fmt.Errorf("no end within %d pages", saraminMaxPages)
}

func (s saramin) listPage(ctx context.Context, cat, page int) ([]saraminCard, error) {
	var root *xhtml.Node
	err := retryWhile(ctx, saraminRetries, s.retryBackoff, saraminRefused, func() error {
		if err := s.limiter.Wait(ctx); err != nil {
			return err
		}
		var err error
		root, err = s.http.GetHTML(ctx, fmt.Sprintf(saraminListURL, cat, page))
		return err
	})
	if err != nil {
		return nil, err
	}
	wrap := firstByID(root, "default_list_wrap")
	if wrap == nil {
		return nil, errors.New("page carries no category listing")
	}
	list := firstByClass(wrap, "list_body")
	if list == nil {
		return nil, errors.New("category listing has no body")
	}
	cards := saraminParseCards(list)
	if len(cards) == 0 && firstByClass(list, "info_empty") == nil {
		return nil, errors.New("category listing carries neither posting cards nor an empty result")
	}
	return cards, nil
}

// saraminRefused reports an answer worth asking again: a rate limit or a server failure.
func saraminRefused(err error) bool {
	var se *StatusError
	return errors.As(err, &se) && (se.Code == http.StatusForbidden || se.Code == http.StatusTooManyRequests || se.Code >= 500)
}

var saraminCardID = regexp.MustCompile(`^rec-(\d+)$`)

// saraminParseCards reads the listing's rows, skipping the sponsored banners above them (those
// are bare links with no card structure and also appear as rows below).
func saraminParseCards(root *xhtml.Node) []saraminCard {
	var out []saraminCard
	walk(root, func(n *xhtml.Node) bool {
		if n.Type != xhtml.ElementNode || n.Data != "div" || !HasClass(n, "list_item") {
			return true
		}
		m := saraminCardID.FindStringSubmatch(Attr(n, "id"))
		if m == nil {
			return true
		}
		c := saraminCard{id: m[1]}
		walk(n, func(d *xhtml.Node) bool {
			if d.Type != xhtml.ElementNode {
				return true
			}
			switch {
			case d.Data == "a" && HasClass(d, "str_tit") && Attr(d, "id") == "rec_link_"+c.id:
				c.title = collapseSpace(Attr(d, "title"))
				if c.title == "" {
					c.title = collapseSpace(textContent(d))
				}
			case HasClass(d, "company_nm"):
				if t := firstByClass(d, "str_tit"); t != nil {
					c.company = collapseSpace(textContent(t))
				}
			case HasClass(d, "job_sector"):
				for sp := d.FirstChild; sp != nil; sp = sp.NextSibling {
					if sp.Type == xhtml.ElementNode && sp.Data == "span" {
						if t := collapseSpace(textContent(sp)); t != "" {
							c.sectors = append(c.sectors, t)
						}
					}
				}
			case d.Data == "p" && HasClass(d, "work_place"):
				c.place = collapseSpace(textContent(d))
			case d.Data == "p" && HasClass(d, "career"):
				c.career = collapseSpace(textContent(d))
			case d.Data == "p" && HasClass(d, "education"):
				c.education = collapseSpace(textContent(d))
			case d.Data == "span" && HasClass(d, "date"):
				c.deadline = collapseSpace(textContent(d))
			case d.Data == "span" && HasClass(d, "deadlines"):
				c.posted = collapseSpace(textContent(d))
			}
			return true
		})
		out = append(out, c)
		return false
	})
	return out
}

// detail reads one posting's body and builds its Job. ok=false with a nil error is the site's
// answer that the posting is gone (404/410). An unreadable posting yields an error.
func (s saramin) detail(ctx context.Context, c saraminCard) (Job, bool, error) {
	if c.title == "" || c.company == "" {
		return Job{}, false, errors.New("listing card is missing its title or employer")
	}
	var root *xhtml.Node
	err := retryWhile(ctx, saraminRetries, s.retryBackoff, saraminRefused, func() error {
		if err := s.limiter.Wait(ctx); err != nil {
			return err
		}
		var err error
		root, err = s.http.GetHTML(ctx, fmt.Sprintf(saraminDetailURL, c.id))
		return err
	})
	if err != nil {
		if !detailUnreadable(err) {
			return Job{}, false, nil
		}
		return Job{}, false, err
	}
	content := firstByClass(root, "user_content")
	if content == nil {
		return Job{}, false, errors.New("page carries no user_content")
	}
	body := ""
	if textContent(content) != "" {
		body = sanitizeHTML(innerHTML(content))
	}
	if strings.TrimSpace(body) == "" {
		// Many employers publish their posting as images alone. The card's own facts are the
		// only text there is, and a stored posting is not asked for again.
		body = saraminFactsBody(c)
	}
	return s.job(c, body), true, nil
}

// saraminFactsBody renders what the card states as a short body.
func saraminFactsBody(c saraminCard) string {
	var b strings.Builder
	b.WriteString("<ul>")
	add := func(label, v string) {
		if v != "" {
			b.WriteString("<li>" + html.EscapeString(label) + ": " + html.EscapeString(v) + "</li>")
		}
	}
	add("직무", strings.Join(c.sectors, ", "))
	add("경력", c.career)
	add("학력", c.education)
	add("근무지역", c.place)
	b.WriteString("</ul>")
	return b.String()
}

// job maps a card and its body to a Job.
func (s saramin) job(c saraminCard, body string) Job {
	now := s.now()
	j := Job{
		ExternalID:          c.id,
		URL:                 fmt.Sprintf(saraminPostingURL, c.id),
		Title:               c.title,
		Company:             c.company,
		Description:         body,
		Location:            strings.TrimSpace(strings.TrimSuffix(c.place, " 외")),
		FallbackCountries:   []string{"kr"},
		PostedAt:            saraminPostedAt(c.posted, now),
		ApplicationDeadline: saraminDeadline(c.deadline, now),
		IsTechHint:          c.tech,
		EducationLevel:      saraminEducation(c.education),
	}
	for _, tok := range strings.Split(c.career, "·") {
		tok = strings.TrimSpace(tok)
		if et := saraminEmploymentType(tok); et != "" {
			j.EmploymentType = et
		}
		if y, ok := saraminMinYears(tok); ok {
			j.ExperienceYearsMin = &y
		}
	}
	return j
}

// saraminEmploymentType maps the listing's employment-type label. A label outside the table
// (병역특례, 교육생, 위촉직) says nothing the vocabulary can hold and is left empty.
func saraminEmploymentType(label string) string {
	switch label {
	case "정규직":
		return "full_time"
	case "계약직", "파견직", "프리랜서":
		return "contract"
	case "인턴":
		return "internship"
	case "아르바이트":
		return "part_time"
	}
	return ""
}

var (
	saraminYearsRe = regexp.MustCompile(`^경력\s*(\d+)\s*(?:년|~)`)
	saraminAgoRe   = regexp.MustCompile(`(\d+)\s*(일|시간|분)\s*전`)
	saraminDateRe  = regexp.MustCompile(`^~\s*(\d{1,2})\.(\d{1,2})`)
	saraminDayRe   = regexp.MustCompile(`^D-(\d+)$`)
)

// saraminMinYears reads the lower bound of "경력 3년↑" or "경력 3~5년"; "경력무관" and the bare
// "경력" and "신입" state none.
func saraminMinYears(tok string) (int, bool) {
	m := saraminYearsRe.FindStringSubmatch(tok)
	if m == nil {
		return 0, false
	}
	y, err := strconv.Atoi(m[1])
	if err != nil || y <= 0 {
		return 0, false
	}
	return y, true
}

// saraminEducation maps the listing's minimum education. Only the levels the vocabulary holds
// are mapped; "고졸↑" and "초대졸↑" are left empty rather than rounded to the nearest.
func saraminEducation(label string) string {
	switch {
	case strings.HasPrefix(label, "학력무관"):
		return "none"
	case strings.HasPrefix(label, "대학교(4년)"):
		return "bachelor"
	case strings.HasPrefix(label, "석사"):
		return "master"
	case strings.HasPrefix(label, "박사"):
		return "phd"
	}
	return ""
}

// saraminKST is Korea's fixed offset; the site states every day in it.
var saraminKST = time.FixedZone("KST", 9*3600)

// saraminPostedAt turns "5일 전 등록" into a date. The listing gives only a distance, so the day
// is the one that many days before today, in Korea.
func saraminPostedAt(label string, now time.Time) *time.Time {
	m := saraminAgoRe.FindStringSubmatch(label)
	if m == nil {
		return nil
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return nil
	}
	var t time.Time
	switch m[2] {
	case "일":
		t = now.In(saraminKST).AddDate(0, 0, -n)
	case "시간":
		t = now.In(saraminKST).Add(-time.Duration(n) * time.Hour)
	default:
		t = now.In(saraminKST).Add(-time.Duration(n) * time.Minute)
	}
	d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	return &d
}

// saraminDeadline reads "~12.19(토)" (a month and day, the year implied) or "D-3". An open-ended
// label ("상시채용", "채용시") carries no deadline.
func saraminDeadline(label string, now time.Time) *time.Time {
	today := now.In(saraminKST)
	day := func(t time.Time) *time.Time {
		d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
		return &d
	}
	if m := saraminDayRe.FindStringSubmatch(label); m != nil {
		n, err := strconv.Atoi(m[1])
		if err != nil {
			return nil
		}
		return day(today.AddDate(0, 0, n))
	}
	m := saraminDateRe.FindStringSubmatch(label)
	if m == nil {
		return nil
	}
	mo, _ := strconv.Atoi(m[1])
	d, _ := strconv.Atoi(m[2])
	if mo < 1 || mo > 12 || d < 1 || d > 31 {
		return nil
	}
	t := time.Date(today.Year(), time.Month(mo), d, 0, 0, 0, 0, time.UTC)
	// A deadline is at most a few months out; one that reads as long past is next year's.
	if t.Before(time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -90)) {
		t = t.AddDate(1, 0, 0)
	}
	return &t
}
