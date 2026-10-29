package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"log"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"

	"github.com/strelov1/freehire/internal/dict/location"
	"github.com/strelov1/freehire/internal/dict/skilltag"
)

// pracuj adapts Pracuj.pl (www.pracuj.pl), Poland's largest general job board: ~63k open offers
// in ~48.5k postings (one posting is offered in several cities as several offers), every trade,
// ~10% of them in the two IT categories. It is boardless and an aggregator — the employer is
// read per posting. Grupa Pracuj's IT board theprotocol.it carries the same postings and is not
// crawled separately. See the "Pracuj.pl traps" section of AGENTS.md for the measurements behind
// each rule.
//
// Cloudflare refuses a plain client on every path, so the whole crawl goes through the Scrapling
// tier's impersonating fetcher (see scraplingProviders). The listing is the site's own search,
// server-rendered with its results in __NEXT_DATA__, walked in a few category slices because the
// unfiltered search fails at depth; each new posting's body comes from its offer page's
// __NEXT_DATA__, bought only for postings the catalogue lacks, newest first, within a per-run
// budget. The rest are refreshed by identity.
type pracuj struct {
	http    HTMLGetter
	limiter waiter
	// slices are the category filters the listing is walked in; see pracujSlices.
	slices       []pracujSlice
	pageSize     int
	maxNewPerRun int
	maxPages     int
	// deadlineMargin is how much of the run's deadline hydration leaves to the pipeline: a
	// hydrating crawl writes nothing until FetchNewGated returns, so one that runs into its
	// deadline stores nothing at all.
	deadlineMargin time.Duration
	// retryBackoff is the first pause before a failed request is asked again; it doubles.
	retryBackoff time.Duration
}

const (
	pracujBaseURL = "https://www.pracuj.pl"
	// pracujListURL is one page of the search, newest first, filtered to a set of categories
	// (the filter is a union: an offer filed under two of them is listed once).
	pracujListURL = pracujBaseURL + "/praca?cc=%s&rop=%d&pn=%d"
	// pracujPageSize is the largest page the search serves; a page is ~1.2 MB of HTML.
	pracujPageSize = 100

	// pracujRequestInterval paces every request of a run, listing and offer page alike, at one a
	// second — a courtesy ceiling, not a measured limit.
	pracujRequestInterval = time.Second
	// pracujWorkers keeps two requests in flight: one round trip through the Scrapling tier is
	// ~1-3 s, so a single worker would read well under the one-a-second pace.
	pracujWorkers = 2
	// pracujRetries bounds how often a failed request is asked again. The search's server render
	// sometimes answers 200 without its results (a deep page of the unfiltered search did so for
	// about half the requests); within a category slice it did not in ~560 pages, and a second
	// try a few seconds later is the cure either way.
	pracujRetries      = 4
	pracujRetryBackoff = 3 * time.Second
	// pracujMaxPages is a safety ceiling per slice: the largest slice was 210 pages on
	// 2026-10-29.
	pracujMaxPages = 600
	// pracujMaxNewPerRun bounds the offer pages of one run: ~560 listing pages and 2,000 offer
	// pages (~470 KB each) at one request a second fill ~45 minutes, inside the 4,500 s run
	// timeout the schedule gives this provider. A cold catalogue is taken in slices, newest
	// first.
	pracujMaxNewPerRun   = 2000
	pracujDeadlineMargin = 5 * time.Minute
)

// pracujSlice is one category filter of the listing walk. tech marks the IT slice: a posting
// listed there was filed by its poster under one of the two IT categories.
type pracujSlice struct {
	categories string
	tech       bool
}

