package sources

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/time/rate"

	"github.com/strelov1/freehire/internal/dict/location"
)

// sapoemprego adapts SAPO Emprego (emprego.sapo.pt), one of Portugal's large general job boards:
// ~23k public postings across every trade, close to half of them posted by staffing agencies,
// ~6% filed under "Informática e Tecnologias". It is boardless and an aggregator — the employer
// is read per posting. See the "SAPO Emprego traps" section of AGENTS.md for the measurements
// behind each rule.
//
// The posting set comes from the offers sitemap (robots.txt disallows /offers/search, the JSON
// the listing pages page through), which carries nothing but each posting's URL and lastmod, so
// every fact is bought with the posting page's schema.org JobPosting: the adapter hydrates only
// postings the catalogue lacks and refreshes the rest by identity.
type sapoemprego struct {
	http    sapoempregoHTTP
	limiter waiter
	// freshPerRun and backlogPerRun bound one run's posting pages; see sapoempregoFreshPerRun.
	freshPerRun   int
	backlogPerRun int
	// deadlineMargin is how much of the run's deadline hydration leaves to the pipeline: a
	// hydrating crawl writes nothing until FetchNew returns, so one that runs into its deadline
	// stores nothing at all. Pages still unread when the margin is reached wait for a later run.
	deadlineMargin time.Duration
	now            func() time.Time
}

// sapoempregoHTTP is the transport the adapter needs: the sitemap as XML and each posting page.
type sapoempregoHTTP interface {
	XMLGetter
	HTMLGetter
}

const (
	sapoempregoBaseURL = "https://emprego.sapo.pt"
	sapoempregoSitemap = sapoempregoBaseURL + "/offers.xml"

	// sapoempregoRequestInterval paces every request of a run, sitemap and posting page alike, at
	// one a second — a courtesy ceiling toward a national site, not a measured limit.
	sapoempregoRequestInterval = time.Second
	// sapoempregoDetailWorkers keeps enough pages in flight that the limiter, not latency (~0.6 s
	// a page from the crawl host, ~2 s from a laptop), sets the pace.
	sapoempregoDetailWorkers = 3

	// sapoempregoFreshPerRun and sapoempregoBacklogPerRun split one run's 1,200 posting pages
	// (~20 minutes at one a second, leaving the rest of the scheduler's default 2,400 s crawl
	// budget to the 5 MB sitemap, the ~23k identity refreshes and the writes after the fetch,
	// which run ~7 new rows a second from the crawl host) into hydrationLanes' two lanes.
	//
	// The listing names no employer, so the coverage gate has nothing to judge before a page is
	// bought, and ~8% of the catalogue is discarded unsaved on every run: ~1,300 postings from
	// employers already covered by a non-aggregator source (GI Group alone posts 848) and ~2.5%
	// whose employer is hidden. Newest-first alone would let those fill the budget for good.
	sapoempregoFreshPerRun    = 800
	sapoempregoBacklogPerRun  = 400
	sapoempregoDeadlineMargin = 5 * time.Minute
)

// sapoempregoHidden is the employer name the JobPosting carries when the poster chose to stay
// anonymous. It is a placeholder, not a company, so such a posting is dropped rather than filed
// under one bogus employer collecting every anonymous posting on the site (catho's rule).
const sapoempregoHidden = "anónima"

// sapoempregoTechIndustry is the category the site files technical postings under. It is the
// poster's own structured choice, so it sets IsTechHint.
const sapoempregoTechIndustry = "informática e tecnologias"

// NewSapoEmprego builds the adapter over the given client.
func NewSapoEmprego(c sapoempregoHTTP) Source {
	return sapoemprego{
		http:           c,
		limiter:        rate.NewLimiter(rate.Every(sapoempregoRequestInterval), 1),
		freshPerRun:    sapoempregoFreshPerRun,
		backlogPerRun:  sapoempregoBacklogPerRun,
		deadlineMargin: sapoempregoDeadlineMargin,
		now:            time.Now,
	}
}

func (sapoemprego) Provider() string { return "sapoemprego" }

// One national board, no per-tenant id.
func (sapoemprego) boardless() {}

// Thousands of employers and agencies post here, each named per posting.
func (sapoemprego) aggregator() {}

// sapoempregoPosting is one sitemap entry: the posting's id (a UUID), its URL and when the site
// last touched it.
type sapoempregoPosting struct {
	id      string
	url     string
	lastmod time.Time
}

