package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/time/rate"

	"github.com/strelov1/freehire/internal/dict/location"
)

// spainjobs adapts SpainJobs.io (www.spainjobs.io), an English-language board that gathers the
// open roles of ~3.5k employers in Spain from their own career pages: ~92k postings, each
// rewritten in English with the employer's own apply link. It is boardless and an aggregator — the
// employer is read per posting. Its value to the catalogue is the employers whose career pages
// sit on their own domains, which no ATS adapter can reach. See the "SpainJobs.io traps" section
// of AGENTS.md for the measurements behind each rule.
//
// The posting set is the job sitemaps, which carry only each posting's URL, and that URL spells
// the employer's slug — so the coverage gate is asked before a page is bought, and a posting of
// an employer we already crawl directly is never fetched. Every other fact comes from the posting
// page: its schema.org JobPosting, or the page itself when it carries none. The adapter hydrates
// only postings the catalogue lacks, most recently modified first, within a per-run budget.
type spainjobs struct {
	http           spainjobsHTTP
	limiter        waiter
	maxNewPerRun   int
	deadlineMargin time.Duration
}

// spainjobsHTTP is the transport the adapter needs: the sitemaps as XML and each posting page.
type spainjobsHTTP interface {
	XMLGetter
	HTMLGetter
}

const (
	spainjobsHost         = "www.spainjobs.io"
	spainjobsBaseURL      = "https://" + spainjobsHost
	spainjobsSitemapIndex = spainjobsBaseURL + "/sitemap.xml"
	// spainjobsSitemapNeedle selects the job sitemaps among the index's entries (jobs-new.xml,
	// jobs-0.xml, ...); the rest list companies, landings, regions and tags.
	spainjobsSitemapNeedle = "/sitemap/jobs-"

	// spainjobsRequestInterval paces every request of a run at one a second — a courtesy ceiling,
	// not a measured limit.
	spainjobsRequestInterval = time.Second
	spainjobsDetailWorkers   = 2
	// spainjobsMaxNewPerRun bounds how many unseen postings one run reads: 1,500 pages at one a
	// second is 25 minutes, inside the scheduler's default 2,400 s crawl budget with the
	// sitemaps. ~2k postings reach the sitemaps a day; the first runs face the whole catalogue,
	// less what the gate condemns, so it is taken in slices, most recently modified first.
	spainjobsMaxNewPerRun   = 1500
	spainjobsDeadlineMargin = 5 * time.Minute
)

// NewSpainJobs builds the adapter over the given client.
func NewSpainJobs(c spainjobsHTTP) Source {
	return spainjobs{
		http:           c,
		limiter:        rate.NewLimiter(rate.Every(spainjobsRequestInterval), 1),
		maxNewPerRun:   spainjobsMaxNewPerRun,
		deadlineMargin: spainjobsDeadlineMargin,
	}
}

func (spainjobs) Provider() string { return "spainjobs" }

// One national board, no per-tenant id.
func (spainjobs) boardless() {}

// Thousands of employers, each named per posting.
func (spainjobs) aggregator() {}

// spainjobsPosting is one sitemap entry: the id, the URL, the employer slug and title slug the
// path spells, and the entry's lastmod.
type spainjobsPosting struct {
	id      string
	url     string
	company string
	title   string
	lastmod string
}

// spainjobsPostingPath matches a posting URL, /companies/<company-slug>/<title-slug>--<id>. The
// id is the identity; the title slug is the advert's ORIGINAL title (often Spanish), the page
// carries its English rewrite.
var spainjobsPostingPath = regexp.MustCompile(`^/companies/([^/]+)/([^/]+)--([0-9a-f]{6,})/?$`)