// pracujSlices partitions the site's 35 top-level categories into four searches small enough to
// walk to the end. The unfiltered search (48.5k postings) answers ~half its deep pages without
// results; a search of up to ~21k did not fail once. The slices still overlap (a posting may be
// filed under categories of two slices, ~15% of the walk) and the walk deduplicates by posting.
// 5015 and 5016 are "IT - Administracja" and "IT - Rozwój oprogramowania".
var pracujSlices = []pracujSlice{
	{categories: "5015,5016", tech: true},
	// Manual work, production, supply chain, transport, quality, health and safety, energy.
	{categories: "5022,5024,5017,5031,5034,5004,5036"},
	// Sales, customer service, hospitality, health, call centre, franchise, real estate,
	// insurance, banking, marketing, e-commerce, advertising, PR, media.
	{categories: "5028,5021,5010,5035,5006,5009,5020,5032,5003,5018,5013,5026,5025,5019"},
	// Engineering, office administration, finance, construction, HR, R&D, consulting,
	// education, law, public sector, purchasing, other.
	{categories: "5014,5001,5008,5005,5011,5002,5037,5007,5023,5027,5033,5012"},
}

// pracujTechCategories are the two IT parent categories.
var pracujTechCategories = map[int]bool{5015: true, 5016: true}

// NewPracuj builds the adapter over the given client.
func NewPracuj(c HTMLGetter) Source {
	return pracuj{
		http:           c,
		limiter:        rate.NewLimiter(rate.Every(pracujRequestInterval), 1),
		slices:         pracujSlices,
		pageSize:       pracujPageSize,
		maxNewPerRun:   pracujMaxNewPerRun,
		maxPages:       pracujMaxPages,
		deadlineMargin: pracujDeadlineMargin,
		retryBackoff:   pracujRetryBackoff,
	}
}

func (pracuj) Provider() string { return "pracuj" }

// One national board, no per-tenant id.
func (pracuj) boardless() {}

// Tens of thousands of employers and agencies post here, each named per posting.
func (pracuj) aggregator() {}

// pracujPosting is one search result: a posting and its offers, one per city.
type pracujPosting struct {
	GroupID     string              `json:"groupId"`
	Title       string              `json:"jobTitle"`
	Company     string              `json:"companyName"`
	Initial     string              `json:"initialPublicated"`
	Offers      []pracujListedOffer `json:"offers"`
	RawModes    []string            `json:"workModes"`
	tech        bool
	publishedAt time.Time
}

type pracujListedOffer struct {
	PartitionID int64  `json:"partitionId"`
	URL         string `json:"offerAbsoluteUri"`
	Workplace   string `json:"displayWorkplace"`
}

// pracujNextData is the part of a page's __NEXT_DATA__ the adapter reads: react-query's
// dehydrated cache, one entry per query the server ran.
type pracujNextData struct {
	Props struct {
		PageProps struct {
			DehydratedState struct {
				Queries []struct {
					QueryKey []json.RawMessage `json:"queryKey"`
					State    struct {
						Data json.RawMessage `json:"data"`
					} `json:"state"`
				} `json:"queries"`
			} `json:"dehydratedState"`
		} `json:"pageProps"`
	} `json:"props"`
}

// query returns the data of the dehydrated query whose key opens with name, or nil when the
// server rendered the page without it.
func (nd pracujNextData) query(name string) json.RawMessage {
	for _, q := range nd.Props.PageProps.DehydratedState.Queries {
		if len(q.QueryKey) == 0 {
			continue
		}
		var key string
		if json.Unmarshal(q.QueryKey[0], &key) == nil && key == name && len(q.State.Data) > 0 && string(q.State.Data) != "null" {
			return q.State.Data
		}
	}
	return nil
}

type pracujListPage struct {
	Postings []pracujPosting `json:"groupedOffers"`
	Total    int             `json:"groupedOffersTotalCount"`
}

// errPracujNoResults marks a page the server rendered without the query it exists for. It is
// not an answer — the same page asked again is normally served whole.
var errPracujNoResults = errors.New("page rendered without its results")

// Fetch is the list-only fallback HydratingSource specifies: everything is unseen.
func (s pracuj) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

func (s pracuj) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	return s.FetchNewGated(ctx, e, seen, DetailGate{})
}

