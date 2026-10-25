package sources

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

// hirist adapts the job platform behind hirist.tech (Indian tech jobs) and iimjobs.com (Indian
// management, finance and IT jobs). One company runs both on one backend, each site with its
// own API host and its own posting database, so the two are two providers over one adapter.
// Board-based aggregator: a board is one of the site's CATEGORY ids, the same id its category
// pages ("/c/backend-development-jobs") and the API's categoryId parameter use.
//
// The listing is the API the sites' own web app calls (gladiator.<site>/job/category/). A row
// carries everything but the body, so this is a HydratingSource that buys a posting's body from
// the posting API (/job/detail) only when the catalogue lacks it, newest first, up to
// hiristDetailBudget a run. See the Hirist traps in AGENTS.md.
type hirist struct {
	site hiristSite
	http HeaderJSONGetter
	// spent counts the detail requests claimed across every board of the run, shared by the
	// adapter's copies (boards may be crawled concurrently, hence atomic).
	spent *atomic.Int64
}

// hiristSite is what differs between the two sites: names, hosts, and which categories are
// technical by the site's own filing.
type hiristSite struct {
	provider string
	web      string // public site, which posting URLs point at
	api      string // the web app's API host
	tech     map[string]bool
}

var (
	// hirist.tech's categories are all IT functions bar four: 4 is semiconductor and VLSI
	// design (hardware, as naukri's area 4), 10 is product design, 12 product management and
	// 13 business analysis and project management. Their postings are left to the title
	// dictionary.
	hiristTechSite = hiristSite{
		provider: "hirist",
		web:      "https://www.hirist.tech",
		api:      "https://gladiator.hirist.tech",
		tech: map[string]bool{
			"1": true, "2": true, "3": true, "5": true, "6": true, "7": true, "8": true, "9": true,
			"11": true, "14": true, "15": true, "16": true, "17": true,
		},
	}
	// iimjobs.com is mostly banking, sales, consulting, HR and operations; 15 "IT & Systems" and
	// 25 "GenAI" are its technical categories.
	iimjobsSite = hiristSite{
		provider: "iimjobs",
		web:      "https://www.iimjobs.com",
		api:      "https://gladiator.iimjobs.com",
		tech:     map[string]bool{"15": true, "25": true},
	}
)

const (
	// hiristPageSize is the listing's page size; the API serves size=100 in full.
	hiristPageSize = 100
	// hiristMaxPages is a safety ceiling, not the platform's: the largest category held 6,346
	// postings (64 pages) on 2026-10-25. A walk that reaches it is logged.
	hiristMaxPages = 200
	// hiristRequestInterval paces every request of a run, listing and posting alike, to one a
	// second. Both sites' robots.txt ask crawlers for a 10 s delay on the web host; the API host
	// publishes no robots.txt, and one a second is this project's courtesy ceiling.
	hiristRequestInterval = time.Second
	hiristRequestBurst    = 1
	hiristDetailWorkers   = 2
	// hiristDetailBudget bounds one run's detail requests (~20 minutes at the pace above, plus
	// the ~300 listing pages of a full hirist walk). A cold start is ~30k postings on hirist and
	// ~25k on iimjobs, so the backlog drains over the first ~25 runs, newest first.
	hiristDetailBudget = 1200
	// hiristBoardDetailShare caps one board's claim on the run's budget, so the largest
	// categories' cold backlogs cannot take a whole run while the rest wait.
	hiristBoardDetailShare = 300
	// hiristLakh is the unit the salary fields are stated in: minSal 30 / maxSal 45 is the
	// "30-45 LPA" (lakhs per annum) the site displays.
	hiristLakh = 100_000
)

var hiristBoardRE = regexp.MustCompile(`^[0-9]+$`)

// hiristListingHeaders select the listing's current response shape, as the web app sends.
var hiristListingHeaders = map[string]string{"version": "2"}

// NewHirist builds the hirist.tech adapter over the given HTTP client.
func NewHirist(c HeaderJSONGetter) Source { return newHirist(hiristTechSite, c) }

// NewIIMJobs builds the iimjobs.com adapter over the given HTTP client.
func NewIIMJobs(c HeaderJSONGetter) Source { return newHirist(iimjobsSite, c) }

func newHirist(site hiristSite, c HeaderJSONGetter) Source {
	return hirist{site: site, http: c, spent: new(atomic.Int64)}
}

