package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"math/rand/v2"
	"net/url"
	"regexp"
	"strings"
	"time"

	xhtml "golang.org/x/net/html"
	"golang.org/x/time/rate"
)

// jobgether adapts Jobgether (jobgether.com), a remote-jobs board that re-lists postings from
// employers' own ATS boards (Workday, Ashby, Greenhouse, iCIMS, Rippling, Gupy, ...). Boardless
// and multi-company: the employer is read per posting, and the employer's own apply link, which
// the page carries, is kept as discovery evidence. See providers/jobgether.md for the
// measurements behind each rule.
//
// robots.txt leaves the sitemap and the /offer/<id>-<slug> pages open (it refuses query strings
// and the literal `/offer/{*}` template), so the whole catalogue is read the way a search engine
// reads it: sitemap.xml -> index-offers.xml -> offers-N.xml name every posting page, and each
// page's schema.org JobPosting carries the facts. The separate AI-agent search API
// (/astroapi/ai/jobs.json) is capped at 10 pages of 25, so it cannot enumerate the catalogue.
type jobgether struct {
	http    sitemapBoardHTTP
	limiter waiter
	maxNew  int
}

const (
	jobgetherSitemapURL = "https://jobgether.com/sitemap/index-offers.xml"
	// jobgetherRequestInterval is a courtesy ceiling: ~250 requests in a row from the prod host drew
	// no refusal, but the pages are ~220 KB each.
	jobgetherRequestInterval = 250 * time.Millisecond
	// jobgetherMaxNewPerRun bounds the pages one run buys. The sitemap names ~100k postings, so
	// the first runs work through a backlog; a steady state needs only the day's new postings.
	jobgetherMaxNewPerRun = 3000
)

// jobgetherPostingPath matches a posting URL and captures its id, the 24-hex Mongo ObjectId that
// opens the path segment (the slug after it is the title and can change).
var jobgetherPostingPath = regexp.MustCompile(`^https://jobgether\.com/offer/([0-9a-f]{24})-[^/?#]+$`)

// NewJobgether builds the Jobgether adapter over the given client.
func NewJobgether(c sitemapBoardHTTP) Source {
	return jobgether{
		http:    c,
		limiter: rate.NewLimiter(rate.Every(jobgetherRequestInterval), 1),
		maxNew:  jobgetherMaxNewPerRun,
	}
}

func (jobgether) Provider() string { return "jobgether" }

// One global catalogue, no per-tenant board.
func (jobgether) Boardless() {}

// Employers post here, each named per posting, and most are reposts of an ATS posting we may
// already hold, which the aggregator-duplicate pass relies on this marker to settle.
func (jobgether) Aggregator() {}

// jobgetherID returns the posting id a sitemap URL carries, or "".
func jobgetherID(loc string) string {
	if m := jobgetherPostingPath.FindStringSubmatch(loc); m != nil {
		return m[1]
	}
	return ""
}

// Fetch is the list-only fallback HydratingSource specifies: everything is unseen.
func (s jobgether) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew refreshes the postings already stored by identity and hydrates up to maxNew of the
// rest. A page that cannot be read is not yielded: nothing is stored for it yet, so a later run
// tries it again.
func (s jobgether) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	locs, err := s.locs(ctx)
	if err != nil {
		return nil, err
	}
	var unseen []string
	jobs := make([]Job, 0, len(locs))
	for _, loc := range locs {
		id := jobgetherID(loc)
		if seen(id) {
			jobs = append(jobs, Job{ExternalID: id, URL: loc, SeenRefresh: true})
			continue
		}
		unseen = append(unseen, loc)
	}
	// A random slice, not the sitemap's first: a page that stays unreadable stays unseen, and a
	// fixed prefix would hand it the same slot every run while later pages starve.
	if deferred := len(unseen) - s.maxNew; deferred > 0 {
		log.Printf("jobgether: %d new postings past this run's bound of %d, deferred to later runs", deferred, s.maxNew)
		rand.Shuffle(len(unseen), func(i, j int) { unseen[i], unseen[j] = unseen[j], unseen[i] }) //nolint:gosec // Only selects which postings to crawl this run.
		unseen = unseen[:s.maxNew]
	}
	read := fetchDetails(unseen, sitemapBoardDetailWorkers, func(loc string) (Job, bool) {
		return s.detail(ctx, loc)
	})
	if len(unseen) > 0 && len(read) == 0 {
		return nil, fmt.Errorf("jobgether: requested %d posting pages and none yielded a posting", len(unseen))
	}
	return append(jobs, read...), nil
}