// FetchNewGated walks every slice of the search, refreshes the postings already stored, and reads
// up to maxNewPerRun of the rest, newest first. A posting the pipeline will discard anyway is
// judged on its search result and yielded body-less without a request. A posting whose page
// cannot be read is not yielded; a later run tries again.
func (s pracuj) FetchNewGated(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool, gate DetailGate) ([]Job, error) {
	postings, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	var covered map[string]bool
	if gate.Covered != nil {
		companies := make([]string, 0, len(postings))
		for _, p := range postings {
			companies = append(companies, pracujClean(p.Company))
		}
		covered = gate.Covered(companies)
	}
	var jobs []Job
	var unseen []pracujPosting
	deferred := 0
	for _, p := range postings {
		base := pracujBaseJob(p)
		if seen(base.ExternalID) {
			base.SeenRefresh = true
			jobs = append(jobs, base)
			continue
		}
		if covered[base.Company] || (gate.Rejected != nil && gate.Rejected(base)) {
			jobs = append(jobs, base)
			continue
		}
		if len(unseen) >= s.maxNewPerRun {
			deferred++
			continue
		}
		unseen = append(unseen, p)
	}
	var answered, outOfTime atomic.Int64
	read := fetchDetails(unseen, pracujWorkers, func(p pracujPosting) (Job, bool) {
		if s.nearDeadline(ctx) {
			outOfTime.Add(1)
			return Job{}, false
		}
		job, ok, err := s.detail(ctx, p)
		if err != nil {
			log.Printf("pracuj: posting %s failed; deferring: %v", p.GroupID, err)
			return Job{}, false
		}
		answered.Add(1)
		return job, ok
	})
	// A crawl that read no page at all is broken, not quiet. "Answered" counts a posting the
	// site reported gone; postings left for want of time are not failures.
	if int64(len(unseen)) > outOfTime.Load() && answered.Load() == 0 {
		return nil, fmt.Errorf("pracuj: requested %d offer pages and none answered", int64(len(unseen))-outOfTime.Load())
	}
	if n := outOfTime.Load(); n > 0 {
		log.Printf("pracuj: %d new postings left unread as the run neared its deadline, deferred to later runs", n)
	}
	if deferred > 0 {
		log.Printf("pracuj: %d new postings past this run's bound of %d, deferred to later runs", deferred, s.maxNewPerRun)
	}
	return append(jobs, read...), nil
}

// nearDeadline reports whether the run's deadline is closer than the margin hydration leaves to
// the pipeline's own writes.
func (s pracuj) nearDeadline(ctx context.Context) bool {
	d, ok := ctx.Deadline()
	return ok && time.Until(d) < s.deadlineMargin
}

// list walks every slice to its end and returns the postings with a named employer, newest first.
// A posting listed by two slices is kept once, marked technical if the IT slice listed it. Any
// page failing fails the crawl — a partial listing would let the company-scoped sweep close the
// postings of the employers it did see on the pages it did not.
func (s pracuj) list(ctx context.Context) ([]pracujPosting, error) {
	byID := make(map[string]*pracujPosting)
	var order []string
	for _, sl := range s.slices {
		pages, err := s.walkSlice(ctx, sl)
		if err != nil {
			return nil, err
		}
		for _, page := range pages {
			for _, p := range page {
				if p.GroupID == "" || len(p.Offers) == 0 || pracujOfferURL(p.Offers[0]) == "" ||
					pracujClean(p.Title) == "" || pracujClean(p.Company) == "" {
					continue
				}
				if prev, ok := byID[p.GroupID]; ok {
					prev.tech = prev.tech || sl.tech
					continue
				}
				p.tech = sl.tech
				if t := parseRFC3339(strings.TrimSpace(p.Initial)); t != nil {
					p.publishedAt = *t
				}
				byID[p.GroupID] = &p
				order = append(order, p.GroupID)
			}
		}
	}
	if len(order) == 0 {
		return nil, fmt.Errorf("pracuj: listing carries no posting")
	}
	out := make([]pracujPosting, 0, len(order))
	for _, id := range order {
		out = append(out, *byID[id])
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].publishedAt.After(out[j].publishedAt) })
	return out, nil
}