// pacedHiristGetter holds every request of a run to one limiter.
func pacedHiristGetter(c HeaderJSONGetter) HeaderJSONGetter {
	return rateLimitedHeaderJSONGetter{
		inner:   c,
		limiter: rate.NewLimiter(rate.Every(hiristRequestInterval), hiristRequestBurst),
	}
}

func (s hirist) Provider() string { return s.site.provider }

// Every posting names its own employer, so both sites stay in the source facet and in the
// cross-source duplicate suppression set — which matters here, since roughly half of hirist's
// postings are copied from the employer's own careers site (the API's "crawled" flag).
func (hirist) aggregator() {}

type hiristLocation struct {
	Name string `json:"name"`
}

// hiristListing is one category row, reduced to what the adapter reads.
type hiristListing struct {
	ID            int64            `json:"id"`
	Title         string           `json:"title"`
	MinExp        int              `json:"min"`
	MaxExp        int              `json:"max"`
	MinSal        float64          `json:"minSal"`
	MaxSal        float64          `json:"maxSal"`
	HideSal       int              `json:"hideSal"`
	Confidential  int              `json:"confidential"`
	WorkFromHome  int              `json:"workFromHome"`
	CreatedTimeMs int64            `json:"createdTimeMs"`
	Locations     []hiristLocation `json:"locations"`
	CompanyData   struct {
		CompanyName string `json:"companyName"`
	} `json:"companyData"`
}

type hiristCategoryPage struct {
	Data      []hiristListing `json:"data"`
	TotalJobs int             `json:"totalJobs"`
	HasMore   bool            `json:"hasMore"`
}

// hiristDetail is the posting API, reduced to what the listing row lacks and to its own
// statement that a posting is gone.
type hiristDetail struct {
	Data struct {
		ID                 int64            `json:"id"`
		IntroText          string           `json:"introText"`
		Locations          []hiristLocation `json:"locations"`
		HasExpired         bool             `json:"hasExpired"`
		PermanentlyRemoved bool             `json:"permanentlyRemoved"`
		State              int              `json:"state"`
	} `json:"data"`
}

func (s hirist) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

func (s hirist) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	return s.FetchNewGated(ctx, e, seen, DetailGate{})
}

// FetchNewGated is FetchNew that buys no body for a posting the pipeline will discard anyway.
// About half of hirist is copied from employers' own careers sites (see the type doc), and a
// discarded posting is never stored, so without the gate every run would buy those bodies again.
func (s hirist) FetchNewGated(ctx context.Context, e CompanyEntry, seen func(externalID string) bool, gate DetailGate) ([]Job, error) {
	if !hiristBoardRE.MatchString(e.Board) {
		return nil, fmt.Errorf("%s: board %q is not a category id", s.site.provider, e.Board)
	}
	rows, err := s.list(ctx, e.Board)
	if err != nil {
		return nil, err
	}
	return s.hydrate(ctx, rows, s.site.tech[e.Board], seen, gate)
}

// list walks the category to its end. The first page failing fails the board; a later page
// failing ends the walk with what was read. The order is the site's own (promoted postings
// first), so the rows are re-sorted newest first for the detail budget.
func (s hirist) list(ctx context.Context, category string) ([]hiristListing, error) {
	var out []hiristListing
	ids := map[int64]bool{}
	for page := 0; page < hiristMaxPages; page++ {
		u := fmt.Sprintf("%s/job/category/?categoryId=%s&page=%d&size=%d", s.site.api, category, page, hiristPageSize)
		var p hiristCategoryPage
		if err := s.http.GetJSONWithHeaders(ctx, u, hiristListingHeaders, &p); err != nil {
			if page == 0 {
				return nil, fmt.Errorf("%s: category %s: %w", s.site.provider, category, err)
			}
			log.Printf("%s: category %s: page %d failed; keeping %d postings read: %v", s.site.provider, category, page, len(out), err)
			break
		}
		for _, r := range p.Data {
			if r.ID <= 0 || ids[r.ID] {
				continue
			}
			ids[r.ID] = true
			out = append(out, r)
		}
		// hasMore is the end: totalPages rounds DOWN (103 postings answer totalPages 1 while
		// page 1 still serves the last 3), so it would drop a category's tail.
		if len(p.Data) == 0 || !p.HasMore {
			break
		}
		if page == hiristMaxPages-1 {
			log.Printf("%s: category %s: stopped at the %d-page ceiling with %d of %d postings read",
				s.site.provider, category, hiristMaxPages, len(out), p.TotalJobs)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedTimeMs > out[j].CreatedTimeMs })
	return out, nil
}