// sapoempregoID matches the posting id the URL's id parameter carries.
var sapoempregoID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// sapoempregoParsePosting reads a sitemap entry, or ok=false when it is not a posting URL. The
// path's slug is cosmetic; the id parameter is the identity.
func sapoempregoParsePosting(loc sitemapLoc) (sapoempregoPosting, bool) {
	raw := strings.TrimSpace(loc.Loc)
	u, err := url.Parse(raw)
	if err != nil || !strings.HasPrefix(u.Path, "/offers/") {
		return sapoempregoPosting{}, false
	}
	id := strings.ToLower(u.Query().Get("id"))
	if !sapoempregoID.MatchString(id) {
		return sapoempregoPosting{}, false
	}
	p := sapoempregoPosting{id: id, url: raw}
	if t := parseRFC3339(strings.TrimSpace(loc.LastMod)); t != nil {
		p.lastmod = *t
	}
	return p, true
}

// Fetch is the list-only fallback HydratingSource specifies: everything is unseen.
func (s sapoemprego) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew lists every posting the sitemap names, refreshes the ones already stored, and reads
// the pages hydrationLanes picks out of the rest. A posting whose page cannot be read is not
// yielded; a later run tries again.
func (s sapoemprego) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	postings, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	var jobs []Job
	var unseen []sapoempregoPosting
	for _, p := range postings {
		if seen(p.id) {
			jobs = append(jobs, Job{ExternalID: p.id, URL: p.url, SeenRefresh: true})
			continue
		}
		unseen = append(unseen, p)
	}
	picked := hydrationLanes(unseen, s.freshPerRun, s.backlogPerRun, s.now())
	var answered, outOfTime atomic.Int64
	read := fetchDetails(picked, sapoempregoDetailWorkers, func(p sapoempregoPosting) (Job, bool) {
		if s.nearDeadline(ctx) {
			outOfTime.Add(1)
			return Job{}, false
		}
		job, ok, err := s.detail(ctx, p)
		if err != nil {
			log.Printf("sapoemprego: posting %s failed; deferring: %v", p.id, err)
			return Job{}, false
		}
		answered.Add(1)
		return job, ok
	})
	// A crawl that read no page at all is broken, not quiet. "Answered" counts a posting the
	// site reported gone; postings left for want of time are not failures.
	if int64(len(picked)) > outOfTime.Load() && answered.Load() == 0 {
		return nil, fmt.Errorf("sapoemprego: requested %d posting pages and none answered", int64(len(picked))-outOfTime.Load())
	}
	if n := outOfTime.Load(); n > 0 {
		log.Printf("sapoemprego: %d new postings left unread as the run neared its deadline, deferred to later runs", n)
	}
	if deferred := len(unseen) - len(picked); deferred > 0 {
		log.Printf("sapoemprego: %d new postings past this run's bound of %d, deferred to later runs", deferred, len(picked))
	}
	return append(jobs, read...), nil
}

// nearDeadline reports whether the run's deadline is closer than the margin hydration leaves to
// the pipeline's own writes.
func (s sapoemprego) nearDeadline(ctx context.Context) bool {
	d, ok := ctx.Deadline()
	return ok && time.Until(d) < s.deadlineMargin
}

// list reads the offers sitemap and returns its postings, most recently touched first. The
// sitemap is the whole listing, so its failing fails the crawl.
func (s sapoemprego) list(ctx context.Context) ([]sapoempregoPosting, error) {
	if err := s.limiter.Wait(ctx); err != nil {
		return nil, err
	}
	doc, err := getSitemap(ctx, s.http, sapoempregoSitemap)
	if err != nil {
		return nil, fmt.Errorf("sapoemprego: sitemap: %w", err)
	}
	ids := make(map[string]bool)
	var out []sapoempregoPosting
	for _, loc := range doc.URLs {
		p, ok := sapoempregoParsePosting(loc)
		if !ok || ids[p.id] {
			continue
		}
		ids[p.id] = true
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("sapoemprego: sitemap names no posting")
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].lastmod.After(out[j].lastmod) })
	return out, nil
}