// walkSlice reads one slice: its first page states the slice's total, and the rest are read two
// at a time. A page past the end answers no posting; postings closing during the walk can bring
// the end a little before the total.
func (s pracuj) walkSlice(ctx context.Context, sl pracujSlice) ([][]pracujPosting, error) {
	first, err := s.listPage(ctx, sl, 1)
	if err != nil {
		return nil, err
	}
	n := (first.Total + s.pageSize - 1) / s.pageSize
	if n > s.maxPages {
		return nil, fmt.Errorf("pracuj: slice %s states %d postings, past the %d-page ceiling", sl.categories, first.Total, s.maxPages)
	}
	pages := make([][]pracujPosting, max(n, 1))
	pages[0] = first.Postings
	var mu sync.Mutex
	var firstErr error
	sem := make(chan struct{}, pracujWorkers)
	var wg sync.WaitGroup
	for pn := 2; pn <= n; pn++ {
		mu.Lock()
		failed := firstErr != nil
		mu.Unlock()
		if failed {
			break
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(pn int) {
			defer wg.Done()
			defer func() { <-sem }()
			p, err := s.listPage(ctx, sl, pn)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				return
			}
			pages[pn-1] = p.Postings
		}(pn)
	}
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	return pages, nil
}

// listPage reads one page of a slice, asking again when the server rendered it without results.
func (s pracuj) listPage(ctx context.Context, sl pracujSlice, pn int) (pracujListPage, error) {
	var page pracujListPage
	err := s.retry(ctx, func() error {
		nd, err := s.nextData(ctx, fmt.Sprintf(pracujListURL, sl.categories, s.pageSize, pn))
		if err != nil {
			return err
		}
		raw := nd.query("jobOffers")
		if raw == nil {
			return errPracujNoResults
		}
		page = pracujListPage{}
		return json.Unmarshal(raw, &page)
	})
	if err != nil {
		return pracujListPage{}, fmt.Errorf("pracuj: slice %s page %d: %w", sl.categories, pn, err)
	}
	return page, nil
}

// nextData fetches a page and decodes its __NEXT_DATA__.
func (s pracuj) nextData(ctx context.Context, url string) (pracujNextData, error) {
	root, err := s.http.GetHTML(ctx, url)
	if err != nil {
		return pracujNextData{}, err
	}
	raw := scriptTextByID(root, "__NEXT_DATA__")
	if raw == "" {
		return pracujNextData{}, errPracujNoResults
	}
	var nd pracujNextData
	if err := json.Unmarshal([]byte(raw), &nd); err != nil {
		return pracujNextData{}, fmt.Errorf("decode __NEXT_DATA__: %w", err)
	}
	return nd, nil
}

// retry paces and runs call, asking again after a page rendered without its results, a refusal
// (403/429) or a server failure, none of which the Scrapling client retries itself.
func (s pracuj) retry(ctx context.Context, call func() error) error {
	return retryWhile(ctx, pracujRetries, s.retryBackoff, pracujRetryable, func() error {
		if err := s.limiter.Wait(ctx); err != nil {
			return err
		}
		return call()
	})
}

func pracujRetryable(err error) bool {
	if errors.Is(err, errPracujNoResults) {
		return true
	}
	var se *StatusError
	return errors.As(err, &se) && (se.Code == http.StatusForbidden || se.Code == http.StatusTooManyRequests || se.Code >= 500)
}

func pracujClean(s string) string { return strings.Join(strings.Fields(s), " ") }

// pracujOfferURL is an offer's page on the site's own host, or "" for anything else.
func pracujOfferURL(o pracujListedOffer) string {
	u := strings.TrimSpace(o.URL)
	if !strings.HasPrefix(u, pracujBaseURL+"/praca/") || !strings.Contains(u, ",oferta,") {
		return ""
	}
	return u
}