// hydrate maps the listed rows to Jobs. A seen posting is a liveness refresh with no request, and
// so is a posting the gate condemns; every other posting costs one detail request, up to the board's share of the run's budget.
// The budget is claimed in a sequential pass over the newest-first rows BEFORE any request
// goes out, so it buys exactly the newest postings whatever order the workers run in. A detail
// that could not be read, or that the budget did not reach, defers its posting to a later run
// rather than storing it body-less, which would never be hydrated again.
func (s hirist) hydrate(ctx context.Context, rows []hiristListing, tech bool, seen func(string) bool, gate DetailGate) ([]Job, error) {
	var covered map[string]bool
	if gate.Covered != nil {
		companies := make([]string, 0, len(rows))
		for _, r := range rows {
			companies = append(companies, strings.TrimSpace(r.CompanyData.CompanyName))
		}
		covered = gate.Covered(companies)
	}
	var jobs, claimed []Job
	board, deferred := 0, 0
	for _, r := range rows {
		base, ok := s.toJob(r, tech)
		if !ok {
			continue
		}
		if seen(base.ExternalID) {
			base.SeenRefresh = true
			jobs = append(jobs, base)
			continue
		}
		// A posting the pipeline will discard is yielded body-less, for it to count.
		if covered[base.Company] || (gate.Rejected != nil && gate.Rejected(base)) {
			jobs = append(jobs, base)
			continue
		}
		if board >= hiristBoardDetailShare || s.spent.Add(1) > hiristDetailBudget {
			deferred++
			continue
		}
		board++
		claimed = append(claimed, base)
	}
	var answered atomic.Int64
	jobs = append(jobs, fetchDetails(claimed, hiristDetailWorkers, func(base Job) (Job, bool) {
		job, ok, err := s.withDetail(ctx, base)
		if err != nil {
			log.Printf("%s: detail %s failed; deferring: %v", s.site.provider, base.ExternalID, err)
			return Job{}, false
		}
		answered.Add(1)
		return job, ok
	})...)
	// A crawl that asked for details and got no answer to any of them is broken, not quiet.
	// "Answered" counts a posting the platform reported gone.
	if len(claimed) > 0 && answered.Load() == 0 {
		return nil, fmt.Errorf("%s: requested %d posting details and none answered", s.site.provider, len(claimed))
	}
	if deferred > 0 {
		log.Printf("%s: detail budget reached (%d a board, %d a run); %d new postings deferred to later runs",
			s.site.provider, hiristBoardDetailShare, hiristDetailBudget, deferred)
	}
	return jobs, nil
}

// withDetail adds the posting's body and places. ok=false with a nil error is the platform's
// own answer that the posting is gone: a 404 JOB_NOT_FOUND, or a posting the API itself flags
// expired or removed (the web app sends its reader home on exactly those flags).
func (s hirist) withDetail(ctx context.Context, base Job) (Job, bool, error) {
	var d hiristDetail
	u := fmt.Sprintf("%s/job/detail?jobcode=%s", s.site.api, base.ExternalID)
	if err := s.http.GetJSONWithHeaders(ctx, u, nil, &d); err != nil {
		var se *StatusError
		if errors.As(err, &se) && (se.Code == http.StatusNotFound || se.Code == http.StatusGone) {
			return Job{}, false, nil
		}
		return Job{}, false, err
	}
	jd := d.Data
	if strconv.FormatInt(jd.ID, 10) != base.ExternalID {
		return Job{}, false, fmt.Errorf("posting answered id %d", jd.ID)
	}
	if jd.HasExpired || jd.PermanentlyRemoved || jd.State == 2 {
		return Job{}, false, nil
	}
	body := sanitizeHTML(jd.IntroText)
	if strings.TrimSpace(body) == "" {
		return Job{}, false, fmt.Errorf("posting carries no body")
	}
	base.Description = body
	if loc, mode := hiristPlaces(jd.Locations); loc != "" {
		base.Location, base.WorkMode = loc, mode
		base.Remote = mode == "remote"
	}
	return base, true, nil
}