func spainjobsParsePosting(raw, lastmod string) (spainjobsPosting, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host != spainjobsHost {
		return spainjobsPosting{}, false
	}
	m := spainjobsPostingPath.FindStringSubmatch(u.Path)
	if m == nil {
		return spainjobsPosting{}, false
	}
	unslug := func(s string) string { return strings.Join(strings.Fields(strings.ReplaceAll(s, "-", " ")), " ") }
	return spainjobsPosting{id: m[3], url: raw, company: unslug(m[1]), title: unslug(m[2]), lastmod: strings.TrimSpace(lastmod)}, true
}

// Fetch is the list-only fallback HydratingSource specifies: everything is unseen.
func (s spainjobs) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

func (s spainjobs) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	return s.FetchNewGated(ctx, e, seen, DetailGate{})
}

// FetchNewGated lists every posting the job sitemaps name, refreshes the ones already stored, and
// reads up to maxNewPerRun of the rest, most recently modified first. A posting the pipeline will
// discard anyway — its employer already covered by a non-aggregator source, or its title rejected
// — is judged on what its URL spells and yielded body-less without a request. A posting whose
// page cannot be read is not yielded; a later run tries again.
func (s spainjobs) FetchNewGated(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool, gate DetailGate) ([]Job, error) {
	postings, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	var covered map[string]bool
	if gate.Covered != nil {
		companies := make([]string, 0, len(postings))
		for _, p := range postings {
			companies = append(companies, p.company)
		}
		covered = gate.Covered(companies)
	}
	var jobs []Job
	var unseen []spainjobsPosting
	deferred := 0
	for _, p := range postings {
		base := Job{ExternalID: p.id, URL: p.url, Title: p.title, Company: p.company, FallbackCountries: []string{"es"}}
		if seen(base.ExternalID) {
			base.SeenRefresh = true
			jobs = append(jobs, base)
			continue
		}
		if covered[p.company] || (gate.Rejected != nil && gate.Rejected(base)) {
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
	read := fetchDetails(unseen, spainjobsDetailWorkers, func(p spainjobsPosting) (Job, bool) {
		if d, ok := ctx.Deadline(); ok && time.Until(d) < s.deadlineMargin {
			outOfTime.Add(1)
			return Job{}, false
		}
		job, ok, err := s.detail(ctx, p)
		if err != nil {
			log.Printf("spainjobs: posting %s failed; deferring: %v", p.id, err)
			return Job{}, false
		}
		answered.Add(1)
		return job, ok
	})
	if asked := int64(len(unseen)) - outOfTime.Load(); asked > 0 && answered.Load() == 0 {
		return nil, fmt.Errorf("spainjobs: requested %d posting pages and none answered", asked)
	}
	if n := outOfTime.Load(); n > 0 {
		log.Printf("spainjobs: %d new postings left unread as the run neared its deadline, deferred to later runs", n)
	}
	if deferred > 0 {
		log.Printf("spainjobs: %d new postings past this run's bound of %d, deferred to later runs", deferred, s.maxNewPerRun)
	}
	return append(jobs, read...), nil
}

// list reads every job sitemap the index names and returns their postings, most recently
// modified first. Any sitemap failing fails the crawl: a short list would read a skewed slice
// and leave the postings it missed un-refreshed.
func (s spainjobs) list(ctx context.Context) ([]spainjobsPosting, error) {
	if err := s.limiter.Wait(ctx); err != nil {
		return nil, err
	}
	index, err := getSitemap(ctx, s.http, spainjobsSitemapIndex)
	if err != nil {
		return nil, fmt.Errorf("spainjobs: sitemap index: %w", err)
	}
	ids := make(map[string]bool)
	var out []spainjobsPosting
	files := 0
	for _, sm := range index.Sitemaps {
		if !strings.Contains(sm.Loc, spainjobsSitemapNeedle) {
			continue
		}
		files++
		if err := s.limiter.Wait(ctx); err != nil {
			return nil, err
		}
		doc, err := getSitemap(ctx, s.http, strings.TrimSpace(sm.Loc))
		if err != nil {
			return nil, fmt.Errorf("spainjobs: sitemap %s: %w", sm.Loc, err)
		}
		for _, u := range doc.URLs {
			p, ok := spainjobsParsePosting(strings.TrimSpace(u.Loc), u.LastMod)
			if !ok || ids[p.id] {
				continue
			}
			ids[p.id] = true
			out = append(out, p)
		}
	}
	if files == 0 {
		return nil, fmt.Errorf("spainjobs: sitemap index names no %s file", spainjobsSitemapNeedle)
	}
	// RFC 3339 timestamps in one zone sort as strings.
	sort.SliceStable(out, func(i, j int) bool { return out[i].lastmod > out[j].lastmod })
	return out, nil
}

// spainjobsLD is the part of the posting page's schema.org JobPosting the adapter reads.
type spainjobsLD struct {
	Title              string          `json:"title"`
	Description        string          `json:"description"`
	DatePosted         string          `json:"datePosted"`
	EmploymentType     json.RawMessage `json:"employmentType"`
	JobLocationType    string          `json:"jobLocationType"`
	JobLocation        json.RawMessage `json:"jobLocation"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
	ExperienceRequirements struct {
		MonthsOfExperience int `json:"monthsOfExperience"`
	} `json:"experienceRequirements"`
	BaseSalary struct {
		Currency string `json:"currency"`
		Value    struct {
			UnitText string  `json:"unitText"`
			Value    float64 `json:"value"`
			MinValue float64 `json:"minValue"`
			MaxValue float64 `json:"maxValue"`
		} `json:"value"`
	} `json:"baseSalary"`
}

type spainjobsPlace struct {
	Address struct {
		Locality string `json:"addressLocality"`
		Region   string `json:"addressRegion"`
		Country  string `json:"addressCountry"`
	} `json:"address"`
}

// spainjobsOneOrMany decodes a schema.org field the site writes as one value or as a list.
func spainjobsOneOrMany[T any](raw json.RawMessage) []T {
	var one T
	if err := json.Unmarshal(raw, &one); err == nil {
		return []T{one}
	}
	var many []T
	_ = json.Unmarshal(raw, &many)
	return many
}

// detail reads one posting page. ok=false with a nil error is the site's answer that the posting
// is gone (404/410) or a page carrying no usable posting; an error is the absence of an answer.
func (s spainjobs) detail(ctx context.Context, p spainjobsPosting) (Job, bool, error) {
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
	var ld spainjobsLD
	if !ldJobPosting(root, &ld) {
		// About one page in ten (mostly staffing agencies' adverts) carries no JobPosting; the
		// page itself still holds the advert.
		ld = spainjobsFromPage(root)
	}
	job, ok := spainjobsJob(ld, p, spainjobsApplyURL(root))
	return job, ok, nil
}

// spainjobsTitleSuffix is the page title's shape: "<role> at <employer> | SpainJobs.io", with the
// city in parentheses after the employer on some ("Emergency Nurse at Marlex Recruiting
// (Valencia) | SpainJobs.io").
var spainjobsTitleSuffix = regexp.MustCompile(`^(.*) at (.+?)(?: \([^)]*\))? \| SpainJobs\.io$`)

// spainjobsFromPage reads an advert from a page that carries no JobPosting: the role from the
// h1, the employer from the page title, the body from the advert's own section.
func spainjobsFromPage(root *html.Node) spainjobsLD {
	var ld spainjobsLD
	if h1 := firstByTag(root, "h1"); h1 != nil {
		ld.Title = textContent(h1)
	}
	if m := spainjobsTitleSuffix.FindStringSubmatch(strings.TrimSpace(titleText(root))); m != nil {
		ld.HiringOrganization.Name = m[2]
	}
	if advert := firstByID(root, "job-advert"); advert != nil {
		ld.Description = innerHTML(advert)
	}
	return ld
}

// spainjobsApplyURL is the employer's own link behind the page's Apply button, or "" when the
// page shows none.
func spainjobsApplyURL(root *html.Node) string {
	var found string
	walk(root, func(n *html.Node) bool {
		if found != "" {
			return false
		}
		if n.Type != html.ElementNode || n.Data != "a" {
			return true
		}
		href := strings.TrimSpace(Attr(n, "href"))
		u, err := url.Parse(href)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || strings.HasSuffix(u.Hostname(), "spainjobs.io") {
			return true
		}
		if strings.EqualFold(strings.TrimSpace(textContent(n)), "Apply") {
			found = href
		}
		return false
	})
	return found
}

// spainjobsJob maps a posting's advert to a Job. ok is false for one with no title, employer or
// body. The Job's URL is the employer's own apply link when the page shows one: that is where a
// candidate applies, and the SpainJobs page is a rewrite of it.
func spainjobsJob(ld spainjobsLD, p spainjobsPosting, applyURL string) (Job, bool) {
	title := strings.Join(strings.Fields(ld.Title), " ")
	company := strings.Join(strings.Fields(ld.HiringOrganization.Name), " ")
	body := sanitizeHTML(ld.Description)
	if title == "" || company == "" || strings.TrimSpace(body) == "" {
		return Job{}, false
	}
	j := Job{
		ExternalID:        p.id,
		URL:               p.url,
		Title:             title,
		Company:           company,
		Description:       body,
		FallbackCountries: []string{"es"},
	}
	if applyURL != "" {
		j.URL = applyURL
	}
	var places []string
	for _, pl := range spainjobsOneOrMany[spainjobsPlace](ld.JobLocation) {
		city, region := strings.TrimSpace(pl.Address.Locality), strings.TrimSpace(pl.Address.Region)
		switch {
		case city != "" && region != "" && !strings.EqualFold(city, region):
			places = append(places, city+", "+region)
		case city != "":
			places = append(places, city)
		case region != "":
			places = append(places, region)
		}
		if code := location.NormalizeCountry(strings.TrimSpace(pl.Address.Country)); code != "" && !slices.Contains(j.Countries, code) {
			j.Countries = append(j.Countries, code)
		}
	}
	j.Location = strings.Join(places, "; ")
	if t, err := time.Parse(time.RFC3339, strings.TrimSpace(ld.DatePosted)); err == nil {
		j.PostedAt = NotFuture(&t)
	}
	if strings.EqualFold(strings.TrimSpace(ld.JobLocationType), "TELECOMMUTE") {
		j.WorkMode, j.Remote = "remote", true
	}
	for _, t := range spainjobsOneOrMany[string](ld.EmploymentType) {
		switch strings.ToUpper(strings.TrimSpace(t)) {
		case "FULL_TIME":
			j.EmploymentType = "full_time"
		case "PART_TIME":
			j.EmploymentType = "part_time"
		case "INTERN", "INTERNSHIP":
			j.EmploymentType = "internship"
		case "CONTRACTOR":
			j.EmploymentType = "contract"
		}
		if j.EmploymentType != "" {
			break
		}
	}
	if years := ld.ExperienceRequirements.MonthsOfExperience / 12; years > 0 {
		j.ExperienceYearsMin = &years
	}
	// The amount is a range (minValue/maxValue) or one figure (value); an hourly rate is left
	// as text in the body.
	sal := ld.BaseSalary
	minV, maxV := sal.Value.MinValue, sal.Value.MaxValue
	if minV == 0 && maxV == 0 {
		minV, maxV = sal.Value.Value, sal.Value.Value
	}
	period := map[string]string{"YEAR": "year", "MONTH": "month"}[strings.ToUpper(strings.TrimSpace(sal.Value.UnitText))]
	if sal.Currency != "" && period != "" && minV > 0 && maxV >= minV {
		lo, hi := int(minV), int(maxV)
		j.SalaryMin, j.SalaryMax = &lo, &hi
		j.SalaryCurrency = strings.ToUpper(strings.TrimSpace(sal.Currency))
		j.SalaryPeriod = period
	}
	return j, true
}