// pracujBaseJob is the body-less Job a search result alone supports: the identity, and the
// facts the gate judges on. The posting (groupId) is the identity, not the per-city offer: one
// posting offered in five cities is one job.
func pracujBaseJob(p pracujPosting) Job {
	j := Job{
		ExternalID:        p.GroupID,
		URL:               pracujOfferURL(p.Offers[0]),
		Title:             pracujClean(p.Title),
		Company:           pracujClean(p.Company),
		Location:          pracujClean(p.Offers[0].Workplace),
		IsTechHint:        p.tech,
		FallbackCountries: []string{"pl"},
	}
	if m := pracujWorkMode(p.RawModes); m != "" {
		j.WorkMode, j.Remote = m, m == "remote"
	}
	if !p.publishedAt.IsZero() {
		t := p.publishedAt
		j.PostedAt = NotFuture(&t)
	}
	return j
}

// pracujOffer is the part of an offer page's jobOffer query the adapter reads. Names are in the
// posting's own language (an English posting's page says "B2B contract"), so only ids and codes
// are matched.
type pracujOffer struct {
	Publication struct {
		Initial  string `json:"dateOfInitialPublicationUtc"`
		IsActive bool   `json:"isActive"`
	} `json:"publicationDetails"`
	Attributes struct {
		Title       string `json:"jobTitle"`
		IsArchive   bool   `json:"isArchive"`
		IsWithdrawn bool   `json:"isWithdrawn"`
		Employer    string `json:"displayEmployerName"`
		Leading     struct {
			Parent struct {
				ID int `json:"id"`
			} `json:"parent"`
		} `json:"leadingCategory"`
		Workplaces []struct {
			IsAbroad bool `json:"isAbroad"`
			Country  struct {
				Name string `json:"name"`
			} `json:"country"`
			Inland *struct {
				Location struct {
					Name string `json:"name"`
				} `json:"location"`
			} `json:"inlandLocation"`
			Display string `json:"displayAddress"`
		} `json:"workplaces"`
		Employment struct {
			PositionLevels []struct {
				ID int `json:"id"`
			} `json:"positionLevels"`
			EntirelyRemote bool `json:"entirelyRemoteWork"`
			WorkSchedules  []struct {
				ID int `json:"id"`
			} `json:"workSchedules"`
			Contracts []pracujContract `json:"typesOfContracts"`
			WorkModes []struct {
				Code string `json:"code"`
			} `json:"workModes"`
		} `json:"employment"`
	} `json:"attributes"`
	Sections []pracujSection `json:"sections"`
}

type pracujContract struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Salary *struct {
		From     *float64 `json:"from"`
		To       *float64 `json:"to"`
		Currency struct {
			Code string `json:"code"`
		} `json:"currency"`
		TimeUnit struct {
			ID       int `json:"id"`
			LongForm struct {
				Name string `json:"name"`
			} `json:"longForm"`
		} `json:"timeUnit"`
		Kind struct {
			Code string `json:"code"`
			Name string `json:"name"`
		} `json:"salaryKind"`
	} `json:"salary"`
}

// pracujSection is one block of the posting's body: a list, paragraphs or a dictionary of named
// items (technologies, benefits), possibly with sub-sections.
type pracujSection struct {
	Type  string          `json:"sectionType"`
	Title string          `json:"title"`
	Subs  []pracujSection `json:"subSections"`
	Model *struct {
		Type       string   `json:"modelType"`
		Bullets    []string `json:"bullets"`
		Paragraphs []string `json:"paragraphs"`
		Items      []struct {
			Name string `json:"name"`
		} `json:"items"`
		CustomItems []struct {
			Name string `json:"name"`
		} `json:"customItems"`
	} `json:"model"`
}

