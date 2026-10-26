package sources

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/platform/flexjson"
)

// mynavi adapts マイナビ転職 (tenshoku.mynavi.jp), one of Japan's largest job boards. Board-based
// aggregator: a board is one of the site's occupation codes in its engineer edition
// ("エンジニア求人サーチ", /engineer/list/<code>/) — o16 for IT engineers, o17 for electrical,
// electronic, mechanical and semiconductor engineers. The main /list/ edition sends both codes
// there with a redirect, so the engineer edition is the only place they can be listed.
//
// The listing is server-rendered HTML, 50 cards a page. It is filtered to the site's own direct
// postings (employment types e01+e02+e03; e05 is the agency type): unfiltered, the edition is
// 94% agency referrals (人材紹介) that link off-site to a separate service and hide the employer
// as 社名非公開. A card carries everything but the body, so this is a HydratingSource that buys a
// posting's body from its detail page's schema.org JobPosting only when the catalogue lacks it,
// up to mynaviDetailBudget a run. See the Mynavi traps in AGENTS.md.
type mynavi struct {
	http HTMLGetter
	// spent counts the detail requests claimed across every board of the run, shared by the
	// adapter's copies (boards may be crawled concurrently, hence atomic).
	spent *atomic.Int64
}

const (
	mynaviBase = "https://tenshoku.mynavi.jp"
	// mynaviDirectTypes is the employment-type filter that keeps the site's own postings.
	mynaviDirectTypes = "e01+e02+e03"
	// mynaviMaxPages is a safety ceiling, not the platform's: o16 held 6,802 direct postings
	// (137 pages) on 2026-10-25. A walk that reaches it is logged.
	mynaviMaxPages = 400
	// The crawl egresses through the proxy (the origin refuses datacenter IPs), where a page
	// takes ~3.5 s, so the pace is a ceiling the workers rarely reach.
	mynaviRequestInterval = time.Second
	mynaviRequestBurst    = 1
	mynaviDetailWorkers   = 2
	// mynaviDetailBudget bounds one run's detail requests. Measured on prod 2026-10-26, a run of
	// o16 alone (137 listing pages + 800 details) took 16 minutes; with o17's ~180 listing pages
	// and its own share of details a run stays near 35 minutes, inside the scheduler's 3000 s.
	mynaviDetailBudget = 1600
	// mynaviBoardDetailShare caps one board's claim on the run's budget: half of it, so each of
	// the two boards gets its share whichever is crawled first and neither cold backlog starves
	// the other.
	mynaviBoardDetailShare = 800
)

// mynaviBoards are the occupation codes the engineer edition serves, and whether a posting
// filed under one is technical by the site's own filing. o17 is hardware engineering, which
// this catalogue does not count as IT, so its postings are left to the title dictionary.
var mynaviBoards = map[string]bool{"o16": true, "o17": false}

// NewMynavi builds the Mynavi adapter over the given HTTP client.
func NewMynavi(c HTMLGetter) Source { return mynavi{http: c, spent: new(atomic.Int64)} }

func (mynavi) Provider() string { return "mynavi" }

// Every posting names its own employer.
func (mynavi) aggregator() {}

func (s mynavi) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

func (s mynavi) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	return s.FetchNewGated(ctx, e, seen, DetailGate{})
}

// FetchNewGated is FetchNew that buys no body for a posting the pipeline will discard anyway.
func (s mynavi) FetchNewGated(ctx context.Context, e CompanyEntry, seen func(externalID string) bool, gate DetailGate) ([]Job, error) {
	tech, ok := mynaviBoards[e.Board]
	if !ok {
		return nil, fmt.Errorf("mynavi: board %q is not an engineer-edition occupation code (o16, o17)", e.Board)
	}
	cards, err := s.list(ctx, e.Board)
	if err != nil {
		return nil, err
	}
	return s.hydrate(ctx, cards, tech, seen, gate)
}