// locs walks the offers sitemap index to every shard and returns the posting URLs, each once.
// A shard that cannot be read fails the run: a silently shorter listing would let the unseen
// sweep close every posting that lived in it.
func (s jobgether) locs(ctx context.Context) ([]string, error) {
	index, err := getSitemap(ctx, s.http, jobgetherSitemapURL)
	if err != nil {
		return nil, fmt.Errorf("jobgether: sitemap index: %w", err)
	}
	if len(index.Sitemaps) == 0 {
		return nil, fmt.Errorf("jobgether: sitemap index names no shard")
	}
	var out []string
	dup := map[string]bool{}
	for _, sm := range index.Sitemaps {
		if err := s.limiter.Wait(ctx); err != nil {
			return nil, err
		}
		shard, err := getSitemap(ctx, s.http, sm.Loc)
		if err != nil {
			return nil, fmt.Errorf("jobgether: sitemap %s: %w", sm.Loc, err)
		}
		for _, u := range shard.URLs {
			id := jobgetherID(u.Loc)
			if id == "" || dup[id] {
				continue
			}
			dup[id] = true
			out = append(out, u.Loc)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("jobgether: sitemap names no posting")
	}
	return out, nil
}

// jobgetherPosting is the part of a posting page's JobPosting the adapter reads. Every posting is
// a TELECOMMUTE one; where it may be worked from is applicantLocationRequirements (a country per
// entry), while jobLocation repeats the same countries once per state or country.
type jobgetherPosting struct {
	Title                         string             `json:"title"`
	Description                   string             `json:"description"`
	DatePosted                    string             `json:"datePosted"`
	ValidThrough                  string             `json:"validThrough"`
	EmploymentType                json.RawMessage    `json:"employmentType"`
	ApplicantLocationRequirements schemaNamedAreas   `json:"applicantLocationRequirements"`
	JobLocation                   sitemapBoardPlaces `json:"jobLocation"`
	HiringOrganization            sitemapldjsonOrg   `json:"hiringOrganization"`
	BaseSalary                    sitemapBoardSalary `json:"baseSalary"`
}

func (s jobgether) detail(ctx context.Context, loc string) (Job, bool) {
	if err := s.limiter.Wait(ctx); err != nil {
		return Job{}, false
	}
	root, err := s.http.GetHTML(ctx, loc)
	if err != nil {
		log.Printf("jobgether: posting %s: %v", loc, err)
		return Job{}, false
	}
	var p jobgetherPosting
	if !ldJobPosting(root, &p) {
		return Job{}, false
	}
	title := strings.TrimSpace(p.Title)
	company := strings.TrimSpace(p.HiringOrganization.Name)
	if title == "" || company == "" {
		return Job{}, false
	}
	// The sitemap keeps naming a posting after it has lapsed; its own validThrough says so.
	if end := parseRFC3339OrDate(strings.TrimSpace(p.ValidThrough)); end != nil && end.Before(time.Now()) {
		return Job{}, false
	}
	job := Job{
		ExternalID:     jobgetherID(loc),
		URL:            loc,
		Title:          title,
		Company:        company,
		Countries:      countriesFromCodes(p.countryCodes()),
		Description:    sanitizeHTML(html.UnescapeString(p.Description)),
		Remote:         true,
		WorkMode:       "remote",
		EmploymentType: schemaEmploymentRaw(p.EmploymentType),
		PostedAt:       NotFuture(parseRFC3339OrDate(strings.TrimSpace(p.DatePosted))),
	}
	if salary := p.BaseSalary.amount; salary != nil {
		if currency := strings.ToUpper(strings.TrimSpace(salary.Currency)); currency != "" {
			salary.apply(&job, currency)
		}
	}
	if target := jobgetherApplyURL(root); target != "" {
		job.ApplyMethod, job.ExternalApplyURL = ApplyExternal, target
	}
	return job, true
}

// countryCodes lists where the posting may be worked from: the applicant requirements when it
// states them, else the countries its job locations name.
func (p jobgetherPosting) countryCodes() []string {
	if names := p.ApplicantLocationRequirements.Names(); len(names) > 0 {
		return names
	}
	var out []string
	for _, a := range p.JobLocation {
		out = append(out, a.AddressCountry)
	}
	return out
}

// jobgetherApplyURL reads the employer's own apply link from the page body's data-offer
// attribute, a JSON object the apply button is rendered from. A posting applied for on Jobgether
// itself (isEasyApply) has no destination of ours to record.
func jobgetherApplyURL(root *xhtml.Node) string {
	var raw string
	walk(root, func(n *xhtml.Node) bool {
		if raw != "" {
			return false
		}
		if n.Type == xhtml.ElementNode && n.Data == "body" {
			raw = Attr(n, "data-offer")
			return false
		}
		return true
	})
	if raw == "" {
		return ""
	}
	var offer struct {
		ApplyURL    string `json:"applyUrl"`
		IsEasyApply bool   `json:"isEasyApply"`
	}
	if json.Unmarshal([]byte(raw), &offer) != nil || offer.IsEasyApply {
		return ""
	}
	u, err := url.Parse(strings.TrimSpace(offer.ApplyURL))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return ""
	}
	return u.String()
}