// detail reads one posting's first offer page. ok=false with a nil error is the site's answer
// that the posting is gone (404/410, or archived, withdrawn or inactive) or carries no usable
// posting; an error is the absence of an answer.
func (s pracuj) detail(ctx context.Context, p pracujPosting) (Job, bool, error) {
	var o pracujOffer
	err := s.retry(ctx, func() error {
		nd, err := s.nextData(ctx, pracujOfferURL(p.Offers[0]))
		if err != nil {
			return err
		}
		raw := nd.query("jobOffer")
		if raw == nil {
			return errPracujNoResults
		}
		o = pracujOffer{}
		return json.Unmarshal(raw, &o)
	})
	if err != nil {
		var se *StatusError
		if errors.As(err, &se) && (se.Code == http.StatusNotFound || se.Code == http.StatusGone) {
			return Job{}, false, nil
		}
		return Job{}, false, err
	}
	a := o.Attributes
	if a.IsArchive || a.IsWithdrawn || !o.Publication.IsActive {
		return Job{}, false, nil
	}
	job, ok := pracujJob(o, p)
	return job, ok, nil
}

// pracujJob maps an offer page and its search result to a Job. ok is false for one with no
// title, employer or body.
func pracujJob(o pracujOffer, p pracujPosting) (Job, bool) {
	a := o.Attributes
	j := pracujBaseJob(p)
	if t := pracujClean(a.Title); t != "" {
		j.Title = t
	}
	if co := pracujClean(a.Employer); co != "" {
		j.Company = co
	}
	body := pracujBody(o.Sections)
	if pay := pracujPayParagraph(a.Employment.Contracts); pay != "" {
		body = pay + body
	}
	j.Description = sanitizeHTML(body)
	if j.Title == "" || j.Company == "" || strings.TrimSpace(j.Description) == "" {
		return Job{}, false
	}
	j.Location, j.Countries = pracujPlace(o)
	if j.Location == "" {
		j.Location = pracujClean(p.Offers[0].Workplace)
	}
	if t := parseRFC3339(strings.TrimSpace(o.Publication.Initial)); t != nil {
		j.PostedAt = NotFuture(t)
	}
	codes := make([]string, 0, len(a.Employment.WorkModes))
	for _, m := range a.Employment.WorkModes {
		codes = append(codes, m.Code)
	}
	if a.Employment.EntirelyRemote {
		codes = append(codes, "home-office")
	}
	if m := pracujWorkMode(codes); m != "" {
		j.WorkMode, j.Remote = m, m == "remote"
	}
	if pracujTechCategories[a.Leading.Parent.ID] {
		j.IsTechHint = true
	}
	levels := make([]int, 0, len(a.Employment.PositionLevels))
	for _, l := range a.Employment.PositionLevels {
		levels = append(levels, l.ID)
	}
	j.Seniority = pracujSeniority(levels)
	j.EmploymentType = pracujEmploymentType(a.Employment.WorkSchedules, a.Employment.Contracts)
	j.Skills = pracujSkills(o.Sections)
	if lo, hi, period, ok := pracujSalary(a.Employment.Contracts); ok {
		j.SalaryMin, j.SalaryMax, j.SalaryCurrency, j.SalaryPeriod = &lo, &hi, "PLN", period
	}
	return j, true
}

// pracujPlace reads the posting's first workplace as its location, and every workplace's country:
// one in Poland is "pl", one abroad is the country its page names (in the posting's language, so
// a Polish page's "Niemcy" resolves to nothing and is left to the dictionaries).
func pracujPlace(o pracujOffer) (string, []string) {
	var loc string
	var countries []string
	for _, w := range o.Attributes.Workplaces {
		code := "pl"
		if w.IsAbroad {
			code = location.NormalizeCountry(strings.TrimSpace(w.Country.Name))
		}
		if code != "" && !slices.Contains(countries, code) {
			countries = append(countries, code)
		}
		if loc != "" {
			continue
		}
		if w.Inland != nil && strings.TrimSpace(w.Inland.Location.Name) != "" {
			loc = pracujClean(w.Inland.Location.Name)
		} else {
			loc = pracujClean(w.Display)
		}
	}
	return loc, countries
}