// mynaviCard is one listing card, reduced to what the adapter reads.
type mynaviCard struct {
	ID      string
	Title   string
	Company string
	Updated string // 情報更新日, YYYY/MM/DD
}

func mynaviPageURL(board string, page int) string {
	u := fmt.Sprintf("%s/engineer/list/%s/%s/", mynaviBase, board, mynaviDirectTypes)
	if page > 1 {
		u += fmt.Sprintf("pg%d/", page)
	}
	return u
}

// list walks the board to its end: a page with no cards. The first page failing fails the board,
// and so does a first page that carries no result counter — that is not a results page at all
// (selector drift, or a code the edition does not serve). A later page failing ends the walk
// with what was read.
func (s mynavi) list(ctx context.Context, board string) ([]mynaviCard, error) {
	var out []mynaviCard
	ids := map[string]bool{}
	total := 0
	for page := 1; page <= mynaviMaxPages; page++ {
		root, err := s.http.GetHTML(ctx, mynaviPageURL(board, page))
		if err != nil {
			if page == 1 {
				return nil, fmt.Errorf("mynavi: board %s: %w", board, err)
			}
			log.Printf("mynavi: board %s: page %d failed; keeping %d postings read: %v", board, page, len(out), err)
			break
		}
		cards := mynaviCards(root)
		if page == 1 {
			n, ok := mynaviTotal(root)
			if !ok && len(cards) > 0 {
				return nil, fmt.Errorf("mynavi: board %s: unrecognized listing page: no result counter", board)
			}
			total = n
		}
		if len(cards) == 0 {
			break
		}
		for _, c := range cards {
			if ids[c.ID] {
				continue
			}
			ids[c.ID] = true
			out = append(out, c)
		}
		if page == mynaviMaxPages {
			log.Printf("mynavi: board %s: stopped at the %d-page ceiling with %d of %d postings read",
				board, mynaviMaxPages, len(out), total)
		}
	}
	return out, nil
}

var (
	mynaviTotalRE  = regexp.MustCompile(`全\s*([\d,]+)\s*件`)
	mynaviJobIDRE  = regexp.MustCompile(`/jobinfo-(\d+-\d+-\d+-\d+)/`)
	mynaviUpdateRE = regexp.MustCompile(`(\d{4})/(\d{2})/(\d{2})`)
)

// mynaviTotal reads the page's own count of matching postings ("全6,802件中").
func mynaviTotal(root *html.Node) (int, bool) {
	pager := firstByClass(root, "pager_text")
	if pager == nil {
		return 0, false
	}
	m := mynaviTotalRE.FindStringSubmatch(textContent(pager))
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(strings.ReplaceAll(m[1], ",", ""))
	return n, err == nil
}

// mynaviCards reads every card on a listing page. A card that does not link a posting on the site
// itself (an agency referral, which links to a separate service) is not a posting this adapter
// can hydrate, and is skipped.
func mynaviCards(root *html.Node) []mynaviCard {
	var out []mynaviCard
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode || n.Data != "section" || !HasClass(n, "recruit") {
			return true
		}
		if c, ok := mynaviReadCard(n); ok {
			out = append(out, c)
		}
		return false
	})
	return out
}

func mynaviReadCard(n *html.Node) (mynaviCard, bool) {
	var c mynaviCard
	if main := firstByClass(n, "main_title"); main != nil {
		// "{company} | {catch copy}"
		company, _, _ := strings.Cut(textContent(main), " | ")
		c.Company = strings.TrimSpace(company)
	}
	if sub := firstByClass(n, "sub_title"); sub != nil {
		if a := firstByTag(sub, "a"); a != nil {
			c.Title = strings.Join(strings.Fields(textContent(a)), " ")
			c.ID = firstSubmatch(mynaviJobIDRE, Attr(a, "href"))
		}
	}
	if u := firstByClass(n, "update"); u != nil {
		c.Updated = textContent(u)
	}
	return c, c.ID != "" && c.Title != ""
}