// sapoempregoLD is the part of the posting page's schema.org JobPosting the adapter reads.
type sapoempregoLD struct {
	Title           string `json:"title"`
	Description     string `json:"description"`
	DatePosted      string `json:"datePosted"`
	EmploymentType  string `json:"employmentType"`
	JobLocationType string `json:"jobLocationType"`
	Industry        string `json:"industry"`
	JobLocation     struct {
		Address struct {
			Locality string `json:"addressLocality"`
			Region   string `json:"addressRegion"`
			Country  string `json:"addressCountry"`
		} `json:"address"`
	} `json:"jobLocation"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
}

// sapoempregoSummary is what the page's "Resumo da Oferta" list states beside the JobPosting:
// the work arrangement and the annual salary band.
type sapoempregoSummary struct {
	workhome string
	salary   string
}

// detail reads one posting page. ok=false with a nil error is the site's answer that the posting
// is gone — a 404/410, or the search page it serves (HTTP 200, no JobPosting) for an id it no
// longer holds; an error is the absence of an answer.
func (s sapoemprego) detail(ctx context.Context, p sapoempregoPosting) (Job, bool, error) {
	if err := s.limiter.Wait(ctx); err != nil {
		return Job{}, false, err
	}
	root, err := s.http.GetHTML(ctx, p.url)
	if err != nil {
		var se *StatusError
		if errors.As(err, &se) && (se.Code == http.StatusNotFound || se.Code == http.StatusGone) {
			return Job{}, false, nil
		}
		return Job{}, false, err
	}
	var ld sapoempregoLD
	if !ldJobPosting(root, &ld) {
		return Job{}, false, nil
	}
	job, ok := sapoempregoJob(ld, sapoempregoReadSummary(root), p)
	return job, ok, nil
}

// sapoempregoReadSummary reads the summary list's items by their class.
func sapoempregoReadSummary(root *html.Node) sapoempregoSummary {
	box := firstByClass(root, "metadata")
	if box == nil {
		return sapoempregoSummary{}
	}
	item := func(class string) string {
		if n := firstByClass(box, class); n != nil {
			return strings.Join(strings.Fields(textContent(n)), " ")
		}
		return ""
	}
	return sapoempregoSummary{workhome: item("workhome"), salary: item("salary")}
}

// sapoempregoSalary matches the summary's annual band, "De 15.000€ a 25.000€".
var sapoempregoSalary = regexp.MustCompile(`(?i)de\s+([\d.]+)\s*€\s+a\s+([\d.]+)\s*€`)

// sapoempregoJob maps a posting's JobPosting and summary to a Job. ok is false for one with no
// title, body or named employer.
func sapoempregoJob(ld sapoempregoLD, sum sapoempregoSummary, p sapoempregoPosting) (Job, bool) {
	title := strings.Join(strings.Fields(ld.Title), " ")
	company := strings.TrimSpace(ld.HiringOrganization.Name)
	body := sanitizeHTML(ld.Description)
	if title == "" || company == "" || strings.EqualFold(company, sapoempregoHidden) || strings.TrimSpace(body) == "" {
		return Job{}, false
	}
	j := Job{
		ExternalID:        p.id,
		URL:               p.url,
		Title:             title,
		Company:           company,
		Description:       body,
		FallbackCountries: []string{"pt"},
	}
	addr := ld.JobLocation.Address
	city, region := strings.TrimSpace(addr.Locality), strings.TrimSpace(addr.Region)
	switch {
	case city != "" && region != "" && !strings.EqualFold(city, region):
		j.Location = city + ", " + region
	case city != "":
		j.Location = city
	case region != "":
		j.Location = region
	}
	if code := location.NormalizeCountry(strings.TrimSpace(addr.Country)); code != "" {
		j.Countries = []string{code}
	}
	if t, err := time.Parse("2006-01-02", strings.TrimSpace(ld.DatePosted)); err == nil {
		j.PostedAt = NotFuture(&t)
	}
	j.WorkMode = sapoempregoWorkMode(ld.JobLocationType, sum.workhome)
	j.Remote = j.WorkMode == "remote"
	j.EmploymentType = sapoempregoEmploymentType(ld.EmploymentType)
	j.IsTechHint = strings.EqualFold(strings.TrimSpace(ld.Industry), sapoempregoTechIndustry)
	if m := sapoempregoSalary.FindStringSubmatch(sum.salary); m != nil {
		lo, errLo := strconv.Atoi(strings.ReplaceAll(m[1], ".", ""))
		hi, errHi := strconv.Atoi(strings.ReplaceAll(m[2], ".", ""))
		if errLo == nil && errHi == nil && lo > 0 && hi >= lo {
			j.SalaryMin, j.SalaryMax = &lo, &hi
			j.SalaryCurrency = "EUR"
			j.SalaryPeriod = "year"
		}
	}
	return j, true
}

// sapoempregoWorkMode reads the two structured signals: the JobPosting's TELECOMMUTE and the
// summary's "Modelo de trabalho" (Presencial / Híbrido / Teletrabalho), which every posting
// states.
func sapoempregoWorkMode(locationType, workhome string) string {
	if strings.EqualFold(strings.TrimSpace(locationType), "TELECOMMUTE") {
		return "remote"
	}
	switch strings.ToLower(strings.TrimSpace(workhome)) {
	case "teletrabalho":
		return "remote"
	case "híbrido":
		return "hybrid"
	case "presencial":
		return "onsite"
	}
	return ""
}

// sapoempregoEmploymentType reads the JobPosting's employmentType (FULL_TIME or PART_TIME on
// every posting sampled).
func sapoempregoEmploymentType(ldType string) string {
	switch strings.ToUpper(strings.TrimSpace(ldType)) {
	case "FULL_TIME":
		return "full_time"
	case "PART_TIME":
		return "part_time"
	case "INTERN", "INTERNSHIP":
		return "internship"
	case "CONTRACTOR", "TEMPORARY":
		return "contract"
	}
	return ""
}