// pracujWorkMode reads the posting's stated arrangements, the most remote one winning, from the
// search result's labels ("Praca zdalna", "Praca hybrydowa (do uzgodnienia)") or the offer page's
// codes ("home-office", "hybrid", "full-office"). "Praca mobilna" (field work) says nothing.
func pracujWorkMode(modes []string) string {
	set := map[string]bool{}
	for _, m := range modes {
		m = strings.ToLower(pracujClean(m))
		switch {
		case m == "home-office" || strings.HasPrefix(m, "praca zdalna"):
			set["remote"] = true
		case m == "hybrid" || strings.HasPrefix(m, "praca hybrydowa"):
			set["hybrid"] = true
		case m == "full-office" || strings.HasPrefix(m, "praca stacjonarna"):
			set["onsite"] = true
		}
	}
	for _, m := range []string{"remote", "hybrid", "onsite"} {
		if set[m] {
			return m
		}
	}
	return ""
}

// pracujSeniorityByLevel maps the site's position-level ids. Assistant, coordinator, manager,
// director and manual worker name a role, not a seniority, and map to nothing.
var pracujSeniorityByLevel = map[int]string{
	1:  "intern", // praktykant / stażysta
	17: "junior", // młodszy specjalista (junior)
	4:  "middle", // specjalista (mid / regular)
	18: "senior", // starszy specjalista (senior)
	19: "senior", // ekspert
	21: "c_level",
}

// pracujSeniority is the seniority every stated level agrees on, or "" when they disagree ("mid /
// senior" is two levels) or none maps.
func pracujSeniority(levels []int) string {
	out := ""
	for _, l := range levels {
		s := pracujSeniorityByLevel[l]
		if s == "" || (out != "" && s != out) {
			return ""
		}
		out = s
	}
	return out
}

// pracujEmploymentType maps a single stated schedule (full-time 0, part-time 1); an internship
// contract (7) as the only contract wins over the schedule.
func pracujEmploymentType(schedules []struct {
	ID int `json:"id"`
}, contracts []pracujContract) string {
	if len(contracts) == 1 && contracts[0].ID == 7 {
		return "internship"
	}
	if len(schedules) != 1 {
		return ""
	}
	switch schedules[0].ID {
	case 0:
		return "full_time"
	case 1:
		return "part_time"
	}
	return ""
}

// pracujContractUoP is "umowa o pracę", the contract of employment; pracujContractB2B is the B2B
// contract, paid net of VAT.
const (
	pracujContractUoP = 0
	pracujContractB2B = 3
)

// pracujSalary reads one stated salary: the contract of employment's gross pay if stated, else
// another gross one, else a B2B contract's net pay — the order in which a figure is comparable to
// the rest of the catalogue. Only PLN, with a stated kind (gross, or net for B2B) and a monthly
// (0) or hourly (1) period. Every contract's pay is also written into the body (pracujPayParagraph).
func pracujSalary(contracts []pracujContract) (lo, hi int, period string, ok bool) {
	pick := func(match func(pracujContract) bool) (int, int, string, bool) {
		for _, c := range contracts {
			if !match(c) {
				continue
			}
			if lo, hi, period, ok := pracujContractPay(c); ok {
				return lo, hi, period, true
			}
		}
		return 0, 0, "", false
	}
	gross := func(c pracujContract) bool { return c.Salary != nil && c.Salary.Kind.Code == "gross" }
	if lo, hi, period, ok = pick(func(c pracujContract) bool { return c.ID == pracujContractUoP && gross(c) }); ok {
		return
	}
	if lo, hi, period, ok = pick(gross); ok {
		return
	}
	return pick(func(c pracujContract) bool {
		return c.ID == pracujContractB2B && c.Salary != nil && strings.HasPrefix(c.Salary.Kind.Code, "net")
	})
}