// hydrate maps the listed cards to Jobs. A seen posting is a liveness refresh with no request, and
// so is a posting the gate condemns; every other posting costs one detail request, up to the
// board's share of the run's budget, claimed in the site's own listing order
// before any request goes out. A detail that could not be read, or that the budget did not
// reach, defers its posting to a later run rather than storing it body-less, which would never be
// hydrated again.
func (s mynavi) hydrate(ctx context.Context, cards []mynaviCard, tech bool, seen func(string) bool, gate DetailGate) ([]Job, error) {
	var covered map[string]bool
	if gate.Covered != nil {
		companies := make([]string, 0, len(cards))
		for _, c := range cards {
			companies = append(companies, c.Company)
		}
		covered = gate.Covered(companies)
	}
	var jobs, claimed []Job
	board, deferred := 0, 0
	for _, c := range cards {
		base, ok := mynaviBaseJob(c, tech)
		if !ok {
			continue
		}
		if seen(base.ExternalID) {
			base.SeenRefresh = true
			jobs = append(jobs, base)
			continue
		}
		if covered[base.Company] || (gate.Rejected != nil && gate.Rejected(base)) {
			jobs = append(jobs, base)
			continue
		}
		if board >= mynaviBoardDetailShare || s.spent.Add(1) > mynaviDetailBudget {
			deferred++
			continue
		}
		board++
		claimed = append(claimed, base)
	}
	var answered atomic.Int64
	jobs = append(jobs, fetchDetails(claimed, mynaviDetailWorkers, func(base Job) (Job, bool) {
		job, ok, err := s.withDetail(ctx, base)
		if err != nil {
			log.Printf("mynavi: detail %s failed; deferring: %v", base.ExternalID, err)
			return Job{}, false
		}
		answered.Add(1)
		return job, ok
	})...)
	// A crawl that asked for details and got no answer to any of them is broken, not quiet.
	if len(claimed) > 0 && answered.Load() == 0 {
		return nil, fmt.Errorf("mynavi: requested %d posting details and none answered", len(claimed))
	}
	if deferred > 0 {
		log.Printf("mynavi: detail budget reached (%d a board, %d a run); %d new postings deferred to later runs",
			mynaviBoardDetailShare, mynaviDetailBudget, deferred)
	}
	return jobs, nil
}

// mynaviHiddenEmployer is the placeholder a posting that withholds its employer carries; filing
// it as a company would build one bogus employer out of every such posting.
const mynaviHiddenEmployer = "社名非公開"

// mynaviBaseJob maps a card to a body-less Job. The site is Japan's domestic market, so Japan is
// the fallback country of every posting; the detail states it per place.
func mynaviBaseJob(c mynaviCard, tech bool) (Job, bool) {
	if c.Company == "" || c.Company == mynaviHiddenEmployer {
		return Job{}, false
	}
	j := Job{
		ExternalID:        c.ID,
		URL:               mynaviBase + "/jobinfo-" + c.ID + "/",
		Title:             c.Title,
		Company:           c.Company,
		IsTechHint:        tech,
		FallbackCountries: []string{"jp"},
	}
	if m := mynaviUpdateRE.FindStringSubmatch(c.Updated); m != nil {
		j.PostedAt = NotFuture(parseDate(m[1] + "-" + m[2] + "-" + m[3]))
	}
	return j, true
}