// toJob maps a listing row to a body-less Job. A confidential posting hides its employer, and
// filing it under the placeholder would build one bogus company out of every such posting.
func (s hirist) toJob(r hiristListing, tech bool) (Job, bool) {
	company := strings.TrimSpace(r.CompanyData.CompanyName)
	title := hiristTitle(r.Title, company)
	if r.ID <= 0 || title == "" || company == "" || r.Confidential != 0 {
		return Job{}, false
	}
	id := strconv.FormatInt(r.ID, 10)
	j := Job{
		ExternalID: id,
		// The id is the identity: /j/<anything>-<id> serves the posting, so the slug is
		// built from the title only to read well.
		URL:               fmt.Sprintf("%s/j/%s-%s", s.site.web, hiristSlug(title), id),
		Title:             title,
		Company:           company,
		IsTechHint:        tech,
		FallbackCountries: []string{"in"},
	}
	j.Location, j.WorkMode = hiristPlaces(r.Locations)
	j.Remote = j.WorkMode == "remote"
	if r.CreatedTimeMs > 0 {
		t := time.UnixMilli(r.CreatedTimeMs)
		j.PostedAt = NotFuture(&t)
	}
	// min/max are the employer's experience range in years; zero states no floor.
	if r.MinExp > 0 && r.MaxExp >= r.MinExp {
		n := r.MinExp
		j.ExperienceYearsMin = &n
	}
	if r.HideSal == 0 && r.MinSal > 0 && r.MaxSal >= r.MinSal {
		lo, hi := int(r.MinSal*hiristLakh), int(r.MaxSal*hiristLakh)
		j.SalaryMin, j.SalaryMax = &lo, &hi
		j.SalaryCurrency = "INR"
		j.SalaryPeriod = "year"
	}
	return j, true
}

// hiristPlaceNames rewrites the site's location vocabulary where the location dictionary would
// misread it. "Anywhere in India/Multiple Locations" names several offices, not remote work,
// which is how the dictionary would read "anywhere"; "Delhi NCR" is not a place it knows. The
// labels that name no place at all are dropped. Everything else ("Gurgaon/Gurugram",
// "Cochin/Kochi", "Bangalore") the dictionary already resolves to its canonical city.
var hiristPlaceNames = map[string]string{
	"anywhere in india/multiple locations": "India",
	"delhi ncr":                            "Delhi",
	"others":                               "",
	"any location":                         "",
	"metros":                               "",
	"overseas/international":               "",
}

// hiristPlaces joins a posting's places into one location string and reads its work mode from
// them: "Remote" is a place in the site's vocabulary, so a posting whose ONLY place is Remote is
// remote work. Remote beside an office is left to the location parse, as is the posting-level
// workFromHome flag, which the site sets on postings whose only place is an office city too.
func hiristPlaces(locs []hiristLocation) (string, string) {
	var places []string
	remote, offices := false, 0
	for _, l := range locs {
		name := strings.TrimSpace(l.Name)
		if mapped, ok := hiristPlaceNames[strings.ToLower(name)]; ok {
			name = mapped
		}
		if name == "" {
			continue
		}
		if strings.EqualFold(name, "remote") {
			remote = true
		} else {
			offices++
		}
		places = append(places, name)
	}
	mode := ""
	if remote && offices == 0 {
		mode = "remote"
	}
	return strings.Join(places, "; "), mode
}

// hiristTitle drops the employer's name the site prefixes to most titles ("PTC Software -
// Technical Lead - Full Stack Development" under the company "PTC"), so the title matches the
// same role listed elsewhere. The prefix goes only when its first word is the company's.
func hiristTitle(title, company string) string {
	title = strings.Join(strings.Fields(title), " ")
	head, rest, ok := strings.Cut(title, " - ")
	if !ok || strings.TrimSpace(rest) == "" {
		return title
	}
	if w := hiristFirstWord(head); w != "" && w == hiristFirstWord(company) {
		return strings.TrimSpace(rest)
	}
	return title
}

// hiristFirstWord is a name's first word, lower-cased, skipping a leading "The".
func hiristFirstWord(s string) string {
	words := strings.Fields(strings.ToLower(s))
	if len(words) > 1 && words[0] == "the" {
		words = words[1:]
	}
	if len(words) == 0 {
		return ""
	}
	return strings.Trim(words[0], ".,()")
}

var hiristSlugRE = regexp.MustCompile(`[^a-z0-9]+`)

func hiristSlug(title string) string {
	return strings.Trim(hiristSlugRE.ReplaceAllString(strings.ToLower(title), "-"), "-")
}