// pracujContractPay is one contract's structured pay, or ok=false when it is not a PLN range with
// a monthly or hourly period. A single figure is stated as from == to.
func pracujContractPay(c pracujContract) (lo, hi int, period string, ok bool) {
	s := c.Salary
	if s == nil || s.From == nil || !strings.EqualFold(strings.TrimSpace(s.Currency.Code), "PLN") {
		return 0, 0, "", false
	}
	switch s.TimeUnit.ID {
	case 0:
		period = "month"
	case 1:
		period = "hour"
	default:
		return 0, 0, "", false
	}
	from, to := *s.From, *s.From
	if s.To != nil {
		to = *s.To
	}
	lo, hi = int(from+0.5), int(to+0.5)
	if lo <= 0 || hi < lo {
		return 0, 0, "", false
	}
	return lo, hi, period, true
}

// pracujPayParagraph states every contract's pay as the posting does — amount, gross or net,
// period and contract — because the salary fields hold one figure and drop which contract and
// tax basis it was.
func pracujPayParagraph(contracts []pracujContract) string {
	var lines []string
	for _, c := range contracts {
		lo, hi, _, ok := pracujContractPay(c)
		if !ok {
			continue
		}
		amount := strconv.Itoa(lo)
		if hi != lo {
			amount += "–" + strconv.Itoa(hi)
		}
		line := fmt.Sprintf("%s PLN %s / %s (%s)", amount, pracujClean(c.Salary.Kind.Name), pracujClean(c.Salary.TimeUnit.LongForm.Name), pracujClean(c.Name))
		lines = append(lines, "<li>"+html.EscapeString(line)+"</li>")
	}
	if len(lines) == 0 {
		return ""
	}
	return "<ul>" + strings.Join(lines, "") + "</ul>\n"
}

// pracujBody renders the posting's sections as HTML: a heading per titled section, bullets as a
// list, paragraphs as paragraphs, and a dictionary (technologies, benefits) as a list of its
// item names. A gallery is skipped.
func pracujBody(sections []pracujSection) string {
	var b strings.Builder
	var render func(secs []pracujSection, depth int)
	render = func(secs []pracujSection, depth int) {
		for _, sec := range secs {
			if t := pracujClean(sec.Title); t != "" {
				tag := "h2"
				if depth > 0 {
					tag = "h3"
				}
				b.WriteString("<" + tag + ">" + html.EscapeString(t) + "</" + tag + ">\n")
			}
			if m := sec.Model; m != nil {
				var items []string
				switch {
				case len(m.Bullets) > 0:
					items = m.Bullets
				case len(m.Items) > 0 || len(m.CustomItems) > 0:
					for _, it := range m.Items {
						items = append(items, it.Name)
					}
					for _, it := range m.CustomItems {
						items = append(items, it.Name)
					}
				}
				if len(items) > 0 {
					b.WriteString("<ul>")
					for _, it := range items {
						if it = strings.TrimSpace(it); it != "" {
							b.WriteString("<li>" + html.EscapeString(it) + "</li>")
						}
					}
					b.WriteString("</ul>\n")
				}
				for _, para := range m.Paragraphs {
					if para = strings.TrimSpace(para); para != "" {
						b.WriteString("<p>" + strings.ReplaceAll(html.EscapeString(para), "\n", "<br>") + "</p>\n")
					}
				}
			}
			render(sec.Subs, depth+1)
		}
	}
	render(sections, 0)
	return b.String()
}

// pracujSkills canonicalizes the technologies the poster picked (expected and optional) through
// the skill dictionary. Canonicalize, not Parse: these are discrete names from a structured
// field, not prose to mine.
func pracujSkills(sections []pracujSection) []string {
	var names []string
	var collect func(secs []pracujSection)
	collect = func(secs []pracujSection) {
		for _, sec := range secs {
			if strings.HasPrefix(sec.Type, "technologies") && sec.Model != nil {
				for _, it := range sec.Model.Items {
					names = append(names, it.Name)
				}
				for _, it := range sec.Model.CustomItems {
					names = append(names, it.Name)
				}
			}
			collect(sec.Subs)
		}
	}
	collect(sections)
	if len(names) == 0 {
		return nil
	}
	return skilltag.Canonicalize(names)
}