// mynaviPosting is the detail page's schema.org JobPosting, reduced to what the adapter reads.
type mynaviPosting struct {
	Title              string `json:"title"`
	URL                string `json:"url"`
	Description        string `json:"description"`
	Experience         string `json:"experienceRequirements"`
	WorkHours          string `json:"workHours"`
	Benefits           string `json:"jobBenefits"`
	EmploymentType     string `json:"employmentType"`
	DatePosted         string `json:"datePosted"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
	// A single-place posting carries one Place object, a multi-place one an array.
	JobLocation schemaPlaces `json:"jobLocation"`
	BaseSalary  struct {
		Currency string `json:"currency"`
		Value    struct {
			Min  flexjson.Int64 `json:"minValue"`
			Max  flexjson.Int64 `json:"maxValue"`
			Unit string         `json:"unitText"`
		} `json:"value"`
	} `json:"baseSalary"`
}

// mynaviEmploymentTypes maps the schema.org enum the site emits onto vocab.EmploymentTypeValues.
// OTHER (業務委託 and the dispatch types) has no freehire equivalent and is left unset.
var mynaviEmploymentTypes = map[string]string{
	"FULL_TIME":  "full_time",
	"PART_TIME":  "part_time",
	"CONTRACTOR": "contract",
}

// mynaviEscapedBreak undoes the site's double escaping: the description's line breaks arrive as
// the text "&lt;br&gt;" after one HTML decode, which would reach the reader literally.
var mynaviEscapedBreak = strings.NewReplacer("&amp;lt;br&amp;gt;", "<br>", "&lt;br&gt;", "<br>")

// withDetail adds the posting's body, places, salary and employment type. ok=false with a nil
// error is the platform's own answer that the posting is gone: the site answers 404 once a
// posting passes its listing end date.
func (s mynavi) withDetail(ctx context.Context, base Job) (Job, bool, error) {
	root, err := s.http.GetHTML(ctx, base.URL)
	if err != nil {
		var se *StatusError
		if errors.As(err, &se) && (se.Code == http.StatusNotFound || se.Code == http.StatusGone) {
			return Job{}, false, nil
		}
		return Job{}, false, err
	}
	var p mynaviPosting
	if !ldJobPosting(root, &p) {
		return Job{}, false, errors.New("page carries no JobPosting")
	}
	if p.URL != "" && !strings.Contains(p.URL, "/jobinfo-"+base.ExternalID+"/") {
		return Job{}, false, fmt.Errorf("page answered posting %s", p.URL)
	}
	body := sanitizeHTML(mynaviEscapedBreak.Replace(p.Description) +
		headedSection("応募資格", p.Experience) +
		headedSection("勤務時間", p.WorkHours) +
		headedSection("福利厚生", p.Benefits))
	if strings.TrimSpace(body) == "" {
		return Job{}, false, errors.New("posting carries no body")
	}
	base.Description = body
	if t := strings.TrimSpace(p.Title); t != "" {
		base.Title = t
	}
	if t := parseDate(p.DatePosted); t != nil {
		base.PostedAt = NotFuture(t)
	}
	base.EmploymentType = mynaviEmploymentTypes[p.EmploymentType]
	var places []string
	inJapan := false
	for _, l := range p.JobLocation {
		a := l.Address
		if strings.EqualFold(a.AddressCountry, "JP") {
			inJapan = true
		}
		if place := joinNonEmpty(strings.TrimSpace(a.AddressLocality), strings.TrimSpace(a.AddressRegion)); place != "" {
			places = append(places, place)
		}
	}
	base.Location = strings.Join(places, "; ")
	if inJapan {
		base.Countries = []string{"jp"}
	}
	mynaviSalary(&base, p)
	return base, true, nil
}

// mynaviSalary copies the posting's structured salary: whole yen, a year on every posting
// observed. A posting that states only a floor keeps the floor and no ceiling.
func mynaviSalary(j *Job, p mynaviPosting) {
	v := p.BaseSalary.Value
	if p.BaseSalary.Currency != "JPY" || !strings.EqualFold(v.Unit, "YEAR") {
		return
	}
	lo, hi := int(v.Min), int(v.Max)
	if lo <= 0 {
		return
	}
	j.SalaryMin = &lo
	if hi >= lo {
		j.SalaryMax = &hi
	}
	j.SalaryCurrency = "JPY"
	j.SalaryPeriod = "year"
}
